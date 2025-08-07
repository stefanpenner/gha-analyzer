#!/usr/bin/env node

/**
 * GitHub Actions Performance Profiler
 * 
 * Analyzes GitHub Actions workflow performance and generates Chrome Tracing format
 * output for visualization in Perfetto.dev or Chrome DevTools.
 * 
 * Features:
 * - Timeline visualization of jobs and steps
 * - Performance metrics and analysis
 * - Critical path identification
 * - Clickable links to GitHub Actions and PRs
 * - Chrome Tracing format for advanced analysis
 * 
 * Usage: node main.mjs <github_url> [github_token]
 *        GITHUB_TOKEN environment variable can be used instead of token argument
 *        
 * Supported URL formats:
 * - PR: https://github.com/owner/repo/pull/123
 * - Commit: https://github.com/owner/repo/commit/abc123...
 * 
 * @author GitHub Actions Performance Team
 * @version 1.0.0
 */

import fs, { writeFileSync } from 'fs';
import url from 'url';
import { spawn } from 'child_process';
import os from 'os';
import path from 'path';

// =============================================================================
// CONFIGURATION AND VALIDATION
// =============================================================================

// Token will be extracted in main() function to handle multi-URL scenarios
// Context object to hold global state
const createContext = (token = process.env.GITHUB_TOKEN) => ({
  githubToken: token
});

/**
 * Progress bar utility for showing processing status
 */
class ProgressBar {
  constructor(totalUrls, totalRuns) {
    this.totalUrls = totalUrls;
    this.totalRuns = totalRuns;
    this.currentUrl = 0;
    this.currentRun = 0;
    this.currentUrlRuns = 0;
    this.isProcessing = false;
  }

  startUrl(urlIndex, url) {
    this.currentUrl = urlIndex + 1;
    this.currentRun = 0;
    this.isProcessing = true;
    this.updateDisplay();
  }

  setUrlRuns(runCount) {
    this.currentUrlRuns = runCount;
    this.updateDisplay();
  }

  processRun() {
    this.currentRun++;
    this.updateDisplay();
  }

  updateDisplay() {
    if (!this.isProcessing) return;
    
    const urlProgress = `${this.currentUrl}/${this.totalUrls}`;
    const runProgress = this.currentUrlRuns > 0 ? ` (${this.currentRun}/${this.currentUrlRuns} runs)` : '';
    const progressText = `Processing URL ${urlProgress}${runProgress}...`;
    
    // Clear the current line and show progress
    process.stderr.write(`\r${progressText}`);
  }

  finish() {
    this.isProcessing = false;
    // Clear the progress line
    process.stderr.write('\r' + ' '.repeat(50) + '\r');
  }
}

// =============================================================================
// UTILITY FUNCTIONS
// =============================================================================

/**
 * Humanizes time duration to show the most appropriate unit
 * @param {number} seconds - Duration in seconds
 * @returns {string} - Humanized time string (e.g., "2h 3m 45s", "15m 30s", "45s")
 */
function humanizeTime(seconds) {
  if (seconds === 0) {
    return '0s';
  }
  if (seconds < 1) {
    return `${Math.round(seconds * 1000)}ms`;
  }
  
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);
  
  const parts = [];
  if (hours > 0) {
    parts.push(`${hours}h`);
  }
  if (minutes > 0) {
    parts.push(`${minutes}m`);
  }
  if (secs > 0 || parts.length === 0) {
    parts.push(`${secs}s`);
  }
  
  return parts.join(' ');
}

/**
 * Extracts group prefix from job name (before first ' / ')
 * @param {string} jobName - Job name
 * @returns {string} - Group name
 */
function getJobGroup(jobName) {
  // Split by '/' and take the first part as the group
  const parts = jobName.split(' / ');
  return parts.length > 1 ? parts[0] : jobName;
}

/**
 * Parses a GitHub PR URL or commit URL and extracts owner, repo, and identifier
 * @param {string} url - GitHub PR URL or commit URL
 * @returns {Object} - {owner, repo, type, identifier}
 * @throws {Error} - If URL format is invalid
 */
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

/**
 * Makes authenticated requests to GitHub API
 * @param {string} url - API endpoint URL
 * @returns {Promise<Object>} - JSON response
 */
async function fetchWithAuth(url, context) {
  const headers = {
    Authorization: `token ${context.githubToken}`,
    Accept: 'application/vnd.github.v3+json',
    'User-Agent': 'Node'
  };
  
  const response = await fetch(url, { headers });
  if (!response.ok) {
    console.error(`Error fetching ${url}: ${response.status} ${response.statusText}`);
    process.exit(1);
  }
  return response.json();
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

async function fetchWorkflowRuns(baseUrl, headSha, context) {
  const commitRunsUrl = `${baseUrl}/actions/runs?head_sha=${headSha}&per_page=100`;
  return await fetchWithPagination(commitRunsUrl, context);
}

async function fetchPRReviews(owner, repo, prNumber, context) {
  const reviewsUrl = `https://api.github.com/repos/${owner}/${repo}/pulls/${prNumber}/reviews?per_page=100`;
  return await fetchWithPagination(reviewsUrl, context);
}

async function fetchWithPagination(url, context) {
  const headers = {
    Authorization: `token ${context.githubToken}`,
    Accept: 'application/vnd.github.v3+json',
    'User-Agent': 'Node'
  };
  
  const allItems = [];
  let currentUrl = url;
  
  while (currentUrl) {
    const response = await fetch(currentUrl, { headers });
    if (!response.ok) {
      console.error(`Error fetching ${currentUrl}: ${response.status}`);
      break;
    }
    
    const data = await response.json();
    const items = Array.isArray(data) ? data : data.workflow_runs || data.jobs || [];
    allItems.push(...items);
    
    const linkHeader = response.headers.get('Link');
    currentUrl = linkHeader?.match(/<([^>]+)>;\s*rel="next"/)?.[1] || null;
  }
  
  return allItems;
}

// =============================================================================
// METRICS AND DATA PROCESSING
// =============================================================================

/**
 * Initializes metrics tracking object
 * @returns {Object} - Empty metrics object
 */
function initializeMetrics() {
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

function findEarliestTimestamp(allRuns) {
  let earliest = Infinity;
  // Simple approximation - use the earliest run creation time
  for (const run of allRuns) {
    const runTime = new Date(run.created_at).getTime();
    earliest = Math.min(earliest, runTime);
  }
  return earliest;
}

// =============================================================================
// TRACE EVENT PROCESSING
// =============================================================================

/**
 * Processes a workflow run and generates trace events
 * @param {Object} run - GitHub workflow run object
 * @param {number} runIndex - Index of the run
 * @param {number} processId - Process ID for trace events
 * @param {number} earliestTime - Earliest timestamp for normalization
 * @param {Object} metrics - Metrics tracking object
 * @param {Array} traceEvents - Array to store trace events
 * @param {Array} jobStartTimes - Array to track job start times
 * @param {Array} jobEndTimes - Array to track job end times
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} identifier - PR number or commit SHA
 */
async function processWorkflowRun(run, runIndex, processId, earliestTime, metrics, traceEvents, jobStartTimes, jobEndTimes, owner, repo, identifier, urlIndex = 0, currentUrlResult = null, context = null, progressBar = null) {
  // Update progress bar instead of console output
  if (progressBar) {
    progressBar.processRun();
  }
  
  // Update run metrics
  metrics.totalRuns++;
  if (run.status === 'completed' && run.conclusion === 'success') {
    metrics.successfulRuns++;
  } else {
    metrics.failedRuns++;
  }
  
  // Fetch jobs for this run
  const baseUrl = `https://api.github.com/repos/${run.repository.owner.login}/${run.repository.name}`;
  const jobsUrl = `${baseUrl}/actions/runs/${run.id}/jobs?per_page=100`;
  const jobs = await fetchWithPagination(jobsUrl, context);
  
  // Calculate run timing - use absolute timestamps for consistency
  const absoluteRunStartMs = new Date(run.created_at).getTime();
  const absoluteRunEndMs = new Date(run.updated_at).getTime();
  const runStartTs = absoluteRunStartMs; // Keep in milliseconds for Perfetto
  let runEndTs = Math.max(runStartTs + 1, absoluteRunEndMs); // Ensure minimum 1ms duration
  
  // Find latest job completion for run end time
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

function addThreadMetadata(traceEvents, processId, threadId, name, sortIndex) {
  traceEvents.push({
    name: 'thread_name',
    ph: 'M',
    pid: processId,
    tid: threadId,
    args: { name }
  });
  
  if (sortIndex !== undefined) {
    traceEvents.push({
      name: 'thread_sort_index',
      ph: 'M',
      pid: processId,
      tid: threadId,
      args: { sort_index: sortIndex }
    });
  }
}

function generateConcurrencyCounters(jobStartTimes, jobEndTimes, traceEvents, earliestTime) {
  if (jobStartTimes.length === 0) return;
  
  const allJobEvents = [...jobStartTimes, ...jobEndTimes].sort((a, b) => a.ts - b.ts);
  let currentConcurrency = 0;
  const metricsProcessId = 999;
  const counterThreadId = 1;
  
  // Add process metadata for global metrics
  traceEvents.push({
    name: 'process_name',
    ph: 'M',
    pid: metricsProcessId,
    args: { name: '📊 Global Metrics' }
  });
  
  addThreadMetadata(traceEvents, metricsProcessId, counterThreadId, '📈 Job Concurrency', 0);
  
  for (const event of allJobEvents) {
    if (event.type === 'start') {
      currentConcurrency++;
    } else {
      currentConcurrency--;
    }
    
    // Normalize timestamp relative to earliest time and convert to microseconds
    const normalizedTs = (event.ts - earliestTime) * 1000; // Convert to microseconds
    
    traceEvents.push({
      name: 'Concurrent Jobs',
      ph: 'C',
      ts: normalizedTs,
      pid: metricsProcessId,
      tid: counterThreadId,
      args: { 'Concurrent Jobs': currentConcurrency }
    });
  }
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

/**
 * Creates a clickable terminal link using ANSI escape sequences
 * @param {string} url - URL to link to
 * @param {string} text - Display text (defaults to URL)
 * @returns {string} - ANSI formatted clickable link
 */
function makeClickableLink(url, text = null) {
  // ANSI escape sequence for clickable links (OSC 8)
  // Format: \u001b]8;;URL\u0007TEXT\u001b]8;;\u0007
  const displayText = text || url;
  return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
}

function grayText(text) {
  // ANSI escape sequence for gray color (bright black)
  return `\u001b[90m${text}\u001b[0m`;
}

function greenText(text) {
  // ANSI escape sequence for green color
  return `\u001b[32m${text}\u001b[0m`;
}

function redText(text) {
  // ANSI escape sequence for red color
  return `\u001b[31m${text}\u001b[0m`;
}

function yellowText(text) {
  // ANSI escape sequence for yellow color
  return `\u001b[33m${text}\u001b[0m`;
}

function blueText(text) {
  // ANSI escape sequence for blue color
  return `\u001b[34m${text}\u001b[0m`;
}

function generateTimelineVisualization(metrics, repoActionsUrl, urlIndex = 0) {
  if (!metrics.jobTimeline || metrics.jobTimeline.length === 0) {
    return '';
  }

  const timeline = metrics.jobTimeline;
  
  // Calculate bottleneck jobs for this timeline
  const timelineBottlenecks = findBottleneckJobs(timeline);
  const bottleneckJobs = new Set();
  timelineBottlenecks.forEach(job => {
    // Create a unique identifier for the job to match against timeline jobs
    const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
    bottleneckJobs.add(jobKey);
  });
  const scale = 60; // Terminal width for timeline bars
  
  // Calculate timeline bounds across all jobs
  const earliestStart = Math.min(...timeline.map(job => job.startTime));
  const latestEnd = Math.max(...timeline.map(job => job.endTime));
  const totalDuration = latestEnd - earliestStart;
  
  // Removed redundant "Pipeline Timeline" line - now handled by the caller
  
  // Format start and end times for display (timeline uses absolute timestamps)
  const startTimeFormatted = new Date(earliestStart).toLocaleTimeString();
  const endTimeFormatted = new Date(latestEnd).toLocaleTimeString();
  
  console.error('┌' + '─'.repeat(scale + 2) + '┐');
  // Position start time on left, end time on right
  const startLabel = `Start: ${startTimeFormatted}`;
  const endLabel = `End: ${endTimeFormatted}`;
  const middlePadding = ' '.repeat(scale - startLabel.length - endLabel.length);
  
  console.error(`│ ${startLabel}${middlePadding}${endLabel} │`);
  console.error('├' + '─'.repeat(scale + 2) + '┤');
  
  // Group jobs by their prefix (before first ' / ')
  const jobGroups = {};
  timeline.forEach(job => {
    const groupKey = getJobGroup(job.name);
    if (!jobGroups[groupKey]) {
      jobGroups[groupKey] = [];
    }
    jobGroups[groupKey].push(job);
  });
  
  // Sort groups by their earliest member's start time
  const sortedGroupNames = Object.keys(jobGroups).sort((a, b) => {
    const earliestA = Math.min(...jobGroups[a].map(job => job.startTime));
    const earliestB = Math.min(...jobGroups[b].map(job => job.startTime));
    return earliestA - earliestB;
  });
  
  // Display each group with tree view
  sortedGroupNames.forEach(groupName => {
    const jobsInGroup = jobGroups[groupName];
    
    // Calculate wall time for this group (earliest start to latest end)
    const groupStartTime = Math.min(...jobsInGroup.map(job => job.startTime));
    const groupEndTime = Math.max(...jobsInGroup.map(job => job.endTime));
    const groupWallTime = groupEndTime - groupStartTime;
    const groupTotalSec = groupWallTime / 1000; // Convert milliseconds to seconds
    
    // Sort jobs within the group by start time
    const sortedJobsInGroup = jobsInGroup.sort((a, b) => a.startTime - b.startTime);
    
    // Show group header with total time
    const timeDisplay = humanizeTime(groupTotalSec);
    // Ensure group name is clean and doesn't contain any problematic characters
    const cleanGroupName = groupName.replace(/[^\w\s\-_/()]/g, '').trim();
    console.error(`│${' '.repeat(scale)}  │ 📁 ${cleanGroupName} (${timeDisplay}, ${jobsInGroup.length} jobs)`);
    
    // Show jobs indented under the group
    sortedJobsInGroup.forEach((job, index) => {
      const relativeStart = job.startTime - earliestStart;
      const duration = job.endTime - job.startTime;
      const durationSec = duration / 1000; // Convert milliseconds to seconds
      
      // Calculate positions in the timeline
      const startPos = Math.floor((relativeStart / totalDuration) * scale);
      const barLength = Math.max(1, Math.floor((duration / totalDuration) * scale));
      
      // Create the timeline bar with better formatting and colors
      const padding = ' '.repeat(Math.max(0, startPos));
      
      // Choose status icon and color based on job status
      let statusIcon, coloredBar;
      if (job.conclusion === 'success') {
        statusIcon = '█';
        coloredBar = greenText(statusIcon.repeat(Math.max(1, barLength)));
      } else if (job.conclusion === 'failure') {
        statusIcon = '█';
        coloredBar = redText(statusIcon.repeat(Math.max(1, barLength)));
      } else if (job.status === 'in_progress' || job.status === 'queued' || job.status === 'waiting') {
        statusIcon = '▒';
        coloredBar = blueText(statusIcon.repeat(Math.max(1, barLength)));
      } else if (job.conclusion === 'skipped' || job.conclusion === 'cancelled') {
        statusIcon = '░';
        coloredBar = grayText(statusIcon.repeat(Math.max(1, barLength)));
      } else {
        statusIcon = '░';
        coloredBar = grayText(statusIcon.repeat(Math.max(1, barLength)));
      }
      
      const remaining = ' '.repeat(Math.max(0, scale - startPos - Math.max(1, barLength)));
      
      // Extract job name without group prefix and ensure it's clean
      const jobNameParts = job.name.split(' / ');
      const jobNameWithoutPrefix = jobNameParts.length > 1 ? jobNameParts.slice(1).join(' / ') : job.name;
      // Ensure job name is clean and doesn't contain any problematic characters
      const cleanJobName = jobNameWithoutPrefix.replace(/[^\w\s\-_/()]/g, '').trim();
      
      // Add group indicator for multiple instances of the same job name
      const sameNameJobs = jobsInGroup.filter(j => j.name === job.name);
      const groupIndicator = sameNameJobs.length > 1 ? ` [${sameNameJobs.indexOf(job) + 1}]` : '';
      
      // Tree indentation
      const isLastJob = index === sortedJobsInGroup.length - 1;
      const treePrefix = isLastJob ? '└── ' : '├── ';
      
      // Check if this job is a bottleneck
      const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
      const isBottleneck = bottleneckJobs.has(jobKey);
      
      // Add bottleneck indicator for high-impact optimization opportunities
      const bottleneckIndicator = isBottleneck ? ' 🔥' : '';
      
      // Create text for job name and time (without tree prefix)
      const jobNameAndTime = `${cleanJobName}${groupIndicator} (${humanizeTime(durationSec)})${bottleneckIndicator}`;
      const jobLink = job.url ? makeClickableLink(job.url, jobNameAndTime) : jobNameAndTime;
      
      // Color the job name and time based on status
      let displayJobText;
      if (job.conclusion === 'success') {
        displayJobText = greenText(jobLink);
      } else if (job.conclusion === 'failure') {
        displayJobText = redText(jobLink);
      } else if (job.status === 'in_progress' || job.status === 'queued' || job.status === 'waiting') {
        displayJobText = blueText(`⏳ ${jobLink}`);
      } else if (job.conclusion === 'skipped' || job.conclusion === 'cancelled') {
        displayJobText = grayText(jobLink);
      } else {
        displayJobText = jobLink;
      }
      const displayText = `${treePrefix}${displayJobText}`;
      
      console.error(`│${padding}${coloredBar}${remaining}  │ ${displayText}`);
    });
    

  });
  
  console.error('└' + '─'.repeat(scale + 2) + '┘');
  
  // Timeline legend with colors
  console.error(`Legend: ${greenText('█ Success')}  ${redText('█ Failed')}  ${blueText('▒ Pending/Running')}  ${grayText('░ Cancelled/Skipped')}`);
  
  // Show group time summaries sorted by wall time
  const groupTimeSummaries = sortedGroupNames.map(groupName => {
    const jobsInGroup = jobGroups[groupName];
    const groupStartTime = Math.min(...jobsInGroup.map(job => job.startTime));
    const groupEndTime = Math.max(...jobsInGroup.map(job => job.endTime));
    const groupWallTime = groupEndTime - groupStartTime;
    const groupTotalSec = groupWallTime / 1000;
    return { name: groupName, totalSec: groupTotalSec, jobCount: jobsInGroup.length };
  }).sort((a, b) => b.totalSec - a.totalSec); // Sort by wall time descending
  

  
  // Show concurrency insights using original timeline for analysis
  const sortedJobs = [...timeline].sort((a, b) => a.startTime - b.startTime);
  
  // Show bottleneck jobs summary
  if (timelineBottlenecks.length > 0) {
    const bottleneckDuration = timelineBottlenecks.reduce((sum, job) => sum + (job.endTime - job.startTime), 0) / 1000;
    console.error(`Bottleneck jobs: ${timelineBottlenecks.length} jobs, ${humanizeTime(bottleneckDuration)} total`);
  }
}

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

function findBottleneckJobs(jobs) {
  if (jobs.length === 0) return [];
  
  // Filter out jobs with 0 or very short duration (less than 1 second)
  const significantJobs = jobs.filter(job => {
    const duration = job.endTime - job.startTime;
    return duration > 1000; // More than 1 second in milliseconds
  });
  
  if (significantJobs.length === 0) return [];
  
  // Sort jobs by duration (longest first)
  const sortedByDuration = [...significantJobs].sort((a, b) => {
    const durationA = b.endTime - b.startTime;
    const durationB = a.endTime - a.startTime;
    return durationA - durationB;
  });
  
  // Calculate total pipeline duration
  const pipelineStart = Math.min(...jobs.map(job => job.startTime));
  const pipelineEnd = Math.max(...jobs.map(job => job.endTime));
  const totalPipelineDuration = pipelineEnd - pipelineStart;
  
  // Find jobs that are significant bottlenecks (more than 10% of total pipeline time)
  const bottleneckThreshold = totalPipelineDuration * 0.1; // 10% threshold
  const bottleneckJobs = sortedByDuration.filter(job => {
    const duration = job.endTime - job.startTime;
    return duration > bottleneckThreshold;
  });
  
  // If no jobs meet the threshold, return the top 2 longest jobs
  if (bottleneckJobs.length === 0) {
    return sortedByDuration.slice(0, 2);
  }
  
  return bottleneckJobs;
}



// Helper function to categorize steps based on their names
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

// Helper function to get appropriate icon for steps
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

// =============================================================================
// PERFETTO INTEGRATION
// =============================================================================

/**
 * Downloads and uses the Perfetto open_trace_in_ui script to open a trace file
 * @param {string} traceFile - Path to the trace file to open
 */
async function openTraceInPerfetto(traceFile) {
  const scriptName = 'open_trace_in_ui';
  const scriptUrl = 'https://raw.githubusercontent.com/google/perfetto/main/tools/open_trace_in_ui';
  const tmpDir = os.tmpdir();
  const scriptPath = path.join(tmpDir, scriptName);
  
  try {
    console.error(`\n🚀 Opening trace in Perfetto UI...`);
    
    // Check if script already exists in temp directory
    if (!fs.existsSync(scriptPath)) {
      console.error(`📥 Downloading ${scriptName} from Perfetto...`);
      
      // Download the script using curl to temp directory
      const downloadResult = await new Promise((resolve, reject) => {
        const curl = spawn('curl', ['-L', '-o', scriptPath, scriptUrl], { stdio: 'inherit' });
        curl.on('close', (code) => {
          if (code === 0) {
            resolve();
          } else {
            reject(new Error(`Failed to download ${scriptName} (exit code: ${code})`));
          }
        });
        curl.on('error', reject);
      });
      
      // Make the script executable
      await new Promise((resolve, reject) => {
        const chmod = spawn('chmod', ['+x', scriptPath], { stdio: 'inherit' });
        chmod.on('close', (code) => {
          if (code === 0) {
            resolve();
          } else {
            reject(new Error(`Failed to make ${scriptName} executable (exit code: ${code})`));
          }
        });
        chmod.on('error', reject);
      });
    } else {
      console.error(`📁 Using existing script: ${scriptPath}`);
    }
    
    // Open the trace file using the script
    console.error(`🔗 Opening ${traceFile} in Perfetto UI...`);
    const openResult = await new Promise((resolve, reject) => {
      const openScript = spawn(scriptPath, [traceFile], { 
        stdio: 'inherit',
        env: { ...process.env, PYTHONIOENCODING: 'utf-8' }
      });
      openScript.on('close', (code) => {
        if (code === 0) {
          resolve();
        } else {
          reject(new Error(`Failed to open trace in Perfetto (exit code: ${code})`));
        }
      });
      openScript.on('error', (error) => {
        reject(new Error(`Failed to execute script: ${error.message}`));
      });
    });
    
    console.error(`✅ Trace opened successfully in Perfetto UI!`);
    
  } catch (error) {
    console.error(`❌ Failed to open trace in Perfetto: ${error.message}`);
    console.error(`💡 You can manually open the trace at: https://ui.perfetto.dev`);
    console.error(`   Then click "Open trace file" and select: ${traceFile}`);
  }
}

// =============================================================================
// MAIN EXECUTION
// =============================================================================

/**
 * Main function that orchestrates the entire profiling process
 */
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
  const allTraceEvents = [];
  const allJobStartTimes = [];
  const allJobEndTimes = [];
  const allMetrics = initializeMetrics();
  const urlResults = [];
  let globalEarliestTime = Infinity;
  let globalLatestTime = 0;
  let totalRuns = 0;

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
          .map(r => ({ type: 'shippit', time: r.submitted_at, reviewer: r.user.login }));
      } else {
        // Handle commit
        const analyzingCommitUrl = `https://github.com/${owner}/${repo}/commit/${identifier}`;

        headSha = identifier;
        branchName = 'commit';
        displayName = `commit ${identifier.substring(0, 8)}`;
        displayUrl = analyzingCommitUrl;
      }
      
      const allRuns = await fetchWorkflowRuns(baseUrl, headSha, context);
      if (allRuns.length === 0) {
        continue; // Skip this URL and continue with others
      }
      
      // Update progress bar with run count for this URL
      progressBar.setUrlRuns(allRuns.length);

      // Initialize per-URL data structures
      const urlMetrics = initializeMetrics();
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
      urlResults.push({
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
        reviewEvents
      });

      // Accumulate global data
      allTraceEvents.push(...urlTraceEvents);
      allJobStartTimes.push(...urlJobStartTimes);
      allJobEndTimes.push(...urlJobEndTimes);
      totalRuns += allRuns.length;
      
      // Update global time bounds
      // Both urlEarliestTime and job timestamps are in milliseconds
      if (urlEarliestTime < globalEarliestTime) {
        globalEarliestTime = urlEarliestTime;
      }
      const urlLatestTime = Math.max(...urlJobEndTimes);
      if (urlLatestTime > globalLatestTime) {
        globalLatestTime = urlLatestTime;
      }
    } catch (error) {
      console.error(`Error processing URL ${githubUrl}: ${error.message}`);
      continue; // Skip this URL and continue with others
    }
  }

  // Finish progress bar
  progressBar.finish();
  
  if (urlResults.length === 0) {
    console.error('No workflow runs found for any of the provided URLs');
    process.exit(1);
  }

  // Generate global concurrency counter events
  generateConcurrencyCounters(allJobStartTimes, allJobEndTimes, allTraceEvents, globalEarliestTime);

  // Calculate combined metrics
  const combinedMetrics = calculateCombinedMetrics(urlResults, totalRuns, allJobStartTimes, allJobEndTimes);

  // Output combined results
  await outputCombinedResults(urlResults, combinedMetrics, allTraceEvents, globalEarliestTime, globalLatestTime, perfettoFile, openInPerfetto);
}

/**
 * Generate high-level timeline showing PR/Commit execution times
 */
function generateHighLevelTimeline(sortedResults, globalEarliestTime, globalLatestTime) {
  const scale = 60;
  
  // Calculate timeline bounds from actual job data
  let timelineEarliestTime = Infinity;
  let timelineLatestTime = 0;
  
  sortedResults.forEach(result => {
    if (result.metrics.jobTimeline.length > 0) {
      const resultEarliestTime = Math.min(...result.metrics.jobTimeline.map(job => job.startTime));
      const resultLatestTime = Math.max(...result.metrics.jobTimeline.map(job => job.endTime));
      timelineEarliestTime = Math.min(timelineEarliestTime, resultEarliestTime);
      timelineLatestTime = Math.max(timelineLatestTime, resultLatestTime);
    }
  });
  
  const totalDuration = timelineLatestTime - timelineEarliestTime;
  
  // Format times for display (timeline uses absolute timestamps)
  const startTimeFormatted = new Date(timelineEarliestTime).toLocaleTimeString();
  const endTimeFormatted = new Date(timelineLatestTime).toLocaleTimeString();
  
  // Create timeline header
  const startLabel = `Start: ${startTimeFormatted}`;
  const endLabel = `End: ${endTimeFormatted}`;
  const middlePadding = ' '.repeat(Math.max(0, scale - startLabel.length - endLabel.length));
  
  console.error(`┌${'─'.repeat(scale + 2)}┐`);
  console.error(`│ ${startLabel}${middlePadding}${endLabel} │`);
  console.error('├' + '─'.repeat(scale + 2) + '┤');
  
  // Display each PR/Commit as a timeline bar
  sortedResults.forEach((result, index) => {
    // Calculate the actual wall time from the job timeline (earliest start to latest end)
    const resultEarliestTime = Math.min(...result.metrics.jobTimeline.map(job => job.startTime));
    const resultLatestTime = Math.max(...result.metrics.jobTimeline.map(job => job.endTime));
    const wallTimeSec = (resultLatestTime - resultEarliestTime) / 1000;
    
    // Calculate relative start position based on actual start time
    const relativeStart = resultEarliestTime - timelineEarliestTime;
    const startPos = Math.floor((relativeStart / totalDuration) * scale);
    
    // Calculate bar length based on wall time, but cap it to prevent overflow
    const maxBarLength = scale - startPos;
    const barLength = Math.max(1, Math.min(maxBarLength, Math.floor((wallTimeSec / (totalDuration / 1000)) * scale)));
    
    // Determine overall status for this URL
    const hasFailedJobs = result.metrics.jobTimeline.some(job => job.conclusion === 'failure');
    const hasPendingJobs = result.metrics.pendingJobs && result.metrics.pendingJobs.length > 0;
    const hasSkippedJobs = result.metrics.jobTimeline.some(job => job.conclusion === 'skipped' || job.conclusion === 'cancelled');

    // Format duration
    let timeDisplay;
    if (isNaN(wallTimeSec) || wallTimeSec <= 0) {
      timeDisplay = '0s';
    } else {
      timeDisplay = humanizeTime(wallTimeSec);
    }

    // Prepare bar with potential review markers
    const barChars = Array(barLength).fill('█');
    const markerInfos = [];
    if (result.reviewEvents && result.reviewEvents.length > 0) {
      result.reviewEvents.forEach(event => {
        const eventTime = new Date(event.time).getTime();
        const column = Math.floor(((eventTime - timelineEarliestTime) / totalDuration) * scale);
        const offset = column - startPos;
        if (offset >= 0 && offset < barLength) {
          barChars[offset] = '▲';
          markerInfos.push(`▲ ${event.reviewer}`);
        }
      });
    }
    const barString = barChars.join('');

    // Create full text for clickable link with URL index
    const fullText = `[${result.urlIndex + 1}] ${result.displayName} (${timeDisplay})`;

    // Choose color based on status
    let coloredBar, coloredLink;
    if (hasFailedJobs) {
      coloredBar = redText(barString);
      coloredLink = redText(makeClickableLink(result.displayUrl, fullText));
    } else if (hasPendingJobs) {
      coloredBar = blueText(barString);
      coloredLink = blueText(makeClickableLink(result.displayUrl, fullText));
    } else if (hasSkippedJobs) {
      coloredBar = grayText(barString);
      coloredLink = grayText(makeClickableLink(result.displayUrl, fullText));
    } else {
      coloredBar = greenText(barString);
      coloredLink = greenText(makeClickableLink(result.displayUrl, fullText));
    }

    // Create the timeline bar
    const padding = ' '.repeat(Math.max(0, startPos));
    const remaining = ' '.repeat(Math.max(0, scale - startPos - barLength));
    const markerLabel = markerInfos.length > 0 ? ' ' + markerInfos.join(' ') : '';

    console.error(`│${padding}${coloredBar}${remaining}  │ ${coloredLink}${markerLabel}`);
  });
  
  console.error('└' + '─'.repeat(scale + 2) + '┘');
}

/**
 * Calculate combined metrics from multiple URL results
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

/**
 * Calculate combined success rate across all URLs
 */
function calculateCombinedSuccessRate(urlResults) {
  const totalSuccessful = urlResults.reduce((sum, result) => {
    const rate = parseFloat(result.metrics.successRate);
    const normalized = Number.isFinite(rate) ? rate : 0;
    return sum + (result.metrics.totalRuns * normalized / 100);
  }, 0);
  const totalRuns = urlResults.reduce((sum, result) => sum + result.metrics.totalRuns, 0);
  return totalRuns > 0 ? (totalSuccessful / totalRuns * 100).toFixed(1) : '0.0';
}

/**
 * Calculate combined job success rate across all URLs
 */
function calculateCombinedJobSuccessRate(urlResults) {
  const totalSuccessfulJobs = urlResults.reduce((sum, result) => {
    const rate = parseFloat(result.metrics.jobSuccessRate);
    const normalized = Number.isFinite(rate) ? rate : 0;
    return sum + (result.metrics.totalJobs * normalized / 100);
  }, 0);
  const totalJobs = urlResults.reduce((sum, result) => sum + result.metrics.totalJobs, 0);
  return totalJobs > 0 ? (totalSuccessfulJobs / totalJobs * 100).toFixed(1) : '0.0';
}

/**
 * Output combined results from multiple URLs
 */
async function outputCombinedResults(urlResults, combinedMetrics, allTraceEvents, globalEarliestTime, globalLatestTime, perfettoFile, openInPerfetto = false) {
  // Simple completion message
  if (perfettoFile) {
    console.error(`\n✅ Generated ${allTraceEvents.length} trace events • Open in Perfetto.dev for analysis`);
  } else {
    console.error(`\n✅ Generated ${allTraceEvents.length} trace events • Use --perfetto=<filename> to save trace for Perfetto.dev analysis`);
  }
  
  // Professional summary report
  console.error(`\n${'='.repeat(80)}`);
  console.error(`📊 ${makeClickableLink('https://ui.perfetto.dev', 'GitHub Actions Performance Report - Multi-URL Analysis')}`);
  console.error(`${'='.repeat(80)}`);
  
  // Summary of all URLs
  console.error(`Analysis Summary: ${urlResults.length} URLs • ${combinedMetrics.totalRuns} runs • ${combinedMetrics.totalJobs} jobs • ${combinedMetrics.totalSteps} steps`);
  console.error(`Success Rate: ${combinedMetrics.successRate}% workflows, ${combinedMetrics.jobSuccessRate}% jobs • Peak Concurrency: ${combinedMetrics.maxConcurrency}`);
  
  // Check for pending jobs across all URLs
  const allPendingJobs = [];
  urlResults.forEach(result => {
    if (result.metrics.pendingJobs && result.metrics.pendingJobs.length > 0) {
      allPendingJobs.push(...result.metrics.pendingJobs.map(job => ({
        ...job,
        sourceUrl: result.displayUrl,
        sourceName: result.displayName
      })));
    }
  });
  
  if (allPendingJobs.length > 0) {
    console.error(`\n${blueText('⚠️  Pending Jobs Detected:')} ${allPendingJobs.length} jobs still running`);
    allPendingJobs.forEach((job, index) => {
      const jobLink = makeClickableLink(job.url, job.name);
      console.error(`  ${index + 1}. ${blueText(jobLink)} (${job.status}) - ${job.sourceName}`);
    });
    console.error(`\n  Note: Timeline shows current progress for pending jobs. Results may change as jobs complete.`);
  }
  
  // Combined Analysis Section (moved to first)
  console.error(`\n${makeClickableLink('https://uiperfetto.dev', 'Combined Analysis')}:`);
  
  // List PRs and Commits ordered by start time
  const sortedResults = [...urlResults].sort((a, b) => a.earliestTime - b.earliestTime);
  console.error(`\nIncluded URLs (ordered by start time):`);
  sortedResults.forEach((result, index) => {
    const repoUrl = `https://github.com/${result.owner}/${result.repo}`;
    if (result.type === 'pr') {
      console.error(`  ${index + 1}. ${makeClickableLink(result.displayUrl, result.displayName)} (${result.branchName}) - ${makeClickableLink(repoUrl, `${result.owner}/${result.repo}`)}`);
    } else {
      console.error(`  ${index + 1}. ${makeClickableLink(result.displayUrl, result.displayName)} - ${makeClickableLink(repoUrl, `${result.owner}/${result.repo}`)}`);
    }
  });
  
  // Combined pipeline timeline (PR/Commit level)
  console.error(`\nCombined Pipeline Timeline:`);
  generateHighLevelTimeline(sortedResults, globalEarliestTime, globalLatestTime);
  
  // Slowest jobs grouped by PR/Commit (ordered by start time like combined timeline)
  const allJobs = combinedMetrics.jobTimeline.sort((a, b) => (b.endTime - b.startTime) - (a.endTime - a.startTime));
  const slowJobs = allJobs.slice(0, 10);
  
  if (slowJobs.length > 0) {
    console.error(`\nSlowest Jobs (grouped by PR/Commit):`);
    
    // Calculate bottleneck jobs for each URL to identify high-impact optimizations
    const bottleneckJobs = new Set();
    sortedResults.forEach(result => {
      const bottlenecks = findBottleneckJobs(result.metrics.jobTimeline);
      bottlenecks.forEach(job => {
        // Create a unique identifier for the job to match against slowJobs
        const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
        bottleneckJobs.add(jobKey);
      });
    });
    
    // Group jobs by their source URL and sort by start time to match combined timeline order
    const jobsBySource = {};
    slowJobs.forEach(job => {
      const sourceKey = job.sourceUrl;
      if (!jobsBySource[sourceKey]) {
        jobsBySource[sourceKey] = [];
      }
      jobsBySource[sourceKey].push(job);
    });
    
    // Display grouped by source in the same order as combined timeline
    sortedResults.forEach(result => {
      const sourceUrl = result.displayUrl;
      const jobs = jobsBySource[sourceUrl];
      if (jobs && jobs.length > 0) {
        const headerText = `[${result.urlIndex + 1}] ${result.displayName}`;
        const headerLink = makeClickableLink(sourceUrl, headerText);
        console.error(`\n  ${headerLink}:`);
        // Sort jobs within each group by duration (descending) to show slowest first
        const sortedJobs = jobs.sort((a, b) => (b.endTime - b.startTime) - (a.endTime - a.startTime));
        sortedJobs.forEach((job, i) => {
          const duration = ((job.endTime - job.startTime) / 1000);
          
          // Check if this job is a bottleneck
          const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
          const isBottleneck = bottleneckJobs.has(jobKey);
          
          // Add bottleneck indicator for high-impact optimization opportunities
          const bottleneckIndicator = isBottleneck ? ' 🔥' : '';
          const fullText = `${i + 1}. ${humanizeTime(duration)} - ${job.name}${bottleneckIndicator}`;
          const jobLink = job.url ? makeClickableLink(job.url, fullText) : fullText;
          console.error(`    ${jobLink}`);
        });
      }
    });
    
    // Add explanation for bottleneck indicator
    const hasBottleneckJobs = slowJobs.some(job => {
      const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
      return bottleneckJobs.has(jobKey);
    });
    
    if (hasBottleneckJobs) {
      console.error(`\n  🔥 Bottleneck jobs - optimizing these will have the most impact on total pipeline time`);
    }
  }
  
  // Individual Pipeline Timelines Section (moved to after combined analysis)
  console.error(`\n${makeClickableLink('https://ui.perfetto.dev', 'Pipeline Timelines')}:`);
  
  urlResults.forEach((result, index) => {
    // Calculate wall time for this URL (earliest start to latest end)
    const timeline = result.metrics.jobTimeline;
    if (timeline && timeline.length > 0) {
      const earliestStart = Math.min(...timeline.map(job => job.startTime));
      const latestEnd = Math.max(...timeline.map(job => job.endTime));
      const wallTimeSec = (latestEnd - earliestStart) / 1000; // Convert milliseconds to seconds
      const wallTimeDisplay = humanizeTime(wallTimeSec);
      const headerText = `[${index + 1}] ${result.displayName} (${wallTimeDisplay}, ${result.metrics.totalJobs} jobs)`;
      const headerLink = makeClickableLink(result.displayUrl, headerText);
      console.error(`\n${headerLink}:`);
    } else {
      const headerText = `[${index + 1}] ${result.displayName} (${result.metrics.totalJobs} jobs)`;
      const headerLink = makeClickableLink(result.displayUrl, headerText);
      console.error(`\n${headerLink}:`);
    }
    generateTimelineVisualization(result.metrics, result.displayUrl, result.urlIndex);
  });
  
  // Generate combined trace metadata
  const traceTitle = `GitHub Actions: Multi-URL Analysis (${urlResults.length} URLs)`;
  const traceMetadata = [
    {
      name: 'process_name', 
      ph: 'M',
      pid: 0,
      args: { 
        name: traceTitle,
        url: 'https://perfetto.dev',
        github_url: 'https://github.com'
      }
    }
  ];

  // Output JSON for Perfetto only if flag is specified
  if (perfettoFile) {
    // Re-normalize all trace events to use global earliest time
    const renormalizedTraceEvents = allTraceEvents.map(event => {
      if (event.ts !== undefined) {
        // Find the URL-specific earliest time for this event
        const eventUrlIndex = event.args?.url_index || 1;
        const eventSource = event.args?.source_url;
        const urlResult = urlResults.find(result => 
          result.urlIndex === eventUrlIndex - 1 || 
          result.displayUrl === eventSource
        );
        
        if (urlResult) {
          // Convert back to absolute time, then normalize against global earliest time
          const absoluteTime = event.ts / 1000 + urlResult.earliestTime; // Convert from microseconds back to milliseconds, add URL earliest time
          const renormalizedTime = (absoluteTime - globalEarliestTime) * 1000; // Normalize against global earliest time, convert to microseconds
          return { ...event, ts: renormalizedTime };
        }
      }
      return event;
    });
    
    const output = {
      displayTimeUnit: 'ms',
      traceEvents: [...traceMetadata, ...renormalizedTraceEvents.sort((a, b) => a.ts - b.ts)],
      otherData: {
        trace_title: traceTitle,
        url_count: urlResults.length,
        total_runs: combinedMetrics.totalRuns,
        total_jobs: combinedMetrics.totalJobs,
        success_rate: `${combinedMetrics.successRate}%`,
        total_events: allTraceEvents.length,
        urls: urlResults.map((result, index) => ({
          index: index + 1,
          owner: result.owner,
          repo: result.repo,
          type: result.type,
          identifier: result.identifier,
          display_name: result.displayName,
          display_url: result.displayUrl,
          total_runs: result.metrics.totalRuns,
          total_jobs: result.metrics.totalJobs,
          success_rate: result.metrics.successRate
        })),
        performance_analysis: {
          slowest_jobs: slowJobs.map(job => ({
            name: job.name,
            duration_seconds: ((job.endTime - job.startTime) / 1000).toFixed(1),
            url: job.url,
            source_url: job.sourceUrl,
            source_name: job.sourceName
          }))
        }
      }
    };
    
    try {
      writeFileSync(perfettoFile, JSON.stringify(output, null, 2));
      console.error(`\n💾 Perfetto trace saved to: ${perfettoFile}`);
      
      // Auto-open in Perfetto if requested
      if (openInPerfetto) {
        await openTraceInPerfetto(perfettoFile);
      }
    } catch (error) {
      console.error(`Error writing perfetto trace to ${perfettoFile}:`, error);
      process.exit(1);
    }
  }
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
  initializeMetrics,
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
  fetchPRReviews,
  createContext,
  extractPRInfo,
  ProgressBar
};

// Only run main if this is the entry point (not imported as a module)
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
