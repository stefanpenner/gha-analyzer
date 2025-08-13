#!/usr/bin/env node

import fs, { writeFileSync } from 'fs';
import url from 'url';
import { spawn } from 'child_process';
import os from 'os';
import path from 'path';
import ProgressBar from './progress.mjs';
import { AnalysisData } from './src/analysis-data.mjs';
import {
  handleGithubError,
  fetchWithAuth,
  fetchWorkflowRuns,
  fetchRepository,
  fetchCommitAssociatedPRs,
  fetchCommit,
  fetchPRReviews,
  fetchWithPagination
} from './src/fetching.mjs';

import {
  makeClickableLink,
  grayText,
  greenText,
  redText,
  yellowText,
  blueText,
  humanizeTime,
  getJobGroup,
  findBottleneckJobs,
  generateTimelineVisualization,
  generateHighLevelTimeline,
  addThreadMetadata,
  generateConcurrencyCounters,
  openTraceInPerfetto,
  outputCombinedResults
} from './src/visualization.mjs';

const createContext = (token = process.env.GITHUB_TOKEN) => ({
  githubToken: token
});







function parseGitHubUrl(url) {
  const parsed = new URL(url);
  const pathParts = parsed.pathname.split('/').filter(Boolean);
  
  // Handle PR URLs: /owner/repo/pull/prNumber
  if (pathParts.length === 4 && pathParts[2] === 'pull') {
    return {
      owner: pathParts[0],
      repo: pathParts[1],
      type: 'pr',
      identifier: pathParts[3]
    };
  }
  
  // Handle commit URLs: /owner/repo/commit/commitSha
  if (pathParts.length === 4 && pathParts[2] === 'commit') {
    return {
      owner: pathParts[0],
      repo: pathParts[1],
      type: 'commit',
      identifier: pathParts[3]
    };
  }
  
  throw new Error(`Invalid GitHub URL: ${url}. Expected format: PR: https://github.com/owner/repo/pull/123 or Commit: https://github.com/owner/repo/commit/abc123...`);
}



function extractPRInfo(prData, prUrl = null) {
  if (!prData.head || !prData.base) {
    const errorMsg = prUrl 
      ? `Invalid ${makeClickableLink(prUrl, 'PR')} response - missing head or base information`
      : 'Invalid PR response - missing head or base information';
    console.error(errorMsg);
    process.exit(1);
  }
  return {
    branchName: prData.head.ref,
    headSha: prData.head.sha
  };
}















function findEarliestTimestamp(allRuns) {
  let earliest = Infinity;
  // Simple approximation - use the earliest run creation time
  for (const run of allRuns) {
    const runTime = new Date(run.created_at).getTime();
    earliest = Math.min(earliest, runTime);
  }
  return earliest;
}

async function processWorkflowRun(run, runIndex, processId, earliestTime, metrics, traceEvents, jobStartTimes, jobEndTimes, owner, repo, identifier, urlIndex = 0, currentUrlResult = null, context = null, progressBar = null) {
  if (progressBar) {
    progressBar.processRun();
  }
  
  metrics.totalRuns++;
  if (run.status === 'completed' && run.conclusion === 'success') {
    metrics.successfulRuns++;
  } else {
    metrics.failedRuns++;
  }
  
  const baseUrl = `https://api.github.com/repos/${run.repository.owner.login}/${run.repository.name}`;
  const jobsUrl = `${baseUrl}/actions/runs/${run.id}/jobs?per_page=100`;
  const jobs = await fetchWithPagination(jobsUrl, context);
  
  const absoluteRunStartMs = new Date(run.created_at).getTime();
  const absoluteRunEndMs = new Date(run.updated_at).getTime();
  const runStartTs = absoluteRunStartMs; // Keep in milliseconds for Perfetto
  let runEndTs = Math.max(runStartTs + 1, absoluteRunEndMs); // Ensure minimum 1ms duration
  
  for (const job of jobs) {
    if (job.completed_at) {
      const absoluteJobEndMs = new Date(job.completed_at).getTime();
      const jobEndTime = absoluteJobEndMs; // Keep in milliseconds
      runEndTs = Math.max(runEndTs, jobEndTime);
    }
  }
  
  const runDurationMs = runEndTs - runStartTs;
  metrics.totalDuration += runDurationMs;
  
  // Add process metadata for this workflow with URL information
  const sourceInfoForProcess = currentUrlResult.type === 'pr' ? `PR #${currentUrlResult.identifier}` : `commit ${currentUrlResult.identifier.substring(0, 8)}`;
  const processName = `[${urlIndex + 1}] ${sourceInfoForProcess} - ${run.name || `Run ${run.id}`} (${run.status})`;
  
  // Define colors for different URLs (cycling through a color palette)
  const colors = ['#4285f4', '#ea4335', '#fbbc04', '#34a853', '#ff6d01', '#46bdc6', '#7b1fa2', '#d81b60'];
  const colorIndex = urlIndex % colors.length;
  
  traceEvents.push({
    name: 'process_name',
    ph: 'M',
    pid: processId,
    args: { 
      name: processName,
      source_url: currentUrlResult.displayUrl,
      source_type: currentUrlResult.type,
      source_identifier: currentUrlResult.identifier,
      repository: `${owner}/${repo}`
    }
  });
  
  // Add process color for visual distinction
  traceEvents.push({
    name: 'process_color',
    ph: 'M',
    pid: processId,
    args: { 
      color: colors[colorIndex],
      color_name: `url_${urlIndex + 1}_color`
    }
  });
  
  // Thread 1: Workflow overview with jobs timeline
  const workflowThreadId = 1;
  addThreadMetadata(traceEvents, processId, workflowThreadId, `📋 Workflow Overview`, 0);
  
  // Add workflow run event on overview thread
  const workflowUrl = `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}`;
  const prUrl = `https://github.com/${owner}/${repo}/pull/${identifier}`;
  const sourceInfo = currentUrlResult.type === 'pr' ? `PR #${identifier}` : `commit ${identifier.substring(0, 8)}`;
  
  // Normalize timestamps for Perfetto (relative to earliest time)
  // Convert to microseconds for Chrome Tracing format when displayTimeUnit is 'ms'
  const normalizedRunStartTs = (runStartTs - earliestTime) * 1000; // Convert to microseconds
  const normalizedRunEndTs = (runEndTs - earliestTime) * 1000; // Convert to microseconds
  
  traceEvents.push({
    name: `Workflow: ${run.name || `Run ${run.id}`} [${urlIndex + 1}]`,
    ph: 'X',
    ts: normalizedRunStartTs,
    dur: normalizedRunEndTs - normalizedRunStartTs,
    pid: processId,
    tid: workflowThreadId,
    cat: 'workflow',
    args: {
      status: run.status,
      conclusion: run.conclusion,
      run_id: run.id,
      duration_ms: runDurationMs,
      job_count: jobs.length,
      url: workflowUrl,
      github_url: workflowUrl,
      pr_url: prUrl,
      pr_number: currentUrlResult.identifier,
      repository: `${owner}/${repo}`,
      source_url: currentUrlResult.displayUrl,
      source_type: currentUrlResult.type,
      source_identifier: currentUrlResult.identifier,
      url_index: urlIndex + 1
    }
  });
  
  // Process jobs (each job gets its own thread with steps)
  for (const [jobIndex, job] of jobs.entries()) {
    const jobThreadId = jobIndex + 10; // Start from thread 10 to keep workflow overview first
    await processJob(job, jobIndex, run, jobThreadId, processId, earliestTime, runStartTs, runEndTs, metrics, traceEvents, jobStartTimes, jobEndTimes, prUrl, urlIndex, currentUrlResult);
  }
}

async function processJob(job, jobIndex, run, jobThreadId, processId, earliestTime, runStartTs, runEndTs, metrics, traceEvents, jobStartTimes, jobEndTimes, prUrl, urlIndex, currentUrlResult) {
  // Check if job has started
  if (!job.started_at) {
    console.error(`  Skipping job ${job.name} - missing start time`);
    return;
  }
  
  // Check if job is still pending/running
  const isPending = job.status !== 'completed' || !job.completed_at;
  if (isPending) {
    // Track pending jobs in metrics
    metrics.pendingJobs = metrics.pendingJobs || [];
    metrics.pendingJobs.push({
      name: job.name,
      status: job.status,
      startedAt: job.started_at,
      url: job.html_url || `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}/job/${job.id}`
    });
  }
  
  // For pending jobs, use current time as end time for visualization
  const absoluteJobStartMs = new Date(job.started_at).getTime();
  const absoluteJobEndMs = isPending ? Date.now() : new Date(job.completed_at).getTime();
  
  // Update job metrics
  metrics.totalJobs++;
  if (isPending) {
    // Don't count pending jobs as failed, they're still running
  } else if (job.status !== 'completed' || job.conclusion !== 'success') {
    metrics.failedJobs++;
  }
  
  // Calculate job timing - use absolute timestamps for consistency
  const jobStartTs = absoluteJobStartMs; // Keep in milliseconds for Perfetto
  let jobEndTs = Math.max(jobStartTs + 1, absoluteJobEndMs); // Ensure minimum 1ms duration
  const jobDurationMs = jobEndTs - jobStartTs;
  
  // Validate timing
  if (jobStartTs >= jobEndTs || jobDurationMs <= 0) {
    console.error(`  Skipping job ${job.name} - invalid timing`);
    return;
  }
  
  // Update metrics
  metrics.jobDurations.push(jobDurationMs);
  metrics.jobNames.push(job.name);
  const metricsJobUrl = job.html_url || `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}/job/${job.id}`;
  metrics.jobUrls.push(metricsJobUrl);
  
  if (jobDurationMs > metrics.longestJob.duration) {
    metrics.longestJob = { name: job.name, duration: jobDurationMs };
  }
  if (jobDurationMs < metrics.shortestJob.duration) {
    metrics.shortestJob = { name: job.name, duration: jobDurationMs };
  }
  if (job.runner_name) {
    metrics.runnerTypes.add(job.runner_name);
  }
  
  // Track for concurrency
  jobStartTimes.push({ ts: jobStartTs, type: 'start' });
  jobEndTimes.push({ ts: jobEndTs, type: 'end' });
  const jobIcon = isPending ? '⏳' : job.conclusion === 'success' ? '✅' : job.conclusion === 'failure' ? '❌' : job.conclusion === 'skipped' || job.conclusion === 'cancelled' ? '⏸️' : '❓';
  const jobUrl = job.html_url || `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}/job/${job.id}`;

  // Add to timeline for visualization - use absolute timestamps for time display
  metrics.jobTimeline.push({
    name: job.name,
    startTime: jobStartTs,
    endTime: jobEndTs,
    conclusion: job.conclusion,
    status: job.status,
    url: jobUrl
  });
  
  // Add thread metadata for this job
  addThreadMetadata(traceEvents, processId, jobThreadId, `${jobIcon} ${job.name}`, jobIndex + 10);
  
  // Add job event (this shows the overall job duration)
  const sourceInfoForJob = currentUrlResult.type === 'pr' ? `PR #${currentUrlResult.identifier}` : `commit ${currentUrlResult.identifier.substring(0, 8)}`;
  
  // Normalize timestamps for Perfetto (relative to earliest time)
  // Convert to microseconds for Chrome Tracing format when displayTimeUnit is 'ms'
  const normalizedJobStartTs = (jobStartTs - earliestTime) * 1000; // Convert to microseconds
  const normalizedJobEndTs = (jobEndTs - earliestTime) * 1000; // Convert to microseconds
  
  traceEvents.push({
    name: `Job: ${job.name} [${urlIndex + 1}]`,
    ph: 'X',
    ts: normalizedJobStartTs,
    dur: normalizedJobEndTs - normalizedJobStartTs,
    pid: processId,
    tid: jobThreadId,
    cat: 'job',
    args: {
      status: job.status,
      conclusion: job.conclusion,
      duration_ms: jobDurationMs,
      runner_name: job.runner_name || 'unknown',
      step_count: job.steps.length,
      url: jobUrl,
      github_url: jobUrl,
      pr_url: prUrl,
      pr_number: prUrl.split('/').pop(),
      repository: prUrl.split('/').slice(-4, -2).join('/'),
      job_id: job.id,
      source_url: currentUrlResult.displayUrl,
      source_type: currentUrlResult.type,
      source_identifier: currentUrlResult.identifier,
      url_index: urlIndex + 1
    }
  });
  
  // Process steps on the same thread as the job
  for (const step of job.steps) {
    processStep(step, job, run, jobThreadId, processId, earliestTime, jobStartTs, jobEndTs, metrics, traceEvents, prUrl, urlIndex, currentUrlResult);
  }
}

function processStep(step, job, run, jobThreadId, processId, earliestTime, jobStartTs, jobEndTs, metrics, traceEvents, prUrl, urlIndex, currentUrlResult) {
  if (!step.started_at || !step.completed_at) return;
  
  // Update step metrics
  metrics.totalSteps++;
  if (step.conclusion === 'failure') {
    metrics.failedSteps++;
  }
  
  // Calculate step timing - use absolute timestamps for consistency
  const absoluteStepStartMs = new Date(step.started_at).getTime();
  const absoluteStepEndMs = new Date(step.completed_at).getTime();
  const stepStartTs = absoluteStepStartMs; // Keep in milliseconds for Perfetto
  let stepEndTs = Math.max(stepStartTs + 1, absoluteStepEndMs); // Ensure minimum 1ms duration
  if (stepEndTs > jobEndTs) {
    stepEndTs = Math.max(stepStartTs + 1, jobEndTs);
  }
  const stepDurationMs = stepEndTs - stepStartTs;
  
  // Validate timing
  if (stepStartTs >= stepEndTs || stepDurationMs <= 0) return;
  
  // Get step icon and category
  const stepIcon = getStepIcon(step.name, step.conclusion);
  const stepCategory = categorizeStep(step.name);
  
  // Add step event  
  const stepUrl = job.html_url || `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}/job/${job.id}`;
  
  // Update metrics
  metrics.stepDurations.push({
    name: `${stepIcon} ${step.name}`,
    duration: stepDurationMs,
    url: stepUrl,
    jobName: job.name
  });
  // Normalize timestamps for Perfetto (relative to earliest time)
  // Convert to microseconds for Chrome Tracing format when displayTimeUnit is 'ms'
  const normalizedStepStartTs = (stepStartTs - earliestTime) * 1000; // Convert to microseconds
  const normalizedStepEndTs = (stepEndTs - earliestTime) * 1000; // Convert to microseconds
  
  traceEvents.push({
    name: `${stepIcon} ${step.name} [${urlIndex + 1}]`,
    ph: 'X',
    ts: normalizedStepStartTs,
    dur: normalizedStepEndTs - normalizedStepStartTs,
    pid: processId,
    tid: jobThreadId,
    cat: stepCategory,
    args: {
      status: step.status,
      conclusion: step.conclusion,
      duration_ms: stepDurationMs,
      job_name: job.name,
      url: stepUrl,
      github_url: stepUrl,
      pr_url: prUrl,
      pr_number: prUrl.split('/').pop(),
      repository: prUrl.split('/').slice(-4, -2).join('/'),
      step_number: step.number,
      source_url: currentUrlResult.displayUrl,
      source_type: currentUrlResult.type,
      source_identifier: currentUrlResult.identifier,
      url_index: urlIndex + 1
    }
  });
}





function calculateMaxConcurrency(jobStartTimes, jobEndTimes) {
  if (jobStartTimes.length === 0) return 0;
  
  const allJobEvents = [...jobStartTimes, ...jobEndTimes].sort((a, b) => a.ts - b.ts);
  let currentConcurrency = 0;
  let maxConcurrency = 0;
  
  for (const event of allJobEvents) {
    if (event.type === 'start') {
      currentConcurrency++;
      maxConcurrency = Math.max(maxConcurrency, currentConcurrency);
    } else {
      currentConcurrency--;
    }
  }
  
  return maxConcurrency;
}

function calculateFinalMetrics(metrics, totalRuns, jobStartTimes, jobEndTimes) {
  const avgJobDuration = metrics.jobDurations.length ? 
    metrics.jobDurations.reduce((a, b) => a + b, 0) / metrics.jobDurations.length : 0;
  const avgStepDuration = metrics.stepDurations.length ? 
    metrics.stepDurations.reduce((a, b) => a + b.duration, 0) / metrics.stepDurations.length : 0;
  const successRate = metrics.totalRuns ? (metrics.successfulRuns / metrics.totalRuns * 100).toFixed(1) : 0;
  const jobSuccessRate = metrics.totalJobs ? ((metrics.totalJobs - metrics.failedJobs) / metrics.totalJobs * 100).toFixed(1) : 0;
  
  // Calculate actual max concurrency from job timing data
  const maxConcurrency = calculateMaxConcurrency(jobStartTimes, jobEndTimes);
  
  return {
    ...metrics,
    avgJobDuration,
    avgStepDuration,
    successRate,
    jobSuccessRate,
    maxConcurrency
  };
}

function analyzeSlowJobs(metrics, limit = 5) {
  // Create job data with names and durations
  const jobData = [];
  for (let i = 0; i < metrics.jobDurations.length; i++) {
    jobData.push({
      name: metrics.jobNames ? metrics.jobNames[i] : `Job ${i + 1}`,
      duration: metrics.jobDurations[i],
      url: metrics.jobUrls ? metrics.jobUrls[i] : null
    });
  }
  
  // Sort by duration (descending) and take top N
  return jobData
    .sort((a, b) => b.duration - a.duration)
    .slice(0, limit);
}

function analyzeSlowSteps(metrics, limit = 5) {
  // Steps already have name and duration, just sort and limit
  return metrics.stepDurations
    .sort((a, b) => b.duration - a.duration)
    .slice(0, limit);
}

// =============================================================================
// VISUALIZATION AND OUTPUT
// =============================================================================





function findOverlappingJobs(jobs) {
  const overlaps = [];
  for (let i = 0; i < jobs.length; i++) {
    for (let j = i + 1; j < jobs.length; j++) {
      const job1 = jobs[i];
      const job2 = jobs[j];
      
      // Check if jobs overlap in time
      if (job1.startTime < job2.endTime && job2.startTime < job1.endTime) {
        overlaps.push([job1, job2]);
      }
    }
  }
  return overlaps;
}



function calculateUnionDurationMs(intervals) {
  if (!Array.isArray(intervals) || intervals.length === 0) return 0;
  const ranges = intervals
    .map(({ startTime, endTime }) => ({ start: Math.min(startTime, endTime), end: Math.max(startTime, endTime) }))
    .filter(r => Number.isFinite(r.start) && Number.isFinite(r.end) && r.end > r.start)
    .sort((a, b) => a.start - b.start);

  if (ranges.length === 0) return 0;

  let total = 0;
  let currentStart = ranges[0].start;
  let currentEnd = ranges[0].end;

  for (let i = 1; i < ranges.length; i++) {
    const r = ranges[i];
    if (r.start <= currentEnd) {
      currentEnd = Math.max(currentEnd, r.end);
    } else {
      total += currentEnd - currentStart;
      currentStart = r.start;
      currentEnd = r.end;
    }
  }
  total += currentEnd - currentStart;
  return total;
}


function categorizeStep(stepName) {
  const name = stepName.toLowerCase();
  
  if (name.includes('checkout') || name.includes('clone')) return 'step_checkout';
  if (name.includes('setup') || name.includes('install') || name.includes('cache')) return 'step_setup';
  if (name.includes('build') || name.includes('compile') || name.includes('make')) return 'step_build';
  if (name.includes('test') || name.includes('spec') || name.includes('coverage')) return 'step_test';
  if (name.includes('lint') || name.includes('format') || name.includes('check')) return 'step_lint';
  if (name.includes('deploy') || name.includes('publish') || name.includes('release')) return 'step_deploy';
  if (name.includes('upload') || name.includes('artifact') || name.includes('store')) return 'step_artifact';
  if (name.includes('security') || name.includes('scan') || name.includes('audit')) return 'step_security';
  if (name.includes('notification') || name.includes('slack') || name.includes('email')) return 'step_notify';
  
  return 'step_other';
}

function getStepIcon(stepName, conclusion) {
  const name = stepName.toLowerCase();
  
  // Failure/success override icons
  if (conclusion === 'failure') return '❌';
  if (conclusion === 'cancelled') return '🚫';
  if (conclusion === 'skipped') return '⏭️';
  
  // Category-based icons
  if (name.includes('checkout') || name.includes('clone')) return '📥';
  if (name.includes('setup') || name.includes('install')) return '⚙️';
  if (name.includes('cache')) return '💾';
  if (name.includes('build') || name.includes('compile')) return '🔨';
  if (name.includes('test') || name.includes('spec')) return '🧪';
  if (name.includes('lint') || name.includes('format')) return '🔍';
  if (name.includes('deploy') || name.includes('publish')) return '🚀';
  if (name.includes('upload') || name.includes('artifact')) return '📤';
  if (name.includes('security') || name.includes('scan')) return '🔒';
  if (name.includes('notification') || name.includes('slack')) return '📢';
  if (name.includes('docker') || name.includes('container')) return '🐳';
  if (name.includes('database') || name.includes('migrate')) return '🗄️';
  
  return '▶️'; // Default step icon
}

async function main() {
  // Parse command line arguments
  const args = process.argv.slice(2);
  
  // Extract --perfetto and --open-in-perfetto flags
  let perfettoFile = null;
  let openInPerfetto = false;
  const filteredArgs = args.filter(arg => {
    if (arg.startsWith('--perfetto=')) {
      perfettoFile = arg.substring('--perfetto='.length);
      return false; // Remove from args
    }
    if (arg === '--open-in-perfetto') {
      openInPerfetto = true;
      return false; // Remove from args
    }
    return true;
  });
  
  // Parse GitHub URLs and validate inputs
  const githubUrls = filteredArgs.slice(0, -1); // All args except the last one (which might be token)
  const providedToken = filteredArgs[filteredArgs.length - 1];
  
  // Check if the last argument is a token (doesn't look like a URL)
  const urls = providedToken && !providedToken.startsWith('http') ? githubUrls : [...githubUrls, providedToken].filter(Boolean);
  
  // Create context with token
  const context = createContext(providedToken && !providedToken.startsWith('http') ? providedToken : undefined);
  
  if (urls.length === 0 || !context.githubToken) {
    // Provide clear error messages before showing usage info
    if (urls.length === 0) {
      console.error('Error: No GitHub URLs provided.');
    }
    if (!context.githubToken) {
      console.error('Error: GitHub token is required.');
    }
    console.error('');
    console.error('Usage: node main.mjs <github_url1> [github_url2] ... [token] [--perfetto=<file_name_for_trace.json>] [--open-in-perfetto]');
    console.error('');
    console.error('Supported URL formats:');
    console.error('  PR: https://github.com/owner/repo/pull/123');
    console.error('  Commit: https://github.com/owner/repo/commit/abc123...');
    console.error('');
    console.error('Examples:');
    console.error('  Single URL: node main.mjs https://github.com/owner/repo/pull/123');
    console.error('  Multiple URLs: node main.mjs https://github.com/owner/repo/pull/123 https://github.com/owner/repo/commit/abc123');
    console.error('  With token: node main.mjs https://github.com/owner/repo/pull/123 your_token');
    console.error('  With perfetto output: node main.mjs https://github.com/owner/repo/pull/123 --perfetto=trace.json');
    console.error('  With token and perfetto: node main.mjs https://github.com/owner/repo/pull/123 your_token --perfetto=trace.json');
    console.error('  Auto-open in perfetto: node main.mjs https://github.com/owner/repo/pull/123 --perfetto=trace.json --open-in-perfetto');
    console.error('');
    console.error('GitHub token can be provided as:');
    console.error('  1. Command line argument: node main.mjs <github_urls> <token>');
    console.error('  2. Environment variable: export GITHUB_TOKEN=<token>');
    console.error('');
    console.error('Perfetto tracing:');
    console.error('  Use --perfetto=<filename> to save Chrome Tracing format output to a file');
    console.error('  Use --open-in-perfetto to automatically open the trace in Perfetto UI');
    console.error('  If not specified, no trace file will be generated');
    process.exit(1);
  }

  // Initialize combined data structures
  const analysisData = new AnalysisData();

    // Initialize progress bar
  const progressBar = new ProgressBar(urls.length, 0);
  
  // Process each URL
  for (const [urlIndex, githubUrl] of urls.entries()) {
    progressBar.startUrl(urlIndex, githubUrl);
    
    try {
      const { owner, repo, type, identifier } = parseGitHubUrl(githubUrl);
      const baseUrl = `https://api.github.com/repos/${owner}/${repo}`;
      
      let headSha, branchName, displayName, displayUrl;
      let reviewEvents = [];

      if (type === 'pr') {
        // Handle PR
        const analyzingPrUrl = `https://github.com/${owner}/${repo}/pull/${identifier}`;

        const prData = await fetchWithAuth(`${baseUrl}/pulls/${identifier}`, context);
        const prInfo = extractPRInfo(prData, analyzingPrUrl);
        headSha = prInfo.headSha;
        branchName = prInfo.branchName;
        displayName = `PR #${identifier}`;
        displayUrl = analyzingPrUrl;

        const reviews = await fetchPRReviews(owner, repo, identifier, context);
        reviewEvents = reviews
          .filter(r => r.state === 'APPROVED' || /ship\s?it/i.test(r.body || ''))
          .map(r => ({ type: 'shippit', time: r.submitted_at, reviewer: r.user.login, url: r.html_url || analyzingPrUrl }));

        // Include PR merged timestamp as a timeline event if available
        if (prData.merged_at) {
          reviewEvents.push({
            type: 'merged',
            time: prData.merged_at,
            mergedBy: prData.merged_by?.login || prData.merged_by?.name || null,
            url: analyzingPrUrl
          });
        }
        var mergedAtMs = prData.merged_at ? new Date(prData.merged_at).getTime() : null;
      } else {
        // Handle commit
        const analyzingCommitUrl = `https://github.com/${owner}/${repo}/commit/${identifier}`;

        headSha = identifier;
        displayName = `commit ${identifier.substring(0, 8)}`;
        displayUrl = analyzingCommitUrl;

        // Determine the most relevant branch for this commit
        // Prefer the base branch of an associated PR; fallback to repo default_branch
        let targetBranch = null;
        try {
          const prs = await fetchCommitAssociatedPRs(owner, repo, headSha, context);
          if (Array.isArray(prs) && prs.length > 0) {
            targetBranch = prs[0]?.base?.ref || null;
          }
        } catch {
          // ignore; we'll fallback below
        }
        if (!targetBranch) {
          try {
            const repoMeta = await fetchRepository(baseUrl, context);
            targetBranch = repoMeta?.default_branch || null;
          } catch {
            // leave null
          }
        }
        branchName = targetBranch || 'unknown';
      }
      
      // For commit URLs: restrict to push runs on the inferred target branch and created at/after the commit time
      let allRuns;
      if (type === 'commit') {
        // Fetch all runs for this head SHA (unfiltered)
        const allRunsForHead = await fetchWorkflowRuns(
          baseUrl,
          headSha,
          context
        );
        // Filtered runs (e.g., by branch/event/time window) used for timeline visualization
        const runs = await fetchWorkflowRuns(
          baseUrl,
          headSha,
          context,
          { branch: branchName && branchName !== 'unknown' ? branchName : undefined, event: 'push' }
        );
        let commitTimeMs = null;
        try {
          const commitMeta = await fetchCommit(baseUrl, headSha, context);
          const dateStr = commitMeta?.commit?.committer?.date || commitMeta?.commit?.author?.date || null;
          if (dateStr) commitTimeMs = new Date(dateStr).getTime();
        } catch {
          // ignore; leave commitTimeMs null
        }
        allRuns = Array.isArray(runs) && commitTimeMs
          ? runs.filter(r => {
              const runCreated = new Date(r.created_at).getTime();
              return isFinite(runCreated) && runCreated >= commitTimeMs;
            })
          : runs;

        // Compute aggregate compute time across ALL runs for this commit head SHA
        let allRunsComputeMs = 0;
        try {
          for (const run of (allRunsForHead || [])) {
            const baseRepoUrl = `https://api.github.com/repos/${owner}/${repo}`;
            const jobsUrl = `${baseRepoUrl}/actions/runs/${run.id}/jobs?per_page=100`;
            const jobs = await fetchWithPagination(jobsUrl, context);
            for (const job of jobs) {
              if (job.started_at && job.completed_at) {
                const s = new Date(job.started_at).getTime();
                const e = new Date(job.completed_at).getTime();
                if (isFinite(s) && isFinite(e) && e > s) {
                  allRunsComputeMs += (e - s);
                }
              }
            }
          }
        } catch {
          // ignore failures computing full compute; leave as 0 if errors
        }
        var allRunsForHeadCount = (allRunsForHead || []).length;
      } else {
        allRuns = await fetchWorkflowRuns(baseUrl, headSha, context);
      }
      if (allRuns.length === 0) {
        continue; // Skip this URL and continue with others
      }
      
      // Update progress bar with run count for this URL
      progressBar.setUrlRuns(allRuns.length);

      // Initialize per-URL data structures
      const urlMetrics = AnalysisData.initializeMetrics();
      const urlTraceEvents = [];
      const urlJobStartTimes = [];
      const urlJobEndTimes = [];
      let urlEarliestTime = Infinity;

      // Find earliest timestamp for this URL
      urlEarliestTime = findEarliestTimestamp(allRuns);

      // urlEarliestTime is already in milliseconds, keep it as is for normalization
      const urlEarliestTimeForProcess = urlEarliestTime;
      
      // Process each workflow run for this URL
      for (const [runIndex, run] of allRuns.entries()) {
        const workflowProcessId = (urlIndex + 1) * 1000 + runIndex + 1; // URL-specific process ID
        await processWorkflowRun(run, runIndex, workflowProcessId, urlEarliestTimeForProcess, urlMetrics, urlTraceEvents, urlJobStartTimes, urlJobEndTimes, owner, repo, identifier, urlIndex, { type: type, displayUrl: displayUrl, identifier: identifier }, context, progressBar);
      }

      // Calculate metrics for this URL
      const urlFinalMetrics = calculateFinalMetrics(urlMetrics, allRuns.length, urlJobStartTimes, urlJobEndTimes);
      
      // Store URL-specific results
      analysisData.addUrlResult({
        owner,
        repo,
        identifier,
        branchName,
        headSha,
        metrics: urlFinalMetrics,
        traceEvents: urlTraceEvents,
        type,
        displayName,
        displayUrl,
        urlIndex,
        jobStartTimes: urlJobStartTimes,
        jobEndTimes: urlJobEndTimes,
        earliestTime: urlEarliestTime,
        reviewEvents,
        mergedAtMs: typeof mergedAtMs === 'number' ? mergedAtMs : null,
        commitTimeMs: typeof commitTimeMs === 'number' ? commitTimeMs : null
        , allCommitRunsCount: typeof allRunsForHeadCount === 'number' ? allRunsForHeadCount : undefined
        , allCommitRunsComputeMs: typeof allRunsComputeMs === 'number' ? allRunsComputeMs : undefined
      });

      // Accumulate global data
      analysisData.addTraceEvents(urlTraceEvents);
      analysisData.addJobStartTimes(urlJobStartTimes);
      analysisData.addJobEndTimes(urlJobEndTimes);
      analysisData.incrementTotalRuns(allRuns.length);
      
      // Update global time bounds
      // Both urlEarliestTime and job timestamps are in milliseconds
      const urlLatestTime = Math.max(...urlJobEndTimes);
      analysisData.updateGlobalTimeRange(urlEarliestTime, urlLatestTime);
    } catch (error) {
      console.error(`Error processing URL ${githubUrl}: ${error.message}`);
      continue; // Skip this URL and continue with others
    }
  }

  // Finish progress bar
  progressBar.finish();
  
  if (analysisData.urlCount === 0) {
    console.error('No workflow runs found for any of the provided URLs');
    process.exit(1);
  }

  // Generate global concurrency counter events
  generateConcurrencyCounters(analysisData.jobStartTimes, analysisData.jobEndTimes, analysisData.traceEvents, analysisData.earliestTime);

  // Add review/merge instant events to trace for visualization
  if (analysisData.urlCount > 0) {
    const metricsProcessId = 999;
    const markersThreadId = 2;
    // Thread metadata for review/merge markers
    addThreadMetadata(analysisData.traceEvents, metricsProcessId, markersThreadId, '🔖 Review & Merge Markers', 1);
    // Create instant events for each review/merge
    analysisData.results.forEach(result => {
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
          analysisData.addTraceEvents([{
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

  // Calculate combined metrics
  const combinedMetrics = calculateCombinedMetrics(analysisData.results, analysisData.runsCount, analysisData.jobStartTimes, analysisData.jobEndTimes);

  // Output combined results
  await outputCombinedResults(analysisData, combinedMetrics, perfettoFile, openInPerfetto);
}

/**
 * Generate high-level timeline showing PR/Commit execution times
 */


function calculateCombinedMetrics(urlResults, totalRuns, allJobStartTimes, allJobEndTimes) {
  const combined = {
    totalRuns,
    totalJobs: urlResults.reduce((sum, result) => sum + result.metrics.totalJobs, 0),
    totalSteps: urlResults.reduce((sum, result) => sum + result.metrics.totalSteps, 0),
    successRate: calculateCombinedSuccessRate(urlResults),
    jobSuccessRate: calculateCombinedJobSuccessRate(urlResults),
    maxConcurrency: Math.max(...allJobStartTimes.map((_, i) => {
      const time = allJobStartTimes[i];
      return allJobStartTimes.filter((start, j) => 
        start <= time && allJobEndTimes[j] >= time
      ).length;
    })),
    jobTimeline: urlResults.flatMap((result, urlIndex) => 
      result.metrics.jobTimeline.map(job => ({
        ...job,
        urlIndex,
        sourceUrl: result.displayUrl,
        sourceName: result.displayName
      }))
    )
  };
  
  return combined;
}

function calculateCombinedSuccessRate(urlResults) {
  const totalSuccessful = urlResults.reduce((sum, result) => {
    const rate = parseFloat(result.metrics.successRate);
    const normalized = Number.isFinite(rate) ? rate : 0;
    return sum + (result.metrics.totalRuns * normalized / 100);
  }, 0);
  const totalRuns = urlResults.reduce((sum, result) => sum + result.metrics.totalRuns, 0);
  return totalRuns > 0 ? (totalSuccessful / totalRuns * 100).toFixed(1) : '0.0';
}

function calculateCombinedJobSuccessRate(urlResults) {
  const totalSuccessfulJobs = urlResults.reduce((sum, result) => {
    const rate = parseFloat(result.metrics.jobSuccessRate);
    const normalized = Number.isFinite(rate) ? rate : 0;
    return sum + (result.metrics.totalJobs * normalized / 100);
  }, 0);
  const totalJobs = urlResults.reduce((sum, result) => sum + result.metrics.totalJobs, 0);
  return totalJobs > 0 ? (totalSuccessfulJobs / totalJobs * 100).toFixed(1) : '0.0';
}

// Export functions for testing
export {
  parseGitHubUrl,
  getJobGroup,
  calculateCombinedMetrics,
  calculateCombinedSuccessRate,
  calculateCombinedJobSuccessRate,
  findBottleneckJobs,
  humanizeTime,

  findEarliestTimestamp,
  calculateMaxConcurrency,
  calculateFinalMetrics,
  analyzeSlowJobs,
  analyzeSlowSteps,
  findOverlappingJobs,
  categorizeStep,
  getStepIcon,
  makeClickableLink,
  grayText,
  greenText,
  redText,
  yellowText,
  blueText,
  generateHighLevelTimeline,
  generateTimelineVisualization,
  createContext,
  extractPRInfo,
  ProgressBar
};

// Only run main if this is the entry point (not imported as a module)
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
