import { 
  parseGitHubUrl, 
  findEarliestTimestamp
} from '../src/utils.mjs';
import {
  calculateFinalMetrics,
  processWorkflowRun
} from '../src/workflow.mjs';

import {
  fetchWithAuth, 
  fetchPRReviews, 
  fetchCommitAssociatedPRs, 
  fetchRepository, 
  fetchCommit, 
  fetchWorkflowRuns, 
  fetchWithPagination
} from './fetching.mjs';

export class AnalysisData {
  constructor() {
    this.allTraceEvents = [];
    this.allJobStartTimes = [];
    this.allJobEndTimes = [];
    this.allMetrics = AnalysisData.initializeMetrics();
    this.urlResults = [];
    this.globalEarliestTime = Infinity;
    this.globalLatestTime = 0;
    this.totalRuns = 0;
  }

  static initializeMetrics() {
    return {
      totalRuns: 0,
      successfulRuns: 0,
      failedRuns: 0,
      totalJobs: 0,
      failedJobs: 0,
      totalSteps: 0,
      failedSteps: 0,
      jobDurations: [],
      jobNames: [],
      jobUrls: [],
      stepDurations: [],
      runnerTypes: new Set(),
      totalDuration: 0,
      longestJob: { name: '', duration: 0 },
      shortestJob: { name: '', duration: Infinity },
      jobTimeline: []
    };
  }

  // Methods to update the data
  addTraceEvents(events) {
    this.allTraceEvents.push(...events);
  }

  addJobStartTimes(times) {
    this.allJobStartTimes.push(...times);
  }

  addJobEndTimes(times) {
    this.allJobEndTimes.push(...times);
  }

  addUrlResult(result) {
    this.urlResults.push(result);
  }

  updateGlobalTimeRange(earliestTime, latestTime) {
    if (earliestTime < this.globalEarliestTime) {
      this.globalEarliestTime = earliestTime;
    }
    if (latestTime > this.globalLatestTime) {
      this.globalLatestTime = latestTime;
    }
  }

  incrementTotalRuns(count) {
    this.totalRuns += count;
  }

  // Getters for accessing the data
  get traceEvents() {
    return this.allTraceEvents;
  }

  get jobStartTimes() {
    return this.allJobStartTimes;
  }

  get jobEndTimes() {
    return this.allJobEndTimes;
  }

  get metrics() {
    return this.allMetrics;
  }

  get results() {
    return this.urlResults;
  }

  get earliestTime() {
    return this.globalEarliestTime;
  }

  get latestTime() {
    return this.globalLatestTime;
  }

  get runsCount() {
    return this.totalRuns;
  }

  get urlCount() {
    return this.urlResults.length;
  }

  // Method to check if we have any data
  hasData() {
    return this.urlResults.length > 0;
  }

  // Method to reset all data (useful for testing or reusing the instance)
  reset() {
    this.allTraceEvents = [];
    this.allJobStartTimes = [];
    this.allJobEndTimes = [];
    this.allMetrics = AnalysisData.initializeMetrics();
    this.urlResults = [];
    this.globalEarliestTime = Infinity;
    this.globalLatestTime = 0;
    this.totalRuns = 0;
  }

  /**
   * Process a single GitHub URL and add its results to the analysis data
   * @param {string} githubUrl - The GitHub URL to process
   * @param {number} urlIndex - The index of this URL in the processing order
   * @param {Object} session - GitHub API session with authentication
   * @param {Object} progressBar - Progress bar instance for user feedback
   * @returns {Promise<boolean>} - True if URL was processed successfully, false if skipped
   */
  async processUrl(githubUrl, urlIndex, session, progressBar) {
    const { owner, repo, type, identifier } = parseGitHubUrl(githubUrl);
    const baseUrl = `https://api.github.com/repos/${owner}/${repo}`;
    
    // Extract URL-specific information
    const urlInfo = await this.extractUrlInfo(type, identifier, owner, repo, baseUrl, session);
    
    // Fetch workflow runs
    const allRuns = await this.fetchWorkflowRunsForUrl(type, identifier, baseUrl, urlInfo, session);
    
    if (allRuns.length === 0) {
      return false; // Skip this URL if no runs found
    }
    
    // Update progress bar
    progressBar.setUrlRuns(allRuns.length);
    
    // Process workflow runs
    const urlData = await this.processWorkflowRunsForUrl(allRuns, urlIndex, owner, repo, identifier, type, urlInfo, session, progressBar);
    
    // Store results
    this.addUrlResult(urlData);
    
    // Accumulate global data
    this.addTraceEvents(urlData.traceEvents);
    this.addJobStartTimes(urlData.jobStartTimes);
    this.addJobEndTimes(urlData.jobEndTimes);
    this.incrementTotalRuns(allRuns.length);
    this.updateGlobalTimeRange(urlData.earliestTime, Math.max(...urlData.jobEndTimes));
    
    return true;
  }

  /**
   * Extract URL-specific information based on type (PR or commit)
   */
  async extractUrlInfo(type, identifier, owner, repo, baseUrl, session) {
    if (type === 'pr') {
      return await this.extractPRInfo(identifier, owner, repo, baseUrl, session);
    } else {
      return await this.extractCommitInfo(identifier, owner, repo, baseUrl, session);
    }
  }

  /**
   * Extract PR-specific information
   */
  async extractPRInfo(identifier, owner, repo, baseUrl, session) {
    const analyzingPrUrl = `https://github.com/${owner}/${repo}/pull/${identifier}`;
    
    const prData = await fetchWithAuth(`${baseUrl}/pulls/${identifier}`, session);
    const prInfo = this.extractPRInfoFromData(prData, analyzingPrUrl);
    
    const reviews = await fetchPRReviews(owner, repo, identifier, session);
    const reviewEvents = reviews
      .filter(r => r.state === 'APPROVED' || /ship\s?it/i.test(r.body || ''))
      .map(r => ({ type: 'shippit', time: r.submitted_at, reviewer: r.user.login, url: r.html_url || analyzingPrUrl }));
    
    if (prData.merged_at) {
      reviewEvents.push({
        type: 'merged',
        time: prData.merged_at,
        mergedBy: prData.merged_by?.login || prData.merged_by?.name || null,
        url: analyzingPrUrl
      });
    }
    
    return {
      headSha: prInfo.headSha,
      branchName: prInfo.branchName,
      displayName: `PR #${identifier}`,
      displayUrl: analyzingPrUrl,
      reviewEvents,
      mergedAtMs: prData.merged_at ? new Date(prData.merged_at).getTime() : null,
      commitTimeMs: null
    };
  }

  /**
   * Extract PR info from PR data (static helper method)
   */
  extractPRInfoFromData(prData, analyzingPrUrl) {
    // Extract branch name from head ref
    const branchName = prData.head?.ref || 'unknown';
    
    // Extract head SHA
    const headSha = prData.head?.sha || null;
    
    if (!headSha) {
      throw new Error('Could not extract head SHA from PR data');
    }
    
    return {
      headSha,
      branchName
    };
  }

  /**
   * Extract commit-specific information
   */
  async extractCommitInfo(identifier, owner, repo, baseUrl, session) {
    const analyzingCommitUrl = `https://github.com/${owner}/${repo}/commit/${identifier}`;
    
    // Determine target branch
    let targetBranch = null;
    try {
      const prs = await fetchCommitAssociatedPRs(owner, repo, identifier, session);
      if (Array.isArray(prs) && prs.length > 0) {
        targetBranch = prs[0]?.base?.ref || null;
      }
    } catch {
      // ignore; we'll fallback below
    }
    
    if (!targetBranch) {
      try {
        const repoMeta = await fetchRepository(baseUrl, session);
        targetBranch = repoMeta?.default_branch || null;
      } catch {
        // leave null
      }
    }
    
    return {
      headSha: identifier,
      branchName: targetBranch || 'unknown',
      displayName: `commit ${identifier.substring(0, 8)}`,
      displayUrl: analyzingCommitUrl,
      reviewEvents: [],
      mergedAtMs: null,
      commitTimeMs: null
    };
  }

  /**
   * Fetch workflow runs for a URL based on its type
   */
  async fetchWorkflowRunsForUrl(type, identifier, baseUrl, urlInfo, session) {
    if (type === 'commit') {
      return await this.fetchWorkflowRunsForCommit(identifier, baseUrl, urlInfo, session);
    } else {
      // For PRs, use the head SHA from the PR info
      return await fetchWorkflowRuns(baseUrl, urlInfo.headSha, session);
    }
  }

    /**
   * Fetch workflow runs for a commit with special filtering
   */
  async fetchWorkflowRunsForCommit(identifier, baseUrl, urlInfo, session) {
    // Fetch all runs for this head SHA (unfiltered)
    const allRunsForHead = await fetchWorkflowRuns(baseUrl, identifier, session);
    
    // Filtered runs for timeline visualization
    const runs = await fetchWorkflowRuns(
      baseUrl,
      identifier,
      session,
      { branch: urlInfo.branchName && urlInfo.branchName !== 'unknown' ? urlInfo.branchName : undefined, event: 'push' }
    );
    
    // Get commit timestamp for filtering
    let commitTimeMs = null;
    try {
      const commitMeta = await fetchCommit(baseUrl, identifier, session);
      const dateStr = commitMeta?.commit?.committer?.date || commitMeta?.commit?.author?.date || null;
    } catch {
      // ignore; leave commitTimeMs null
    }
    
    // Filter runs by commit time
    const filteredRuns = Array.isArray(runs) && commitTimeMs
      ? runs.filter(r => {
          const runCreated = new Date(r.created_at).getTime();
          return isFinite(runCreated) && runCreated >= commitTimeMs;
        })
      : runs;
    
    // Store additional commit metadata
    urlInfo.commitTimeMs = commitTimeMs;
    urlInfo.allRunsForHeadCount = (allRunsForHead || []).length;
    urlInfo.allRunsComputeMs = await this.calculateTotalComputeTime(allRunsForHead, baseUrl, session);
    
    return filteredRuns;
  }

    /**
   * Calculate total compute time across multiple runs
   */
  async calculateTotalComputeTime(runs, baseUrl, session) {
    let totalComputeMs = 0;
    
    try {
      for (const run of (runs || [])) {
        const jobsUrl = `${baseUrl}/actions/runs/${run.id}/jobs?per_page=100`;
        const jobs = await fetchWithPagination(jobsUrl, session);
        
        for (const job of jobs) {
          if (job.started_at && job.completed_at) {
            const s = new Date(job.started_at).getTime();
            const e = new Date(job.completed_at).getTime();
            if (isFinite(s) && e > s) {
              totalComputeMs += (e - s);
            }
          }
        }
      }
    } catch {
      // ignore failures computing total compute time
    }
    
    return totalComputeMs;
  }

  /**
   * Process workflow runs for a URL and return processed data
   */
  async processWorkflowRunsForUrl(allRuns, urlIndex, owner, repo, identifier, type, urlInfo, session, progressBar) {
    // Initialize per-URL data structures
    const urlMetrics = AnalysisData.initializeMetrics();
    const urlTraceEvents = [];
    const urlJobStartTimes = [];
    const urlJobEndTimes = [];
    
    // Find earliest timestamp for this URL
    const urlEarliestTime = findEarliestTimestamp(allRuns);
    
    // Process each workflow run
    for (const [runIndex, run] of allRuns.entries()) {
      const workflowProcessId = (urlIndex + 1) * 1000 + runIndex + 1;
    
      progressBar.processRun();
      
      await processWorkflowRun(
        run, runIndex, workflowProcessId, urlEarliestTime,
        urlMetrics, urlTraceEvents, urlJobStartTimes, urlJobEndTimes,
        owner, repo, identifier, urlIndex,
        { type, displayUrl: urlInfo.displayUrl, identifier },
        session
      );
    }
    
    // Calculate final metrics for this URL
    const urlFinalMetrics = calculateFinalMetrics(urlMetrics, allRuns.length, urlJobStartTimes, urlJobEndTimes);
    
    return {
      owner,
      repo,
      identifier,
      branchName: urlInfo.branchName,
      headSha: urlInfo.headSha,
      metrics: urlFinalMetrics,
      traceEvents: urlTraceEvents,
      type,
      displayName: urlInfo.displayName,
      displayUrl: urlInfo.displayUrl,
      urlIndex,
      jobStartTimes: urlJobStartTimes,
      jobEndTimes: urlJobEndTimes,
      earliestTime: urlEarliestTime,
      reviewEvents: urlInfo.reviewEvents,
      mergedAtMs: urlInfo.mergedAtMs,
      commitTimeMs: urlInfo.commitTimeMs,
      allCommitRunsCount: urlInfo.allRunsForHeadCount,
      allCommitRunsComputeMs: urlInfo.allRunsComputeMs
    };
  }

  /**
   * Finalize the analysis by generating global analysis data
   * This includes concurrency counters and review/merge events
   */
  async finalizeAnalysis() {
    // Generate global concurrency counter events
    const { generateConcurrencyCounters } = await this.getVisualizationFunctions();
    
    generateConcurrencyCounters(this.jobStartTimes, this.jobEndTimes, this.traceEvents, this.earliestTime);
    
    // Add review/merge instant events to trace for visualization
    if (this.urlCount > 0) {
      await this.addReviewMergeEventsToTrace();
    }
  }

  /**
   * Add review/merge instant events to trace for visualization
   */
  async addReviewMergeEventsToTrace() {
    const { addThreadMetadata, greenText, yellowText } = await this.getVisualizationFunctions();
    
    const metricsProcessId = 999;
    const markersThreadId = 2;
    
    // Thread metadata for review/merge markers
    addThreadMetadata(this.traceEvents, metricsProcessId, markersThreadId, '🔖 Review & Merge Markers', 1);
    
    // Create instant events for each review/merge
    this.results.forEach(result => {
      if (result.reviewEvents && result.reviewEvents.length > 0) {
        // Determine timeline bounds for clamping
        let timelineStartMs = result.earliestTime;
        let timelineEndMs = result.earliestTime;
        
        if (result.metrics && Array.isArray(result.metrics.jobTimeline) && result.metrics.jobTimeline.length > 0) {
          timelineStartMs = Math.min(...result.metrics.jobTimeline.map(j => j.startTime));
          timelineEndMs = Math.max(...result.metrics.jobTimeline.map(j => j.endTime));
        }
        
        result.reviewEvents.forEach(event => {
          const originalEventTimeMs = new Date(event.time).getTime();
          // Clamp to timeline bounds to avoid extreme zoom-out
          const clampedEventTimeMs = Math.max(timelineStartMs, Math.min(originalEventTimeMs, timelineEndMs));
          // Normalize to URL earliest time then convert to microseconds
          const ts = (clampedEventTimeMs - result.earliestTime) * 1000;
          const name = event.type === 'merged' ? 'Merged' : 'Approved';
          const user = event.type === 'merged' ? (event.mergedBy || '') : (event.reviewer || '');
          const label = event.type === 'merged'
            ? (user ? greenText(`◆ merged by ${user}`) : greenText('◆ merged'))
            : (user ? yellowText(`▲ approved by ${user}`) : yellowText('▲ approved'));
          const userUrl = user ? `https://github.com/${user}` : '';
          
          this.addTraceEvents([{
            name,
            ph: 'i',
            s: 'p',
            ts,
            pid: metricsProcessId,
            tid: markersThreadId,
            args: {
              url_index: result.urlIndex + 1,
              source_url: result.displayUrl,
              source_type: result.type,
              source_identifier: result.identifier,
              user,
              user_url: userUrl,
              label,
              original_event_time_ms: originalEventTimeMs,
              clamped: originalEventTimeMs !== clampedEventTimeMs
            }
          }]);
        });
      }
    });
  }

  /**
   * Get visualization functions to avoid circular dependencies
   */
  async getVisualizationFunctions() {
    // Dynamic import to avoid circular dependencies
    const { 
      generateConcurrencyCounters, 
      addThreadMetadata, 
      greenText, 
      yellowText 
    } = await import('./visualization.mjs');
    
    return { 
      generateConcurrencyCounters, 
      addThreadMetadata, 
      greenText, 
      yellowText 
    };
  }
}
