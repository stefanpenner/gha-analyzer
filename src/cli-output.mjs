/**
 * CLI Output Manager
 * Centralizes all terminal output for better maintainability, testing, and previewing
 */

import { spawn } from 'child_process';
import os from 'os';
import path from 'path';
import fs from 'fs';

// Output stream management
class OutputStream {
  constructor(stream = process.stderr) {
    this.stream = stream;
    this.buffer = [];
    this.isCapturing = false;
  }

  write(text) {
    if (this.isCapturing) {
      this.buffer.push(text);
    } else {
      this.stream.write(text);
    }
  }

  startCapture() {
    this.isCapturing = true;
    this.buffer = [];
  }

  stopCapture() {
    this.isCapturing = false;
    return this.buffer.join('');
  }

  getCaptured() {
    return this.buffer.join('');
  }

  clear() {
    this.buffer = [];
  }
}

// Main CLI output manager
export class CLIOutput {
  constructor() {
    this.stdout = new OutputStream(process.stdout);
    this.stderr = new OutputStream(process.stderr);
    this.isVerbose = false;
  }

  // Configuration
  setVerbose(verbose) {
    this.isVerbose = verbose;
  }

  // Utility functions
  makeClickableLink(url, text = null) {
    const displayText = text || url;
    return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
  }

  grayText(text) {
    return `\u001b[90m${text}\u001b[0m`;
  }

  blueText(text) {
    return `\u001b[34m${text}\u001b[0m`;
  }

  yellowText(text) {
    return `\u001b[33m${text}\u001b[0m`;
  }

  redText(text) {
    return `\u001b[31m${text}\u001b[0m`;
  }

  greenText(text) {
    return `\u001b[32m${text}\u0007`;
  }

  // Progress bar output
  showProgress(spinner, urlProgress, runProgress, timing) {
    const progressLine = `${spinner} Processing: ${urlProgress}${runProgress} | ${timing}`;
    this.stderr.write(`\r\x1b[K${progressLine}`);
  }

  showProgressCompletion(totalUrls, totalTime, totalRuns) {
    const completion = [
      `✅ Analysis Complete!`,
      `╭${'─'.repeat(78)}╮`,
      `│ 🎯 Processed ${totalUrls} URLs in ${this.formatDuration(totalTime)}`,
      `│ 📈 Total workflow runs analyzed: ${totalRuns}`,
      `│ 🚀 Average time per URL: ${this.formatDuration(totalTime / totalUrls)}`,
      `╰${'─'.repeat(78)}╯`
    ].join('\n');
    
    this.stderr.write('\r\x1b[K' + completion + '\n');
  }

  // Main report output
  showTraceGeneration(traceCount, perfettoFile) {
    if (perfettoFile) {
      this.stderr.write(`\n✅ Generated ${traceCount} trace events • Open in Perfetto.dev for analysis\n`);
    } else {
      this.stderr.write(`\n✅ Generated ${traceCount} trace events • Use --perfetto=<filename> to save trace for Perfetto.dev analysis\n`);
    }
  }

  showReportHeader() {
    this.stderr.write(`\n${'='.repeat(80)}\n`);
    this.stderr.write(`📊 ${this.makeClickableLink('https://ui.perfetto.dev', 'GitHub Actions Performance Report - Multi-URL Analysis')}\n`);
    this.stderr.write(`${'='.repeat(80)}\n`);
  }

  showAnalysisSummary(urlCount, totalRuns, totalJobs, totalSteps, successRate, jobSuccessRate, maxConcurrency) {
    this.stderr.write(`Analysis Summary: ${urlCount} URLs • ${totalRuns} runs • ${totalJobs} jobs • ${totalSteps} steps\n`);
    this.stderr.write(`Success Rate: ${successRate}% workflows, ${jobSuccessRate}% jobs • Peak Concurrency: ${maxConcurrency}\n`);
  }

  showPendingJobs(pendingJobs) {
    if (pendingJobs.length === 0) return;

    this.stderr.write(`\n${this.blueText('⚠️  Pending Jobs Detected:')} ${pendingJobs.length} jobs still running\n`);
    pendingJobs.forEach((job, index) => {
      const jobLink = this.makeClickableLink(job.url, job.name);
      this.stderr.write(`  ${index + 1}. ${this.blueText(jobLink)} (${job.status}) - ${job.sourceName}\n`);
    });
    this.stderr.write(`\n  Note: Timeline shows current progress for pending jobs. Results may change as jobs complete.\n`);
  }

  showCombinedAnalysis(sortedResults) {
    if (sortedResults.length <= 1) return;

    this.stderr.write(`\n${this.makeClickableLink('https://ui.perfetto.dev', 'Combined Analysis')}:\n`);
    this.stderr.write(`\nIncluded URLs (ordered by start time):\n`);
    
    sortedResults.forEach((result, index) => {
      const repoUrl = `https://github.com/${result.owner}/${result.repo}`;
      if (result.type === 'pr') {
        this.stderr.write(`  ${index + 1}. ${this.makeClickableLink(result.displayUrl, result.displayName)} (${result.branchName}) - ${this.makeClickableLink(repoUrl, `${result.owner}/${result.repo}`)}\n`);
      } else {
        this.stderr.write(`  ${index + 1}. ${this.makeClickableLink(result.displayUrl, result.displayName)} - ${this.makeClickableLink(repoUrl, `${result.owner}/${result.repo}`)}\n`);
      }
    });
    
    this.stderr.write(`\nCombined Pipeline Timeline:\n`);
  }

  showCommitRuns(commitAggregates) {
    if (commitAggregates.length === 0) return;

    this.stderr.write(`\nCommit Runs (all runs for the commit head SHA):\n`);
    commitAggregates.forEach(agg => {
      const computeDisplay = this.humanizeTime((agg.totalComputeMsForCommit || 0) / 1000);
      this.stderr.write(`  [${agg.urlIndex + 1}] ${agg.name}: runs=${agg.totalRunsForCommit}, compute=${computeDisplay}\n`);
    });
  }

  showRunSummary(results) {
    this.stderr.write(`\nRun Summary:\n`);
    results.forEach(result => {
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
      
      const line = `  [${result.urlIndex + 1}] ${result.displayName}: runs=${runsCount}, wall=${this.humanizeTime(wallMs/1000)}, compute=${this.humanizeTime(computeMs/1000)}, approvals=${approvals}, merged=${merged ? 'yes' : 'no'}\n`;
      this.stderr.write(line);
    });
  }

  showPreCommitRuns(commitResults) {
    if (commitResults.length === 0) return;

    this.stderr.write(`\nPre-commit Runs (created before commit time):\n`);
    for (const result of commitResults) {
      const commitTimeMs = result.earliestTime;
      const preJobs = (result.metrics?.jobTimeline || []).filter(j => j.startTime < commitTimeMs);
      
      if (preJobs.length === 0) {
        this.stderr.write(`  [${result.urlIndex + 1}] ${result.displayName}: none\n`);
        continue;
      }
      
      const preComputeMs = preJobs.reduce((s, j) => s + Math.max(0, Math.min(j.endTime, commitTimeMs) - j.startTime), 0);
      this.stderr.write(`  [${result.urlIndex + 1}] ${result.displayName}: compute=${this.humanizeTime(preComputeMs/1000)} across ${preJobs.length} jobs (prior activity)\n`);
    }
  }

  showSlowestJobs(slowJobs, sortedResults, bottleneckJobs) {
    if (slowJobs.length === 0) return;

    this.stderr.write(`\nSlowest Jobs (grouped by PR/Commit):\n`);
    
    // Group jobs by their source URL
    const jobsBySource = {};
    slowJobs.forEach(job => {
      const sourceKey = job.sourceUrl;
      if (!jobsBySource[sourceKey]) {
        jobsBySource[sourceKey] = [];
      }
      jobsBySource[sourceKey].push(job);
    });
    
    // Display grouped by source
    sortedResults.forEach(result => {
      const sourceUrl = result.displayUrl;
      const jobs = jobsBySource[sourceUrl];
      if (jobs && jobs.length > 0) {
        const headerText = `[${result.urlIndex + 1}] ${result.displayName}`;
        const headerLink = this.makeClickableLink(sourceUrl, headerText);
        this.stderr.write(`\n  ${headerLink}:\n`);
        
        const sortedJobs = jobs.sort((a, b) => (b.endTime - b.startTime) - (a.endTime - b.startTime));
        sortedJobs.forEach((job, i) => {
          const duration = ((job.endTime - job.startTime) / 1000);
          const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
          const isBottleneck = bottleneckJobs.has(jobKey);
          const bottleneckIndicator = isBottleneck ? ' 🔥' : '';
          const fullText = `${i + 1}. ${this.humanizeTime(duration)} - ${job.name}${bottleneckIndicator}`;
          const jobLink = job.url ? this.makeClickableLink(job.url, fullText) : fullText;
          this.stderr.write(`    ${jobLink}\n`);
        });
      }
    });
    
    // Add explanation for bottleneck indicator
    const hasBottleneckJobs = slowJobs.some(job => {
      const jobKey = `${job.name}-${job.startTime}-${job.endTime}`;
      return bottleneckJobs.has(jobKey);
    });
    
    if (hasBottleneckJobs) {
      this.stderr.write(`\n  🔥 Bottleneck jobs - optimizing these will have the most impact on total pipeline time\n`);
    }
  }

  showPipelineTimelines(results) {
    this.stderr.write(`\n${this.makeClickableLink('https://ui.perfetto.dev', 'Pipeline Timelines')}:\n`);
    
    results.forEach((result, index) => {
      const timeline = result.metrics.jobTimeline;
      if (timeline && timeline.length > 0) {
        const earliestStart = Math.min(...timeline.map(job => job.startTime));
        const latestEnd = Math.max(...timeline.map(job => job.endTime));
        const wallTimeSec = (latestEnd - earliestStart) / 1000;
        const wallTimeDisplay = this.humanizeTime(wallTimeSec);
        const headerText = `[${index + 1}] ${result.displayName} (${wallTimeDisplay}, ${result.metrics.totalJobs} jobs)`;
        const headerLink = this.makeClickableLink(result.displayUrl, headerText);
        this.stderr.write(`\n${headerLink}:\n`);
        
        const computeMs = timeline.reduce((sum, j) => sum + Math.max(0, j.endTime - j.startTime), 0);
        const approvals = (result.reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged').length;
        const merged = (result.reviewEvents || []).some(ev => ev.type === 'merged');
        this.stderr.write(`  Summary — runs: ${result.metrics.totalRuns} • wall: ${wallTimeDisplay} • compute: ${this.humanizeTime(computeMs/1000)} • approvals: ${approvals} • merged: ${merged ? 'yes' : 'no'}\n`);
      } else {
        const headerText = `[${index + 1}] ${result.displayName} (${result.metrics.totalJobs} jobs)`;
        const headerLink = this.makeClickableLink(result.displayUrl, headerText);
        this.stderr.write(`\n${headerLink}:\n`);
      }
    });
  }

  showPerfettoSave(perfettoFile) {
    this.stderr.write(`\n💾 Perfetto trace saved to: ${perfettoFile}\n`);
  }

  // Error and warning output
  showError(message) {
    this.stderr.write(`❌ ${this.redText(message)}\n`);
  }

  showWarning(message) {
    this.stderr.write(`⚠️  ${this.yellowText(message)}\n`);
  }

  showInfo(message) {
    this.stderr.write(`ℹ️  ${message}\n`);
  }

  showSuccess(message) {
    this.stderr.write(`✅ ${this.greenText(message)}\n`);
  }

  // Utility methods
  humanizeTime(seconds) {
    if (seconds < 1) return `${(seconds * 1000).toFixed(0)}ms`;
    if (seconds < 60) return `${seconds.toFixed(1)}s`;
    if (seconds < 3600) return `${(seconds / 60).toFixed(1)}m`;
    return `${(seconds / 3600).toFixed(1)}h`;
  }

  formatDuration(ms) {
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    if (ms < 3600000) return `${(ms / 60000).toFixed(1)}m`;
    return `${(ms / 3600000).toFixed(1)}h`;
  }

  // Testing and preview support
  startCapture() {
    this.stdout.startCapture();
    this.stderr.startCapture();
  }

  stopCapture() {
    const stdout = this.stdout.stopCapture();
    const stderr = this.stderr.stopCapture();
    return { stdout, stderr, combined: stdout + stderr };
  }

  getCaptured() {
    return {
      stdout: this.stdout.getCaptured(),
      stderr: this.stderr.getCaptured()
    };
  }

  clearCapture() {
    this.stdout.clear();
    this.stderr.clear();
  }

  // Preview mode - simulate output with mock data
  previewWithMockData() {
    this.startCapture();
    
    // Simulate a complete report
    this.showTraceGeneration(25, null);
    this.showReportHeader();
    this.showAnalysisSummary(2, 5, 12, 45, 80.0, 75.0, 3);
    this.showCombinedAnalysis([
      { owner: 'owner1', repo: 'repo1', type: 'pr', displayUrl: 'https://github.com/owner1/repo1/pull/123', displayName: 'PR #123', branchName: 'feature-branch', urlIndex: 0 },
      { owner: 'owner2', repo: 'repo2', type: 'commit', displayUrl: 'https://github.com/owner2/repo2/commit/abc123', displayName: 'commit abc123', urlIndex: 1 }
    ]);
    
    const result = this.stopCapture();
    return result;
  }
}

// Export singleton instance
export const cliOutput = new CLIOutput();
