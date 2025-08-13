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

const createContext = (token = process.env.GITHUB_TOKEN) => ({
  githubToken: token
});

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



function getJobGroup(jobName) {
  // Split by '/' and take the first part as the group
  const parts = jobName.split(' / ');
  return parts.length > 1 ? parts[0] : jobName;
}

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

function generateTimelineVisualization(metrics, repoActionsUrl, urlIndex = 0, reviewEvents = []) {
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
  const scale = 80; // Terminal width for timeline bars (80 characters)
  const headerScale = 60; // Header box width (original size)
  
  // Calculate timeline bounds across all jobs
  const earliestStart = Math.min(...timeline.map(job => job.startTime));
  const latestEnd = Math.max(...timeline.map(job => job.endTime));
  const totalDuration = latestEnd - earliestStart;
  
  // Top header: start/end box header for the visualization (60 characters)
  console.error('┌' + '─'.repeat(headerScale + 2) + '┐');
  // Format start and end times for display (timeline uses absolute timestamps)
  const startTimeFormatted = new Date(earliestStart).toLocaleTimeString();
  const endTimeFormatted = new Date(latestEnd).toLocaleTimeString();
  const headerStart = `Start: ${startTimeFormatted}`;
  const headerEnd = `End: ${endTimeFormatted}`;
  const headerPadding = ' '.repeat(Math.max(0, headerScale - headerStart.length - headerEnd.length));
  console.error(`│ ${headerStart}${headerPadding}${headerEnd} │`);
  console.error('├' + '─'.repeat(headerScale + 2) + '┤');
  
  
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
    console.error(`│${' '.repeat(headerScale)}  │ 📁 ${cleanGroupName} (${timeDisplay}, ${jobsInGroup.length} jobs)`);
    
    // Show jobs indented under the group
    sortedJobsInGroup.forEach((job, index) => {
      const relativeStart = job.startTime - earliestStart;
      const duration = job.endTime - job.startTime;
      const durationSec = duration / 1000; // Convert milliseconds to seconds
      
      // Calculate positions in the timeline (use headerScale for consistency with header box)
      const startPos = Math.floor((relativeStart / totalDuration) * headerScale);
      const barLength = Math.max(1, Math.floor((duration / totalDuration) * headerScale));
      
      // Ensure bar length doesn't exceed available space in headerScale
      const clampedBarLength = Math.min(barLength, headerScale - startPos);
      
      // Create the timeline bar with better formatting and colors
      const padding = ' '.repeat(Math.max(0, startPos));
      
      // Choose status icon and color based on job status
      let statusIcon, coloredBar;
      if (job.conclusion === 'success') {
        statusIcon = '█';
        coloredBar = greenText(statusIcon.repeat(Math.max(1, clampedBarLength)));
      } else if (job.conclusion === 'failure') {
        statusIcon = '█';
        coloredBar = redText(statusIcon.repeat(Math.max(1, clampedBarLength)));
      } else if (job.status === 'in_progress' || job.status === 'queued' || job.status === 'waiting') {
        statusIcon = '▒';
        coloredBar = blueText(statusIcon.repeat(Math.max(1, clampedBarLength)));
      } else if (job.conclusion === 'skipped' || job.conclusion === 'cancelled') {
        statusIcon = '░';
        coloredBar = grayText(statusIcon.repeat(Math.max(1, clampedBarLength)));
      } else {
        statusIcon = '░';
        coloredBar = grayText(statusIcon.repeat(Math.max(1, clampedBarLength)));
      }
      
      const remaining = ' '.repeat(Math.max(0, headerScale - startPos - Math.max(1, clampedBarLength)));
      
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
  
  // Approvals & Merge as a dedicated directory-like group with one entry per event
  const approvalAndMergeEvents = (reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged');
  if (approvalAndMergeEvents.length > 0 && totalDuration > 0) {
    console.error(`│${' '.repeat(headerScale)}  │ 📁 Approvals & Merge (${approvalAndMergeEvents.length} items)`);
    const sortedEvents = [...approvalAndMergeEvents].sort((a, b) => new Date(a.time) - new Date(b.time));
    // Combined marker line rendering both ▲ review and ◆ merged markers on the same line
    {
      const markerSlots = Array(headerScale).fill(' ');
      const reviewers = [];
      // Note: Only show approval markers (▲) in combined line, merge markers (◆) shown in detailed events
      sortedEvents.forEach(ev => {
        const eventTime = new Date(ev.time).getTime();
        const relativeStart = Math.max(0, Math.min(eventTime, latestEnd) - earliestStart);
        const col = Math.floor((relativeStart / totalDuration) * headerScale);
        const clampedCol = Math.max(0, Math.min(col, Math.max(0, headerScale - 1)));
        if (ev.type === 'shippit') {
          markerSlots[clampedCol] = '▲';
          if (ev.reviewer) reviewers.push(ev.reviewer);
        }
        // Merge markers (◆) are not shown in combined line to avoid duplication
      });
      const markerLineLeft = markerSlots.join('');
      const rightParts = [];
      if (reviewers.length > 0) rightParts.push(yellowText(`▲ ${reviewers[0]}`));
      // Note: Merge information is shown in the detailed events below, not here
      const combinedRight = rightParts.join('  ');
      
      // Ensure the combined right label fits within the header box width
      const maxCombinedWidth = headerScale - 4; // Account for padding and tree prefix
      let displayCombined = combinedRight;
      if (displayCombined.length > maxCombinedWidth) {
        // Truncate and add ellipsis if too long
        displayCombined = displayCombined.substring(0, maxCombinedWidth - 3) + '...';
      }
      
      console.error(`│${markerLineLeft}  │ ${'└── '}${displayCombined}`);
    }
    sortedEvents.forEach((ev, index) => {
      const eventTime = new Date(ev.time).getTime();
      const relativeStart = Math.max(0, Math.min(eventTime, latestEnd) - earliestStart);
      // Clamp column to [0, headerScale-1]
      const col = Math.floor((relativeStart / totalDuration) * headerScale);
      const clampedCol = Math.max(0, Math.min(col, Math.max(0, headerScale - 1)));
      const padding = ' '.repeat(clampedCol);
      const markerChar = ev.type === 'merged' ? '◆' : '▲';
      const marker = ev.type === 'merged' ? greenText(markerChar) : yellowText(markerChar);
      const remaining = ' '.repeat(Math.max(0, headerScale - clampedCol - 1));
      const isLast = index === sortedEvents.length - 1;
      const treePrefix = isLast ? '└── ' : '├── ';
      const timeStr = new Date(ev.time).toLocaleTimeString();
      let rightLabel;
      if (ev.type === 'merged') {
        const who = ev.mergedBy ? makeClickableLink(`https://github.com/${ev.mergedBy}`, ev.mergedBy) : 'merged';
        const timeLink = ev.url ? makeClickableLink(ev.url, timeStr) : timeStr;
        rightLabel = greenText(`merged by ${who} (${timeLink})`);
      } else {
        const who = ev.reviewer ? makeClickableLink(`https://github.com/${ev.reviewer}`, ev.reviewer) : 'approved';
        const timeLink = ev.url ? makeClickableLink(ev.url, timeStr) : timeStr;
        rightLabel = yellowText(`${who} (${timeLink})`);
      }
      
      // Note: Removed truncation to show full information with clickable links
      
      console.error(`│${padding}${marker}${remaining}  │ ${treePrefix}${rightLabel}`);
    });
  }

  // Timeline legend with colors + review markers (footer box)
  // Footer box top border (same inner width as the header box: headerScale + 2)
  console.error('┌' + '─'.repeat(headerScale + 2) + '┐');
  const jobCount = timeline.length;
  const wallTimeSec = (latestEnd - earliestStart) / 1000;
  const footerText = `Timeline: ${startTimeFormatted} → ${endTimeFormatted} • ${humanizeTime(wallTimeSec)} • ${jobCount} jobs`;
  // Footer content line ensures total inner width is headerScale + 2
  const footerInnerWidth = headerScale + 2; // includes the leading space we add below
  const footerLine = ` ${footerText}`;
  const footerPadding = ' '.repeat(Math.max(0, footerInnerWidth - footerLine.length));
  console.error(`│${footerLine}${footerPadding}│`);
  // Note: Summary information is printed by the caller above, not duplicated here
  
  // Calculate values needed for legend (not for summary display)
  const runsCount = metrics.totalRuns || 0;
  const computeMs = timeline.reduce((sum, j) => sum + Math.max(0, j.endTime - j.startTime), 0);
  const approvalsCount = (reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged').length;
  const hasMerged = (reviewEvents || []).some(ev => ev.type === 'merged');
  
  // Merge per-run summary into the footer if provided by caller (printed by caller right after this box)
  // Caller prints a concise "Summary — runs: … • wall: … • compute: … • approvals: … • merged: …"
  
  // Legend row
  const baseLegend = `Legend: ${greenText('█ Success')}  ${redText('█ Failed')}  ${blueText('▒ Pending/Running')}  ${grayText('░ Cancelled/Skipped')}`;
  const markersLegend = `${approvalsCount > 0 ? '  ' + yellowText(`▲ approvals`) : ''}${hasMerged ? '  ' + greenText('◆ merged') : ''}`;
  let legendLine = baseLegend + markersLegend;
  const legendInnerWidth = headerScale + 2;
  let legendContent = ` ${legendLine}`;
  if (legendContent.length > legendInnerWidth) legendContent = legendContent.slice(0, legendInnerWidth);
  const legendPadding = ' '.repeat(Math.max(0, legendInnerWidth - legendContent.length));
  console.error(`│${legendContent}${legendPadding}│`);
  console.error('└' + '─'.repeat(headerScale + 2) + '┘');
  
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
  
  // (Dropped aggregated bottleneck wall-time percentage to reduce confusion)
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
function generateHighLevelTimeline(sortedResults, globalEarliestTime, globalLatestTime) {
  const scale = 80;
  
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

    // Prepare bar with bars + overlayed review markers (no per-user list here)
    const barChars = Array(barLength).fill('█');
    let approvalCount = 0;
    let mergedBy = null;
    let mergedTimeMs = null;
    if (result.reviewEvents && result.reviewEvents.length > 0) {
      result.reviewEvents.forEach(event => {
        const eventTime = new Date(event.time).getTime();
        const column = Math.floor(((eventTime - timelineEarliestTime) / totalDuration) * scale);
        const offset = column - startPos;
        const clampedOffset = Math.min(Math.max(offset, 0), Math.max(0, barLength - 1));
        if (event.type === 'merged') {
          barChars[clampedOffset] = '◆';
          mergedBy = event.mergedBy || mergedBy || null;
          mergedTimeMs = eventTime;
        } else {
          barChars[clampedOffset] = '▲';
          approvalCount++;
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
    // Compact suffix to avoid overwhelming inline labels; detailed list printed later
    const suffixParts = [];
    if (approvalCount > 0) suffixParts.push(yellowText(`▲ ${approvalCount}`));
    // Note: Merge information is shown in the detailed timeline, not here
    const markerLabel = suffixParts.length > 0 ? ' ' + suffixParts.join('  ') : '';

    console.error(`│${padding}${coloredBar}${remaining}  │ ${coloredLink}${markerLabel}`);
  });
  
  console.error('└' + '─'.repeat(scale + 2) + '┘');
}

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

async function outputCombinedResults(analysisData, combinedMetrics, perfettoFile, openInPerfetto = false) {
  if (perfettoFile) {
    console.error(`\n✅ Generated ${analysisData.traceEvents.length} trace events • Open in Perfetto.dev for analysis`);
  } else {
          console.error(`\n✅ Generated ${analysisData.traceEvents.length} trace events • Use --perfetto=<filename> to save trace for Perfetto.dev analysis`);
  }
  
  console.error(`\n${'='.repeat(80)}`);
  console.error(`📊 ${makeClickableLink('https://ui.perfetto.dev', 'GitHub Actions Performance Report - Multi-URL Analysis')}`);
  console.error(`${'='.repeat(80)}`);
  
  console.error(`Analysis Summary: ${analysisData.urlCount} URLs • ${combinedMetrics.totalRuns} runs • ${combinedMetrics.totalJobs} jobs • ${combinedMetrics.totalSteps} steps`);
  console.error(`Success Rate: ${combinedMetrics.successRate}% workflows, ${combinedMetrics.jobSuccessRate}% jobs • Peak Concurrency: ${combinedMetrics.maxConcurrency}`);
  
  const allPendingJobs = [];
  analysisData.results.forEach(result => {
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
  
      const sortedResults = [...analysisData.results].sort((a, b) => a.earliestTime - b.earliestTime);
    if (analysisData.urlCount > 1) {
    console.error(`\n${makeClickableLink('https://uiperfetto.dev', 'Combined Analysis')}:`);
    console.error(`\nIncluded URLs (ordered by start time):`);
    sortedResults.forEach((result, index) => {
      const repoUrl = `https://github.com/${result.owner}/${result.repo}`;
      if (result.type === 'pr') {
        console.error(`  ${index + 1}. ${makeClickableLink(result.displayUrl, result.displayName)} (${result.branchName}) - ${makeClickableLink(repoUrl, `${result.owner}/${result.repo}`)}`);
      } else {
        console.error(`  ${index + 1}. ${makeClickableLink(result.displayUrl, result.displayName)} - ${makeClickableLink(repoUrl, `${result.owner}/${result.repo}`)}`);
      }
    });
    console.error(`\nCombined Pipeline Timeline:`);
    generateHighLevelTimeline(sortedResults, analysisData.earliestTime, analysisData.latestTime);
  }

      const commitAggregates = analysisData.results
    .filter(r => r.type === 'commit')
    .map(r => ({
      name: r.displayName,
      urlIndex: r.urlIndex,
      totalRunsForCommit: r.allCommitRunsCount ?? r.metrics.totalRuns ?? 0,
      totalComputeMsForCommit: r.allCommitRunsComputeMs ?? 0
    }));
  if (commitAggregates.length > 0) {
    console.error(`\nCommit Runs (all runs for the commit head SHA):`);
    commitAggregates.forEach(agg => {
      const computeDisplay = humanizeTime((agg.totalComputeMsForCommit || 0) / 1000);
      console.error(`  [${agg.urlIndex + 1}] ${agg.name}: runs=${agg.totalRunsForCommit}, compute=${computeDisplay}`);
    });
  }

  // Summary of runs per URL with compute, wall time, and approvals
  console.error(`\nRun Summary:`);
      analysisData.results.forEach(result => {
      const runsCount = result.metrics?.totalRuns ?? 0;
    const jobs = result.metrics?.jobTimeline ?? [];
    const computeMs = jobs.reduce((sum, j) => sum + Math.max(0, j.endTime - j.startTime), 0);
    let wallMs = 0;
    if (jobs.length > 0) {
      const start = Math.min(...jobs.map(j => j.startTime));
      const end = Math.max(...jobs.map(j => j.endTime));
      wallMs = Math.max(0, end - start);
    }
    const approvals = (result.reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged').length;
    const merged = (result.reviewEvents || []).some(ev => ev.type === 'merged');
    const line = `  [${result.urlIndex + 1}] ${result.displayName}: runs=${runsCount}, wall=${humanizeTime(wallMs/1000)}, compute=${humanizeTime(computeMs/1000)}, approvals=${approvals}, merged=${merged ? 'yes' : 'no'}`;
    console.error(line);
  });

  // Pre-commit runs (created before commit timestamp) summary when commit URL was included
      const commitResults = analysisData.results.filter(r => r.type === 'commit');
  if (commitResults.length > 0) {
    console.error(`\nPre-commit Runs (created before commit time):`);
    for (const result of commitResults) {
      // We don't have raw runs list here; compute approximation from metrics timeline
      const commitTimeMs = result.earliestTime; // For commit, earliestTime aligns with run timeline start baseline
      const preJobs = (result.metrics?.jobTimeline || []).filter(j => j.startTime < commitTimeMs);
      if (preJobs.length === 0) {
        console.error(`  [${result.urlIndex + 1}] ${result.displayName}: none`);
        continue;
      }
      const preComputeMs = preJobs.reduce((s, j) => s + Math.max(0, Math.min(j.endTime, commitTimeMs) - j.startTime), 0);
      console.error(`  [${result.urlIndex + 1}] ${result.displayName}: compute=${humanizeTime(preComputeMs/1000)} across ${preJobs.length} jobs (prior activity)`);
    }
  }
  
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
  
      analysisData.results.forEach((result, index) => {
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
      // Concise per-run summary line
      const computeMs = timeline.reduce((sum, j) => sum + Math.max(0, j.endTime - j.startTime), 0);
      const approvals = (result.reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged').length;
      const merged = (result.reviewEvents || []).some(ev => ev.type === 'merged');
      console.error(`  Summary — runs: ${result.metrics.totalRuns} • wall: ${wallTimeDisplay} • compute: ${humanizeTime(computeMs/1000)} • approvals: ${approvals} • merged: ${merged ? 'yes' : 'no'}`);
      // Note: Review and merge events are shown in the timeline visualization below, not duplicated here
      // if (result.reviewEvents && result.reviewEvents.length > 0) {
      //   const sortedEvents = [...result.reviewEvents].sort((a, b) => new Date(a.time) - new Date(b.time));
      //   sortedEvents.forEach(ev => {
      //     const timeStr = new Date(ev.time).toLocaleTimeString();
      //     const timeLink = makeClickableLink(ev.url || result.displayUrl, timeStr);
      //     if (ev.type === 'shippit' && ev.reviewer) {
      //       const userLink = makeClickableLink(`https://github.com/${ev.reviewer}`, ev.reviewer);
      //       console.error(`  ${yellowText(`▲ ${userLink}`)} ${grayText(`(${timeLink})`)}`);
      //     }
      //     if (ev.type === 'merged') {
      //       if (ev.mergedBy) {
      //       const userLink = makeClickableLink(`https://github.com/${ev.mergedBy}`, ev.mergedBy);
      //       console.error(`  ${yellowText(`◆ merged by ${userLink}`)} ${grayText(`(${timeLink})`)}`);
      //       } else {
      //         console.error(`  ${greenText('◆ merged')} ${grayText(`(${timeLink})`)}`);
      //       }
      //     }
      //   });
      // }
    } else {
      const headerText = `[${index + 1}] ${result.displayName} (${result.metrics.totalJobs} jobs)`;
      const headerLink = makeClickableLink(result.displayUrl, headerText);
      console.error(`\n${headerLink}:`);
    }
    generateTimelineVisualization(result.metrics, result.displayUrl, result.urlIndex, result.reviewEvents || []);
  });
  
  // Generate combined trace metadata
      const traceTitle = `GitHub Actions: Multi-URL Analysis (${analysisData.urlCount} URLs)`;
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
    const renormalizedTraceEvents = analysisData.traceEvents.map(event => {
      if (event.ts !== undefined) {
        // Find the URL-specific earliest time for this event
        const eventUrlIndex = event.args?.url_index || 1;
        const eventSource = event.args?.source_url;
        const urlResult = analysisData.results.find(result => 
          result.urlIndex === eventUrlIndex - 1 || 
          result.displayUrl === eventSource
        );
        
        if (urlResult) {
          // Convert back to absolute time, then normalize against global earliest time
          const absoluteTime = event.ts / 1000 + urlResult.earliestTime; // Convert from microseconds back to milliseconds, add URL earliest time
          const renormalizedTime = (absoluteTime - analysisData.earliestTime) * 1000; // Normalize against global earliest time, convert to microseconds
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
        url_count: analysisData.urlCount,
        total_runs: combinedMetrics.totalRuns,
        total_jobs: combinedMetrics.totalJobs,
        success_rate: `${combinedMetrics.successRate}%`,
        total_events: analysisData.traceEvents.length,
        urls: analysisData.results.map((result, index) => ({
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
