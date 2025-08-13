/**
 * Workflow Processing Module
 * Handles workflow run processing, job analysis, and metrics calculation
 */

import { fetchWithPagination } from './fetching.mjs';

/**
 * Process a single workflow run and extract metrics and trace events
 */
export async function processWorkflowRun(run, runIndex, processId, earliestTime, metrics, traceEvents, jobStartTimes, jobEndTimes, owner, repo, identifier, urlIndex = 0, currentUrlResult = null, session = null) {  
  metrics.totalRuns++;
  if (run.status === 'completed' && run.conclusion === 'success') {
    metrics.successfulRuns++;
  } else {
    metrics.failedRuns++;
  }
  
  const baseUrl = `https://api.github.com/repos/${run.repository.owner.login}/${run.repository.name}`;
  const jobsUrl = `${baseUrl}/actions/runs/${run.id}/jobs?per_page=100`;
  const jobs = await fetchWithPagination(jobsUrl, session);
  
  const absoluteRunStartMs = new Date(run.created_at).getTime();
  const absoluteRunEndMs = new Date(run.updated_at).getTime();
  const runStartTs = absoluteRunStartMs; // Keep in milliseconds for Perfetto
  let runEndTs = Math.max(runStartTs + 1, absoluteRunEndMs); // Ensure minimum 1ms duration
  
  for (const job of jobs) {
    if (job.started_at && job.completed_at) {
      const jobStartMs = new Date(job.started_at).getTime();
      const jobEndMs = new Date(job.completed_at).getTime();
      
      if (isFinite(jobStartMs) && isFinite(jobEndMs) && jobEndMs > jobStartMs) {
        const jobDuration = jobEndMs - jobStartMs;
        const jobStartTs = (jobStartMs - earliestTime) * 1000; // Convert to microseconds
        const jobEndTs = (jobEndMs - earliestTime) * 1000;
        
        // Store job timing data
        jobStartTimes.push(jobStartMs);
        jobEndTimes.push(jobEndMs);
        
        // Update metrics
        metrics.totalJobs++;
        
        // Add job to timeline regardless of conclusion (for visualization)
        metrics.jobTimeline.push({
          name: job.name,
          startTime: jobStartMs,
          endTime: jobEndMs,
          url: job.html_url,
          duration: jobDuration,
          conclusion: job.conclusion,
          status: job.status
        });
        
        if (job.conclusion === 'success') {
          metrics.totalDuration += jobDuration;
          metrics.jobDurations.push(jobDuration);
          metrics.jobNames.push(job.name);
          metrics.jobUrls.push(job.html_url);
          
          // Track longest/shortest jobs
          if (jobDuration > metrics.longestJob.duration) {
            metrics.longestJob = { name: job.name, duration: jobDuration };
          }
          if (jobDuration < metrics.shortestJob.duration) {
            metrics.shortestJob = { name: job.name, duration: jobDuration };
          }
          
          // Process job steps
          if (job.steps) {
            for (const step of job.steps) {
              if (step.started_at && step.completed_at) {
                const stepStartMs = new Date(step.started_at).getTime();
                const stepEndMs = new Date(step.completed_at).getTime();
                
                if (isFinite(stepStartMs) && isFinite(stepEndMs) && stepEndMs > stepStartMs) {
                  const stepDuration = stepEndMs - stepStartMs;
                  const stepStartTs = (stepStartMs - earliestTime) * 1000;
                  const stepEndTs = (stepEndMs - earliestTime) * 1000;
                  
                  metrics.totalSteps++;
                  if (step.conclusion === 'success') {
                    metrics.stepDurations.push(stepDuration);
                  } else {
                    metrics.failedSteps++;
                  }
                  
                  // Add step to trace events
                  traceEvents.push({
                    name: step.name,
                    ph: 'X',
                    pid: processId,
                    tid: job.id,
                    ts: stepStartTs,
                    dur: stepEndTs - stepStartTs,
                    args: {
                      step_name: step.name,
                      step_conclusion: step.conclusion,
                      job_name: job.name,
                      run_id: run.id,
                      url_index: urlIndex + 1,
                      source_url: currentUrlResult?.displayUrl || '',
                      source_type: currentUrlResult?.type || '',
                      source_identifier: currentUrlResult?.identifier || ''
                    }
                  });
                }
              }
            }
          }
          
          // Add job to trace events
          traceEvents.push({
            name: job.name,
            ph: 'X',
            pid: processId,
            tid: job.id,
            ts: jobStartTs,
            dur: jobEndTs - jobStartTs,
            args: {
              job_name: job.name,
              job_conclusion: job.conclusion,
              run_id: run.id,
              url_index: urlIndex + 1,
              source_url: currentUrlResult?.displayUrl || '',
              source_type: currentUrlResult?.type || '',
              source_identifier: currentUrlResult?.identifier || ''
            }
          });
        } else {
          metrics.failedJobs++;
        }
        
        // Track runner type
        if (job.runner_name) {
          metrics.runnerTypes.add(job.runner_name);
        }
      }
    }
  }
  
  // Add run to trace events
  traceEvents.push({
    name: `Run ${run.id}`,
    ph: 'X',
    pid: processId,
    tid: run.id,
    ts: (runStartTs - earliestTime) * 1000,
    dur: (runEndTs - earliestTime) * 1000,
    args: {
      run_id: run.id,
      run_status: run.status,
      run_conclusion: run.conclusion,
      url_index: urlIndex + 1,
      source_url: currentUrlResult?.displayUrl || '',
      source_type: currentUrlResult?.type || '',
      source_identifier: currentUrlResult?.identifier || ''
    }
  });
}

/**
 * Calculate final metrics for a URL
 */
export function calculateFinalMetrics(metrics, totalRuns, jobStartTimes, jobEndTimes) {
  const finalMetrics = { ...metrics };
  
  // Calculate success rates
  finalMetrics.successRate = totalRuns > 0 ? (metrics.successfulRuns / totalRuns * 100).toFixed(1) : '0.0';
  finalMetrics.jobSuccessRate = metrics.totalJobs > 0 ? ((metrics.totalJobs - metrics.failedJobs) / metrics.totalJobs * 100).toFixed(1) : '0.0';
  
  // Calculate max concurrency
  finalMetrics.maxConcurrency = calculateMaxConcurrency(jobStartTimes, jobEndTimes);
  
  // Sort job timeline by start time if it exists
  if (finalMetrics.jobTimeline && Array.isArray(finalMetrics.jobTimeline)) {
    finalMetrics.jobTimeline.sort((a, b) => a.startTime - b.startTime);
  }
  
  return finalMetrics;
}

/**
 * Calculate maximum concurrency across jobs
 */
export function calculateMaxConcurrency(jobStartTimes, jobEndTimes) {
  if (jobStartTimes.length === 0) return 0;
  
  let maxConcurrency = 0;
  for (let i = 0; i < jobStartTimes.length; i++) {
    const time = jobStartTimes[i];
    const concurrentJobs = jobStartTimes.filter((start, j) => 
      start <= time && jobEndTimes[j] >= time
    ).length;
    maxConcurrency = Math.max(maxConcurrency, concurrentJobs);
  }
  
  return maxConcurrency;
}

/**
 * Analyze slow jobs for optimization insights
 */
export function analyzeSlowJobs(jobTimeline) {
  if (jobTimeline.length === 0) return [];
  
  const sortedJobs = [...jobTimeline].sort((a, b) => b.duration - a.duration);
  return sortedJobs.slice(0, 5); // Top 5 slowest jobs
}

/**
 * Analyze slow steps for optimization insights
 */
export function analyzeSlowSteps(stepDurations) {
  if (stepDurations.length === 0) return [];
  
  const sortedSteps = [...stepDurations].sort((a, b) => b - a);
  return sortedSteps.slice(0, 10); // Top 10 slowest steps
}

/**
 * Find overlapping jobs for concurrency analysis
 */
export function findOverlappingJobs(jobTimeline) {
  const overlapping = [];
  
  for (let i = 0; i < jobTimeline.length; i++) {
    for (let j = i + 1; j < jobTimeline.length; j++) {
      const jobA = jobTimeline[i];
      const jobB = jobTimeline[j];
      
      if (jobA.startTime < jobB.endTime && jobB.startTime < jobA.endTime) {
        overlapping.push({
          jobA: jobA.name,
          jobB: jobB.name,
          overlapStart: Math.max(jobA.startTime, jobB.startTime),
          overlapEnd: Math.min(jobA.endTime, jobB.endTime),
          overlapDuration: Math.min(jobA.endTime, jobB.endTime) - Math.max(jobA.startTime, jobB.startTime)
        });
      }
    }
  }
  
  return overlapping.sort((a, b) => b.overlapDuration - a.overlapDuration);
}

/**
 * Categorize step names for grouping
 */
export function categorizeStep(stepName) {
  const lowerName = stepName.toLowerCase();
  
  if (lowerName.includes('test') || lowerName.includes('spec')) return 'Testing';
  if (lowerName.includes('build') || lowerName.includes('compile')) return 'Build';
  if (lowerName.includes('deploy') || lowerName.includes('release')) return 'Deployment';
  if (lowerName.includes('lint') || lowerName.includes('format')) return 'Code Quality';
  if (lowerName.includes('install') || lowerName.includes('setup') || lowerName.includes('checkout')) return 'Setup';
  if (lowerName.includes('cache') || lowerName.includes('restore')) return 'Caching';
  
  return 'Other';
}

/**
 * Get step icon for visualization
 */
export function getStepIcon(stepName) {
  const category = categorizeStep(stepName);
  
  switch (category) {
    case 'Testing': return '🧪';
    case 'Build': return '🔨';
    case 'Deployment': return '🚀';
    case 'Code Quality': return '✨';
    case 'Setup': return '⚙️';
    case 'Caching': return '💾';
    default: return '▶️';
  }
}
