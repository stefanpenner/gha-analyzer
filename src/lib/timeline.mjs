import { makeClickableLink, grayText, greenText, redText, yellowText, blueText, humanizeTime } from './text.mjs';
import { getJobGroup } from './analysis.mjs';

export function generateTimelineVisualization(metrics, repoActionsUrl, urlIndex = 0, reviewEvents = []) {
  if (!metrics.jobTimeline || metrics.jobTimeline.length === 0) {
    return '';
  }

  const timeline = metrics.jobTimeline;

  // bottleneck jobs removed
  const headerScale = 60;

  const earliestStart = Math.min(...timeline.map(job => job.startTime));
  const latestEnd = Math.max(...timeline.map(job => job.endTime));
  const totalDuration = latestEnd - earliestStart;

  console.error('┌' + '─'.repeat(headerScale + 2) + '┐');
  const startTimeFormatted = new Date(earliestStart).toLocaleTimeString();
  const endTimeFormatted = new Date(latestEnd).toLocaleTimeString();
  const headerStart = `Start: ${startTimeFormatted}`;
  const headerEnd = `End: ${endTimeFormatted}`;
  const headerPadding = ' '.repeat(Math.max(0, headerScale - headerStart.length - headerEnd.length));
  console.error(`│ ${headerStart}${headerPadding}${headerEnd} │`);
  console.error('├' + '─'.repeat(headerScale + 2) + '┤');

  const jobGroups = {};
  timeline.forEach(job => {
    const groupKey = getJobGroup(job.name);
    if (!jobGroups[groupKey]) {
      jobGroups[groupKey] = [];
    }
    jobGroups[groupKey].push(job);
  });

  const sortedGroupNames = Object.keys(jobGroups).sort((a, b) => {
    const earliestA = Math.min(...jobGroups[a].map(job => job.startTime));
    const earliestB = Math.min(...jobGroups[b].map(job => job.startTime));
    return earliestA - earliestB;
  });

  sortedGroupNames.forEach(groupName => {
    const jobsInGroup = jobGroups[groupName];

    const groupStartTime = Math.min(...jobsInGroup.map(job => job.startTime));
    const groupEndTime = Math.max(...jobsInGroup.map(job => job.endTime));
    const groupWallTime = groupEndTime - groupStartTime;
    const groupTotalSec = groupWallTime / 1000;

    const sortedJobsInGroup = jobsInGroup.sort((a, b) => a.startTime - b.startTime);

    const timeDisplay = humanizeTime(groupTotalSec);
    const cleanGroupName = groupName.replace(/[^\w\s\-_/()]/g, '').trim();
    console.error(`│${' '.repeat(headerScale)}  │ 📁 ${cleanGroupName} (${timeDisplay}, ${jobsInGroup.length} jobs)`);

    sortedJobsInGroup.forEach((job, index) => {
      const relativeStart = job.startTime - earliestStart;
      const duration = job.endTime - job.startTime;
      const durationSec = duration / 1000;

      const startPos = Math.floor((relativeStart / totalDuration) * headerScale);
      const barLength = Math.max(1, Math.floor((duration / totalDuration) * headerScale));
      const clampedBarLength = Math.min(barLength, headerScale - startPos);

      const padding = ' '.repeat(Math.max(0, startPos));

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

      const jobNameParts = job.name.split(' / ');
      const jobNameWithoutPrefix = jobNameParts.length > 1 ? jobNameParts.slice(1).join(' / ') : job.name;
      const cleanJobName = jobNameWithoutPrefix.replace(/[^\w\s\-_/()]/g, '').trim();

      const sameNameJobs = jobsInGroup.filter(j => j.name === job.name);
      const groupIndicator = sameNameJobs.length > 1 ? ` [${sameNameJobs.indexOf(job) + 1}]` : '';

      const isLastJob = index === sortedJobsInGroup.length - 1;
      const treePrefix = isLastJob ? '└── ' : '├── ';

      const jobNameAndTime = `${cleanJobName}${groupIndicator} (${humanizeTime(durationSec)})`;
      const jobLink = job.url ? makeClickableLink(job.url, jobNameAndTime) : jobNameAndTime;

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

  const approvalAndMergeEvents = (reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged');
  if (approvalAndMergeEvents.length > 0 && totalDuration > 0) {
    console.error(`│${' '.repeat(headerScale)}  │ 📁 Approvals & Merge (${approvalAndMergeEvents.length} items)`);
    const sortedEvents = [...approvalAndMergeEvents].sort((a, b) => new Date(a.time) - new Date(b.time));
    {
      const markerSlots = Array(headerScale).fill(' ');
      const reviewers = [];
      sortedEvents.forEach(ev => {
        const eventTime = new Date(ev.time).getTime();
        const relativeStart = Math.max(0, Math.min(eventTime, latestEnd) - earliestStart);
        const col = Math.floor((relativeStart / totalDuration) * headerScale);
        const clampedCol = Math.max(0, Math.min(col, Math.max(0, headerScale - 1)));
        if (ev.type === 'shippit') {
          markerSlots[clampedCol] = '▲';
          if (ev.reviewer) reviewers.push(ev.reviewer);
        }
      });
      const markerLineLeft = markerSlots.join('');
      const rightParts = [];
      if (reviewers.length > 0) rightParts.push(yellowText(`▲ ${reviewers[0]}`));
      const combinedRight = rightParts.join('  ');
      const maxCombinedWidth = headerScale - 4;
      let displayCombined = combinedRight;
      if (displayCombined.length > maxCombinedWidth) {
        displayCombined = displayCombined.substring(0, maxCombinedWidth - 3) + '...';
      }
      console.error(`│${markerLineLeft}  │ ${'└── '}${displayCombined}`);
    }
    sortedEvents.forEach((ev, index) => {
      const eventTime = new Date(ev.time).getTime();
      const relativeStart = Math.max(0, Math.min(eventTime, latestEnd) - earliestStart);
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
      console.error(`│${padding}${marker}${remaining}  │ ${treePrefix}${rightLabel}`);
    });
  }

  console.error('┌' + '─'.repeat(headerScale + 2) + '┐');
  const jobCount = timeline.length;
  const wallTimeSec = (latestEnd - earliestStart) / 1000;
  const footerText = `Timeline: ${new Date(earliestStart).toLocaleTimeString()} → ${new Date(latestEnd).toLocaleTimeString()} • ${humanizeTime(wallTimeSec)} • ${jobCount} jobs`;
  const footerInnerWidth = headerScale + 2;
  const footerLine = ` ${footerText}`;
  const footerPadding = ' '.repeat(Math.max(0, footerInnerWidth - footerLine.length));
  console.error(`│${footerLine}${footerPadding}│`);

  const runsCount = metrics.totalRuns || 0;
  const computeMs = timeline.reduce((sum, j) => sum + Math.max(0, j.endTime - j.startTime), 0);
  const approvalsCount = (reviewEvents || []).filter(ev => ev.type === 'shippit' || ev.type === 'merged').length;
  const hasMerged = (reviewEvents || []).some(ev => ev.type === 'merged');

  const baseLegend = `Legend: ${greenText('█ Success')}  ${redText('█ Failed')}  ${blueText('▒ Pending/Running')}  ${grayText('░ Cancelled/Skipped')}`;
  const markersLegend = `${approvalsCount > 0 ? '  ' + yellowText(`▲ approvals`) : ''}${hasMerged ? '  ' + greenText('◆ merged') : ''}`;
  let legendLine = baseLegend + markersLegend;
  const legendInnerWidth = headerScale + 2;
  let legendContent = ` ${legendLine}`;
  if (legendContent.length > legendInnerWidth) legendContent = legendContent.slice(0, legendInnerWidth);
  const legendPadding = ' '.repeat(Math.max(0, legendInnerWidth - legendContent.length));
  console.error(`│${legendContent}${legendPadding}│`);
  console.error('└' + '─'.repeat(headerScale + 2) + '┘');
}

export function generateHighLevelTimeline(sortedResults, globalEarliestTime, globalLatestTime) {
  const scale = 80;

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

  const startTimeFormatted = new Date(timelineEarliestTime).toLocaleTimeString();
  const endTimeFormatted = new Date(timelineLatestTime).toLocaleTimeString();

  const startLabel = `Start: ${startTimeFormatted}`;
  const endLabel = `End: ${endTimeFormatted}`;
  const middlePadding = ' '.repeat(Math.max(0, scale - startLabel.length - endLabel.length));

  console.error(`┌${'─'.repeat(scale + 2)}┐`);
  console.error(`│ ${startLabel}${middlePadding}${endLabel} │`);
  console.error('├' + '─'.repeat(scale + 2) + '┤');

  sortedResults.forEach((result, index) => {
    const resultEarliestTime = Math.min(...result.metrics.jobTimeline.map(job => job.startTime));
    const resultLatestTime = Math.max(...result.metrics.jobTimeline.map(job => job.endTime));
    const wallTimeSec = (resultLatestTime - resultEarliestTime) / 1000;

    const relativeStart = resultEarliestTime - timelineEarliestTime;
    const startPos = Math.floor((relativeStart / totalDuration) * scale);

    const maxBarLength = scale - startPos;
    const barLength = Math.max(1, Math.min(maxBarLength, Math.floor((wallTimeSec / (totalDuration / 1000)) * scale)));

    const hasFailedJobs = result.metrics.jobTimeline.some(job => job.conclusion === 'failure');
    const hasPendingJobs = result.metrics.pendingJobs && result.metrics.pendingJobs.length > 0;
    const hasSkippedJobs = result.metrics.jobTimeline.some(job => job.conclusion === 'skipped' || job.conclusion === 'cancelled');

    let timeDisplay;
    if (isNaN(wallTimeSec) || wallTimeSec <= 0) {
      timeDisplay = '0s';
    } else {
      timeDisplay = humanizeTime(wallTimeSec);
    }

    const barChars = Array(barLength).fill('█');
    let approvalCount = 0;
    if (result.reviewEvents && result.reviewEvents.length > 0) {
      result.reviewEvents.forEach(event => {
        const eventTime = new Date(event.time).getTime();
        const column = Math.floor(((eventTime - timelineEarliestTime) / totalDuration) * scale);
        const offset = column - startPos;
        const clampedOffset = Math.min(Math.max(offset, 0), Math.max(0, barLength - 1));
        if (event.type === 'merged') {
          barChars[clampedOffset] = '◆';
        } else {
          barChars[clampedOffset] = '▲';
          approvalCount++;
        }
      });
    }
    const barString = barChars.join('');

    const fullText = `[${result.urlIndex + 1}] ${result.displayName} (${timeDisplay})`;

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

    const padding = ' '.repeat(Math.max(0, startPos));
    const remaining = ' '.repeat(Math.max(0, scale - startPos - barLength));
    const suffixParts = [];
    if (approvalCount > 0) suffixParts.push(yellowText(`▲ ${approvalCount}`));
    const markerLabel = suffixParts.length > 0 ? ' ' + suffixParts.join('  ') : '';

    console.error(`│${padding}${coloredBar}${remaining}  │ ${coloredLink}${markerLabel}`);
  });

  console.error('└' + '─'.repeat(scale + 2) + '┘');
}


