/**
 * Utility Functions Module
 * Contains helper functions for GitHub URL parsing, metrics calculation, and workflow processing
 */

import { makeClickableLink } from './visualization.mjs';
import { calculateMaxConcurrency } from './workflow.mjs';

/**
 * Create a GitHub API session with authentication
 */
export const createSession = (token = process.env.GITHUB_TOKEN) => ({
  githubToken: token
});

/**
 * Parse GitHub URLs to extract owner, repo, type, and identifier
 */
export function parseGitHubUrl(url) {
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
 * Extract PR information from PR data
 */
export function extractPRInfo(prData, prUrl = null) {
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

/**
 * Find the earliest timestamp from workflow runs
 */
export function findEarliestTimestamp(allRuns) {
  let earliest = Infinity;
  // Simple approximation - use the earliest run creation time
  for (const run of allRuns) {
    const runTime = new Date(run.created_at).getTime();
    earliest = Math.min(earliest, runTime);
  }
  return earliest;
}

/**
 * Calculate combined metrics across multiple URL results
 */
export function calculateCombinedMetrics(urlResults, totalRuns, allJobStartTimes, allJobEndTimes) {
  const combined = {
    totalRuns,
    totalJobs: urlResults.reduce((sum, result) => sum + result.metrics.totalJobs, 0),
    totalSteps: urlResults.reduce((sum, result) => sum + result.metrics.totalSteps, 0),
    successRate: calculateCombinedSuccessRate(urlResults),
    jobSuccessRate: calculateCombinedJobSuccessRate(urlResults),
    maxConcurrency: calculateMaxConcurrency(allJobStartTimes, allJobEndTimes),
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
 * Calculate combined success rate across multiple URL results
 */
export function calculateCombinedSuccessRate(urlResults) {
  const totalSuccessful = urlResults.reduce((sum, result) => {
    const rate = parseFloat(result.metrics.successRate);
    const normalized = Number.isFinite(rate) ? rate : 0;
    return sum + (result.metrics.totalRuns * normalized / 100);
  }, 0);
  const totalRuns = urlResults.reduce((sum, result) => sum + result.metrics.totalRuns, 0);
  return totalRuns > 0 ? (totalSuccessful / totalRuns * 100).toFixed(1) : '0.0';
}

/**
 * Calculate combined job success rate across multiple URL results
 */
export function calculateCombinedJobSuccessRate(urlResults) {
  const totalSuccessfulJobs = urlResults.reduce((sum, result) => {
    const rate = parseFloat(result.metrics.jobSuccessRate);
    const normalized = Number.isFinite(rate) ? rate : 0;
    return sum + (result.metrics.totalJobs * normalized / 100);
  }, 0);
  const totalJobs = urlResults.reduce((sum, result) => sum + result.metrics.totalJobs, 0);
  return totalJobs > 0 ? (totalSuccessfulJobs / totalJobs * 100).toFixed(1) : '0.0';
}
