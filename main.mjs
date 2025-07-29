import fs, { writeFileSync } from 'fs';
import url from 'url';

const GITHUB_TOKEN = process.argv[3];
if (!GITHUB_TOKEN) {
  console.error('Usage: node script.mjs <pr_url> <token>');
  process.exit(1);
}

// Helper functions
function parsePRUrl(prUrl) {
  const parsed = new URL(prUrl);
  const pathParts = parsed.pathname.split('/').filter(Boolean);
  if (pathParts.length !== 4 || pathParts[2] !== 'pull') {
    console.error('Invalid PR URL');
    process.exit(1);
  }
  return {
    owner: pathParts[0],
    repo: pathParts[1],
    prNumber: pathParts[3]
  };
}

async function fetchWithAuth(url) {
  const headers = {
    Authorization: `token ${GITHUB_TOKEN}`,
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

function extractPRInfo(prData) {
  if (!prData.head || !prData.base) {
    console.error('Invalid PR response - missing head or base information');
    process.exit(1);
  }
  return {
    branchName: prData.head.ref,
    headSha: prData.head.sha
  };
}

async function fetchWorkflowRuns(baseUrl, headSha) {
  const commitRunsUrl = `${baseUrl}/actions/runs?head_sha=${headSha}&per_page=100`;
  return await fetchWithPagination(commitRunsUrl);
}

async function fetchWithPagination(url) {
  const headers = {
    Authorization: `token ${GITHUB_TOKEN}`,
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
    shortestJob: { name: '', duration: Infinity }
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

async function processWorkflowRun(run, runIndex, processId, earliestTime, metrics, traceEvents, jobStartTimes, jobEndTimes) {
  console.error(`Processing run ${run.id}: ${run.name || 'unnamed'}...`);
  
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
  const jobs = await fetchWithPagination(jobsUrl);
  
  // Calculate run timing
  const runStartTs = Math.max(0, (new Date(run.created_at).getTime() - earliestTime) * 1000);
  
  // Find latest job completion for run end time
  let runEndTs = Math.max(runStartTs + 1000, (new Date(run.updated_at).getTime() - earliestTime) * 1000);
  for (const job of jobs) {
    if (job.completed_at) {
      const jobEndTime = (new Date(job.completed_at).getTime() - earliestTime) * 1000;
      runEndTs = Math.max(runEndTs, jobEndTime);
    }
  }
  
  const runDurationMs = (runEndTs - runStartTs) / 1000;
  metrics.totalDuration += runDurationMs;
  
  // Add process metadata for this workflow
  traceEvents.push({
    name: 'process_name',
    ph: 'M',
    pid: processId,
    args: { name: `${run.name || `Run ${run.id}`} (${run.status})` }
  });
  
  // Thread 1: Workflow overview with jobs timeline
  const workflowThreadId = 1;
  addThreadMetadata(traceEvents, processId, workflowThreadId, `📋 Workflow Overview`, 0);
  
  // Add workflow run event on overview thread
  const workflowUrl = `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}`;
  traceEvents.push({
    name: `Workflow: ${run.name || `Run ${run.id}`}`,
    ph: 'X',
    ts: runStartTs,
    dur: runEndTs - runStartTs,
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
    }
  });
  
  // Process jobs (each job gets its own thread with steps)
  for (const [jobIndex, job] of jobs.entries()) {
    const jobThreadId = jobIndex + 10; // Start from thread 10 to keep workflow overview first
    await processJob(job, jobIndex, run, jobThreadId, processId, earliestTime, runStartTs, runEndTs, metrics, traceEvents, jobStartTimes, jobEndTimes);
  }
}

async function processJob(job, jobIndex, run, jobThreadId, processId, earliestTime, runStartTs, runEndTs, metrics, traceEvents, jobStartTimes, jobEndTimes) {
  if (!job.started_at || !job.completed_at) {
    console.error(`  Skipping job ${job.name} - missing timing data`);
    return;
  }
  
  // Update job metrics
  metrics.totalJobs++;
  if (job.status !== 'completed' || job.conclusion !== 'success') {
    metrics.failedJobs++;
  }
  
  // Calculate job timing
  const jobStartTs = Math.max(runStartTs, (new Date(job.started_at).getTime() - earliestTime) * 1000);
  let jobEndTs = Math.max(jobStartTs + 1000, (new Date(job.completed_at).getTime() - earliestTime) * 1000);
  const jobDurationMs = (jobEndTs - jobStartTs) / 1000;
  
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
  
  // Add thread metadata for this job
  const jobIcon = job.conclusion === 'success' ? '✅' : job.conclusion === 'failure' ? '❌' : '⏸️';
  addThreadMetadata(traceEvents, processId, jobThreadId, `${jobIcon} ${job.name}`, jobIndex + 10);
  
  // Add job event (this shows the overall job duration)
  const jobUrl = job.html_url || `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}/job/${job.id}`;
  traceEvents.push({
    name: `Job: ${job.name}`,
    ph: 'X',
    ts: jobStartTs,
    dur: jobEndTs - jobStartTs,
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
      job_id: job.id
    }
  });
  
  // Process steps on the same thread as the job
  for (const step of job.steps) {
    processStep(step, job, run, jobThreadId, processId, earliestTime, jobStartTs, jobEndTs, metrics, traceEvents);
  }
}

function processStep(step, job, run, jobThreadId, processId, earliestTime, jobStartTs, jobEndTs, metrics, traceEvents) {
  if (!step.started_at || !step.completed_at) return;
  
  // Update step metrics
  metrics.totalSteps++;
  if (step.conclusion === 'failure') {
    metrics.failedSteps++;
  }
  
  // Calculate step timing
  const stepStartTs = Math.max(jobStartTs, (new Date(step.started_at).getTime() - earliestTime) * 1000);
  let stepEndTs = Math.max(stepStartTs + 500, (new Date(step.completed_at).getTime() - earliestTime) * 1000);
  if (stepEndTs > jobEndTs) {
    stepEndTs = Math.max(stepStartTs + 500, jobEndTs);
  }
  const stepDurationMs = (stepEndTs - stepStartTs) / 1000;
  
  // Validate timing
  if (stepStartTs >= stepEndTs || stepDurationMs <= 0) return;
  
  // Get step icon and category
  const stepIcon = getStepIcon(step.name, step.conclusion);
  const stepCategory = categorizeStep(step.name);
  
  // Update metrics
  metrics.stepDurations.push({
    name: `${stepIcon} ${step.name}`,
    duration: stepDurationMs
  });
  
  // Add step event  
  const stepUrl = job.html_url || `https://github.com/${run.repository.owner.login}/${run.repository.name}/actions/runs/${run.id}/job/${job.id}`;
  traceEvents.push({
    name: `${stepIcon} ${step.name}`,
    ph: 'X',
    ts: stepStartTs,
    dur: stepEndTs - stepStartTs,
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
      step_number: step.number
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

function generateConcurrencyCounters(jobStartTimes, jobEndTimes, traceEvents) {
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
    
    traceEvents.push({
      name: 'Concurrent Jobs',
      ph: 'C',
      ts: event.ts,
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

function generateRecommendations(metrics, finalMetrics) {
  const recommendations = [];
  

  if (metrics.longestJob.duration > finalMetrics.avgJobDuration * 3 && finalMetrics.avgJobDuration > 0) {
    recommendations.push({
      title: "Investigate Slowest Job",
      description: `"${metrics.longestJob.name}" takes ${(metrics.longestJob.duration/1000).toFixed(1)}s`,
      impact: "MEDIUM"
    });
  }
  
  return recommendations;
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

function makeClickableLink(url, text = null) {
  // ANSI escape sequence for clickable links (OSC 8)
  // Format: \u001b]8;;URL\u0007TEXT\u001b]8;;\u0007
  const displayText = text || url;
  return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
}

function outputResults(owner, repo, prNumber, branchName, headSha, metrics, recommendations, traceEvents) {
  // Simple completion message
  console.error(`\n✅ Generated ${traceEvents.length} trace events • Open in Perfetto.dev for analysis`);
  
  // Professional summary report
  console.error(`\n${'='.repeat(60)}`);
  const repoUrl = `https://github.com/${owner}/${repo}`;
  console.error(`📊 ${makeClickableLink(repoUrl, 'GitHub Actions Performance Report')}`);
  console.error(`${'='.repeat(60)}`);
  console.error(`Repository: ${makeClickableLink(repoUrl, `${owner}/${repo}`)}`);
  const headerPrUrl = `https://github.com/${owner}/${repo}/pull/${prNumber}`;
  console.error(`Pull Request: ${makeClickableLink(headerPrUrl, `#${prNumber}`)} (${branchName})`);
  const headerCommitUrl = `https://github.com/${owner}/${repo}/commit/${headSha}`;
  console.error(`Commit: ${makeClickableLink(headerCommitUrl, headSha.substring(0, 8))}`);
  console.error(`Analysis: ${metrics.totalRuns} runs • ${metrics.totalJobs} jobs (peak concurrency: ${metrics.maxConcurrency}) • ${metrics.totalSteps} steps`);
  console.error(`Success Rate: ${metrics.successRate}% workflows, ${metrics.jobSuccessRate}% jobs`);
  
  // Generate and show slowest jobs analysis
  const repoActionsUrl = `https://github.com/${owner}/${repo}/actions`;
  console.error(`\n${makeClickableLink(repoActionsUrl, 'Performance Analysis')}:`);
  
  const slowJobs = analyzeSlowJobs(metrics, 5);
  if (slowJobs.length > 0) {
    console.error(`\n${makeClickableLink(repoActionsUrl, 'Slowest Jobs')}:`);
    slowJobs.forEach((job, i) => {
      const jobLink = job.url ? makeClickableLink(job.url, job.name) : job.name;
      console.error(`  ${i + 1}. ${(job.duration / 1000).toFixed(1)}s - ${jobLink}`);
    });
  }

  const slowSteps = analyzeSlowSteps(metrics, 5);
  if (slowSteps.length > 0) {
    console.error(`\n${makeClickableLink(repoActionsUrl, 'Slowest Steps')}:`);
    slowSteps.forEach((step, i) => {
      console.error(`  ${i + 1}. ${(step.duration / 1000).toFixed(1)}s - ${step.name}`);
    });
  }

  if (recommendations.length > 0) {
    console.error(`\n${makeClickableLink(repoActionsUrl, 'Recommendations')}:`);
    recommendations.forEach((rec, i) => {
      console.error(`  ${i + 1}. ${rec.title} (${rec.impact} impact)`);
      console.error(`     ${rec.description}`);
    });
  }
  
  console.error(`\nLinks:`);
  const prUrl = `https://github.com/${owner}/${repo}/pull/${prNumber}`;
  const actionsUrl = `https://github.com/${owner}/${repo}/actions`;
  const commitUrl = `https://github.com/${owner}/${repo}/commit/${headSha}`;
  const perfettoUrl = `https://perfetto.dev`;
  
  console.error(`• PR: ${makeClickableLink(prUrl)}`);
  console.error(`• Actions: ${makeClickableLink(actionsUrl)}`);
  console.error(`• Commit: ${makeClickableLink(commitUrl)}`);
  console.error(`• Trace Analysis: ${makeClickableLink(perfettoUrl)}`);
  console.error(`${'='.repeat(60)}`);
  
  // Use already generated performance analysis data

  // Output JSON for Perfetto
  const output = {
    displayTimeUnit: 'ms',
    traceEvents: traceEvents.sort((a, b) => a.ts - b.ts),
    otherData: {
      pr_number: prNumber,
      head_sha: headSha,
      branch_name: branchName,
      total_runs: metrics.totalRuns,
      success_rate: `${metrics.successRate}%`,
      total_events: traceEvents.length,
      performance_analysis: {
        slowest_jobs: slowJobs.map(job => ({
          name: job.name,
          duration_seconds: (job.duration / 1000).toFixed(1),
          url: job.url
        })),
        slowest_steps: slowSteps.map(step => ({
          name: step.name,
          duration_seconds: (step.duration / 1000).toFixed(1)
        }))
      },
      github_urls: {
        pr: `https://github.com/${owner}/${repo}/pull/${prNumber}`,
        actions: `https://github.com/${owner}/${repo}/actions`,
        commit: `https://github.com/${owner}/${repo}/commit/${headSha}`,
        repository: `https://github.com/${owner}/${repo}`
      }
    }
  };
  
  try {
    console.log(JSON.stringify(output, null, 2));
  } catch (error) {
    console.error('JSON validation failed:', error);
    process.exit(1);
  }
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

async function main() {
  // Parse PR URL and validate inputs
  const prUrl = process.argv[2];
  if (!prUrl || !GITHUB_TOKEN) {
    console.error('Usage: node script.mjs <pr_url> <token>');
    process.exit(1);
  }

  const { owner, repo, prNumber } = parsePRUrl(prUrl);
  const baseUrl = `https://api.github.com/repos/${owner}/${repo}`;
  
  // Fetch PR and workflow data
  const analyzingPrUrl = `https://github.com/${owner}/${repo}/pull/${prNumber}`;
  console.error(`Analyzing ${makeClickableLink(`https://github.com/${owner}/${repo}`, `${owner}/${repo}`)} ${makeClickableLink(analyzingPrUrl, `PR #${prNumber}`)}...`);
  
  const prData = await fetchWithAuth(`${baseUrl}/pulls/${prNumber}`);
  const { branchName, headSha } = extractPRInfo(prData);
  
  const allRuns = await fetchWorkflowRuns(baseUrl, headSha);
  if (allRuns.length === 0) {
    const noPrUrl = `https://github.com/${owner}/${repo}/pull/${prNumber}`;
    console.error(`No workflow runs found for ${makeClickableLink(noPrUrl, 'this PR')}`);
    process.exit(1);
  }

  // Initialize metrics and trace data
  const metrics = initializeMetrics();
  const traceEvents = [];
  const jobStartTimes = [];
  const jobEndTimes = [];
  let earliestTime = Infinity;
  let processId = 1;

  // Find earliest timestamp for normalization
  earliestTime = findEarliestTimestamp(allRuns);

  // Process each workflow run (each run gets its own process)
  for (const [runIndex, run] of allRuns.entries()) {
    const workflowProcessId = runIndex + 1;
    await processWorkflowRun(run, runIndex, workflowProcessId, earliestTime, metrics, traceEvents, jobStartTimes, jobEndTimes);
  }

  // Generate concurrency counter events (use process 1 for global metrics)
  generateConcurrencyCounters(jobStartTimes, jobEndTimes, traceEvents);

  // Calculate final metrics
  const finalMetrics = calculateFinalMetrics(metrics, allRuns.length, jobStartTimes, jobEndTimes);

  // Generate recommendations
  const recommendations = generateRecommendations(metrics, finalMetrics);

  // Output results
  outputResults(owner, repo, prNumber, branchName, headSha, finalMetrics, recommendations, traceEvents);
}

main();
