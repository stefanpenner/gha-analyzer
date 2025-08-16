// Domain utilities for grouping and bottleneck analysis

export function getJobGroup(jobName) {
  const parts = jobName.split(' / ');
  return parts.length > 1 ? parts[0] : jobName;
}

export function findBottleneckJobs(jobs) {
  if (!jobs || jobs.length === 0) return [];

  const significantJobs = jobs.filter(job => {
    const duration = job.endTime - job.startTime;
    return duration > 1000;
  });
  if (significantJobs.length === 0) return [];

  const sortedByDuration = [...significantJobs].sort((a, b) => {
    const durationA = b.endTime - b.startTime;
    const durationB = a.endTime - a.startTime;
    return durationA - durationB;
  });

  const pipelineStart = Math.min(...jobs.map(job => job.startTime));
  const pipelineEnd = Math.max(...jobs.map(job => job.endTime));
  const totalPipelineDuration = pipelineEnd - pipelineStart;

  const bottleneckThreshold = totalPipelineDuration * 0.1;
  const bottleneckJobs = sortedByDuration.filter(job => {
    const duration = job.endTime - job.startTime;
    return duration > bottleneckThreshold;
  });

  if (bottleneckJobs.length === 0) {
    return sortedByDuration.slice(0, 2);
  }

  return bottleneckJobs;
}


