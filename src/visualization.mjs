/**
 * Visualization and Output Module
 * Handles all terminal output, timeline generation, and user interface rendering
 * Separated from the underlying data model for better maintainability
 */

import { spawn } from 'child_process';
import os from 'os';
import path from 'path';
import fs from 'fs'; // Added for file system operations

// Utility functions for text formatting and display
export function makeClickableLink(url, text = null) {
  // ANSI escape sequence for clickable links (OSC 8)
  // Format: \u001b]8;;URL\u0007TEXT\u001b]8;;\u0007
  const displayText = text || url;
  return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
}

export function grayText(text) {
  // ANSI escape sequence for gray color (bright black)
  return `\u001b[90m${text}\u001b[0m`;
}

export function greenText(text) {
  // ANSI escape sequence for green color
  return `\u001b[32m${text}\u001b[0m`;
}

export function redText(text) {
  // ANSI escape sequence for red color
  return `\u001b[31m${text}\u001b[0m`;
}

export function yellowText(text) {
  // ANSI escape sequence for yellow color
  return `\u001b[33m${text}\u001b[0m`;
}

export function blueText(text) {
  // ANSI escape sequence for blue color
  return `\u001b[34m${text}\u001b[0m`;
}

// Time formatting utility
export function humanizeTime(seconds) {
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

// Job grouping utility
export function getJobGroup(jobName) {
  // Split by '/' and take the first part as the group
  const parts = jobName.split(' / ');
  return parts.length > 1 ? parts[0] : jobName;
}

// Bottleneck analysis utility
export function findBottleneckJobs(jobs) {
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

// Timeline visualization functions
export function generateTimelineVisualization(metrics, repoActionsUrl, urlIndex = 0, reviewEvents = []) {
  if (!metrics.jobTimeline || metrics.jobTimeline.length === 0) {
    return '';
  }

  const timeline = metrics.jobTimeline;
  
  // Bottleneck job indication removed
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
      
      // Create text for job name and time (without tree prefix)
      const jobNameAndTime = `${cleanJobName}${groupIndicator} (${humanizeTime(durationSec)})`;
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

// High-level timeline visualization
export function generateHighLevelTimeline(sortedResults, globalEarliestTime, globalLatestTime) {
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

// Trace generation functions
export function addThreadMetadata(traceEvents, processId, threadId, name, sortIndex) {
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

export function generateConcurrencyCounters(jobStartTimes, jobEndTimes, traceEvents, earliestTime) {
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

// Perfetto integration
export async function openTraceInPerfetto(traceFile) {
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

// Main output function
export async function outputCombinedResults(analysisData, combinedMetrics, perfettoFile, openInPerfetto = false) {
  if (perfettoFile) {
    console.error(`\n✅ Generated ${analysisData.traceEvents.length} trace events • Open in Perfetto.dev for analysis`);
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
    // Bottleneck jobs removed
    
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
          const fullText = `${i + 1}. ${humanizeTime(duration)} - ${job.name}`;
          const jobLink = job.url ? makeClickableLink(job.url, fullText) : fullText;
          console.error(`    ${jobLink}`);
        });
      }
    });
    // Bottleneck indicator explanation removed
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
      const { writeFileSync } = await import('fs');
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
