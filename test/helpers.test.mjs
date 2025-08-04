import { test, describe } from 'node:test';
import assert from 'node:assert';

describe('Helper Functions - Advanced Features', () => {
  describe('Critical Path Analysis', () => {
    test('should find dominant job as critical path', () => {
      const findCriticalPath = (jobs) => {
        if (jobs.length === 0) return [];
        
        // Sort jobs by start time
        const sortedByStart = [...jobs].sort((a, b) => a.startTime - b.startTime);
        const pipelineStart = sortedByStart[0].startTime;
        const pipelineEnd = Math.max(...sortedByStart.map(job => job.endTime));
        
        // Find the job that ends latest (likely the bottleneck)
        const bottleneckJob = sortedByStart.reduce((longest, job) => 
          job.endTime > longest.endTime ? job : longest
        );
        
        // Simple heuristic: if one job dominates the timeline, it's likely the critical path
        const bottleneckDuration = bottleneckJob.endTime - bottleneckJob.startTime;
        const totalPipelineDuration = pipelineEnd - pipelineStart;
        
        // If the bottleneck job takes up most of the pipeline duration, it's the critical path
        if (bottleneckDuration > totalPipelineDuration * 0.7) {
          return [bottleneckJob];
        }
        
        // Otherwise, find the longest sequential chain
        let criticalPath = [sortedByStart[0]];
        
        for (let i = 1; i < sortedByStart.length; i++) {
          const currentJob = sortedByStart[i];
          const lastInPath = criticalPath[criticalPath.length - 1];
          
          // If this job starts after the last one ends, it could be in the critical path
          if (currentJob.startTime >= lastInPath.endTime) {
            criticalPath.push(currentJob);
          }
        }
        
        return criticalPath;
      };

      const jobs = [
        { name: 'FastJob1', startTime: 1000, endTime: 2000 },
        { name: 'FastJob2', startTime: 2000, endTime: 3000 },
        { name: 'DominantJob', startTime: 1500, endTime: 10000 }, // Takes 85% of pipeline time
        { name: 'FastJob3', startTime: 3000, endTime: 4000 }
      ];
      
      const criticalPath = findCriticalPath(jobs);
      assert.strictEqual(criticalPath.length, 1);
      assert.strictEqual(criticalPath[0].name, 'DominantJob');
    });

    test('should find sequential critical path when no dominant job', () => {
      const findCriticalPath = (jobs) => {
        if (jobs.length === 0) return [];
        
        const sortedByStart = [...jobs].sort((a, b) => a.startTime - b.startTime);
        let criticalPath = [sortedByStart[0]];
        
        for (let i = 1; i < sortedByStart.length; i++) {
          const currentJob = sortedByStart[i];
          const lastInPath = criticalPath[criticalPath.length - 1];
          
          if (currentJob.startTime >= lastInPath.endTime) {
            criticalPath.push(currentJob);
          }
        }
        
        return criticalPath;
      };

      const jobs = [
        { name: 'Job1', startTime: 1000, endTime: 2000 },
        { name: 'Job2', startTime: 2000, endTime: 3000 }, // Sequential after Job1
        { name: 'Job3', startTime: 1500, endTime: 2500 }  // Overlaps, not in critical path
      ];
      
      const criticalPath = findCriticalPath(jobs);
      assert.strictEqual(criticalPath.length, 2);
      assert.strictEqual(criticalPath[0].name, 'Job1');
      assert.strictEqual(criticalPath[1].name, 'Job2');
    });
  });

  describe('Timeline Visualization', () => {
    test('should generate timeline data for visualization', () => {
      const generateTimeline = (jobs) => {
        return jobs.map(job => ({
          name: job.name,
          startTime: job.startTime,
          endTime: job.endTime,
          duration: job.endTime - job.startTime,
          conclusion: job.conclusion || 'success',
          url: job.url || null
        }));
      };

      const jobs = [
        { name: 'lint', startTime: 1000, endTime: 3000, conclusion: 'success' },
        { name: 'test', startTime: 2000, endTime: 8000, conclusion: 'failure' },
        { name: 'deploy', startTime: 9000, endTime: 12000, conclusion: 'success' }
      ];

      const timeline = generateTimeline(jobs);
      
      assert.strictEqual(timeline.length, 3);
      assert.strictEqual(timeline[0].name, 'lint');
      assert.strictEqual(timeline[0].duration, 2000);
      assert.strictEqual(timeline[1].conclusion, 'failure');
      assert.strictEqual(timeline[2].startTime, 9000);
    });
  });

  describe('Performance Metrics', () => {
    test('should calculate comprehensive performance metrics', () => {
      const calculateMetrics = (jobs, steps) => {
        const jobDurations = jobs.map(j => j.endTime - j.startTime);
        const stepDurations = steps.map(s => s.duration);
        
        const successfulJobs = jobs.filter(j => j.conclusion === 'success').length;
        const successfulSteps = steps.filter(s => s.conclusion === 'success').length;
        
        return {
          totalJobs: jobs.length,
          totalSteps: steps.length,
          successfulJobs,
          successfulSteps,
          jobSuccessRate: jobs.length > 0 ? (successfulJobs / jobs.length * 100).toFixed(1) : '0.0',
          stepSuccessRate: steps.length > 0 ? (successfulSteps / steps.length * 100).toFixed(1) : '0.0',
          avgJobDuration: jobDurations.length > 0 ? 
            jobDurations.reduce((a, b) => a + b, 0) / jobDurations.length : 0,
          avgStepDuration: stepDurations.length > 0 ? 
            stepDurations.reduce((a, b) => a + b, 0) / stepDurations.length : 0,
          totalPipelineDuration: jobs.length > 0 ? 
            Math.max(...jobs.map(j => j.endTime)) - Math.min(...jobs.map(j => j.startTime)) : 0
        };
      };

      const jobs = [
        { name: 'lint', startTime: 1000, endTime: 3000, conclusion: 'success' },
        { name: 'test', startTime: 2000, endTime: 8000, conclusion: 'failure' },
        { name: 'deploy', startTime: 9000, endTime: 12000, conclusion: 'success' }
      ];

      const steps = [
        { name: 'Checkout', duration: 1000, conclusion: 'success' },
        { name: 'Setup', duration: 2000, conclusion: 'success' },
        { name: 'Build', duration: 3000, conclusion: 'failure' },
        { name: 'Test', duration: 4000, conclusion: 'success' }
      ];

      const metrics = calculateMetrics(jobs, steps);
      
      assert.strictEqual(metrics.totalJobs, 3);
      assert.strictEqual(metrics.totalSteps, 4);
      assert.strictEqual(metrics.successfulJobs, 2);
      assert.strictEqual(metrics.successfulSteps, 3);
      assert.strictEqual(metrics.jobSuccessRate, '66.7');
      assert.strictEqual(metrics.stepSuccessRate, '75.0');
             assert(Math.abs(metrics.avgJobDuration - 3666.67) < 0.1); // (2000 + 6000 + 3000) / 3 = 3666.67
      assert.strictEqual(metrics.avgStepDuration, 2500); // (1000 + 2000 + 3000 + 4000) / 4
      assert.strictEqual(metrics.totalPipelineDuration, 11000); // 12000 - 1000
    });
  });

  describe('Clickable Link Generation', () => {
    test('should generate terminal clickable links', () => {
      const makeClickableLink = (url, text = null) => {
        const displayText = text || url;
        return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
      };

      const url = 'https://github.com/owner/repo/actions/runs/123';
      const result = makeClickableLink(url, 'View Job');
      
      assert.strictEqual(result, `\u001b]8;;${url}\u0007View Job\u001b]8;;\u0007`);
    });
  });

  describe('Overlap Detection', () => {
    test('should find overlapping jobs for optimization analysis', () => {
      const findOverlappingJobs = (jobs) => {
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
      };

      const jobs = [
        { name: 'Job1', startTime: 1000, endTime: 3000 },
        { name: 'Job2', startTime: 2000, endTime: 4000 }, // Overlaps with Job1
        { name: 'Job3', startTime: 5000, endTime: 6000 }  // No overlap
      ];
      
      const overlaps = findOverlappingJobs(jobs);
      assert.strictEqual(overlaps.length, 1);
      assert.strictEqual(overlaps[0][0].name, 'Job1');
      assert.strictEqual(overlaps[0][1].name, 'Job2');
    });
  });

  describe('Timeline Grouping Functions', () => {
    function getJobGroup(jobName) {
      // Split by '/' and take the first part as the group
      const parts = jobName.split(' / ');
      return parts.length > 1 ? parts[0] : jobName;
    }

    function groupJobsByPrefix(timeline) {
      const jobGroups = {};
      timeline.forEach(job => {
        const groupKey = getJobGroup(job.name);
        if (!jobGroups[groupKey]) {
          jobGroups[groupKey] = [];
        }
        jobGroups[groupKey].push(job);
      });
      return jobGroups;
    }

    function sortGroupsByEarliestStart(jobGroups) {
      return Object.keys(jobGroups).sort((a, b) => {
        const earliestA = Math.min(...jobGroups[a].map(job => job.startTime));
        const earliestB = Math.min(...jobGroups[b].map(job => job.startTime));
        return earliestA - earliestB;
      });
    }

    test('should extract job group from job name', () => {
      assert.strictEqual(getJobGroup('CI-validations / Build: ci-mariner2'), 'CI-validations');
      assert.strictEqual(getJobGroup('Release / Setup'), 'Release');
      assert.strictEqual(getJobGroup('Post-merge / Build / (Dry run) Build'), 'Post-merge');
      assert.strictEqual(getJobGroup('Simple Job'), 'Simple Job');
      assert.strictEqual(getJobGroup('No / Slash'), 'No');
    });

    test('should group jobs by prefix', () => {
      const timeline = [
        { name: 'CI-validations / Setup', startTime: 1000, endTime: 2000 },
        { name: 'CI-validations / Build', startTime: 2000, endTime: 3000 },
        { name: 'Release / Setup', startTime: 1500, endTime: 2500 },
        { name: 'Release / Publish', startTime: 2500, endTime: 3500 },
        { name: 'Simple Job', startTime: 500, endTime: 1500 }
      ];

      const groups = groupJobsByPrefix(timeline);
      
      assert.strictEqual(Object.keys(groups).length, 3);
      assert.strictEqual(groups['CI-validations'].length, 2);
      assert.strictEqual(groups['Release'].length, 2);
      assert.strictEqual(groups['Simple Job'].length, 1);
    });

    test('should sort groups by earliest start time', () => {
      const jobGroups = {
        'Release': [
          { name: 'Release / Setup', startTime: 2000, endTime: 3000 },
          { name: 'Release / Publish', startTime: 3000, endTime: 4000 }
        ],
        'CI-validations': [
          { name: 'CI-validations / Setup', startTime: 1000, endTime: 2000 },
          { name: 'CI-validations / Build', startTime: 1500, endTime: 2500 }
        ],
        'Post-merge': [
          { name: 'Post-merge / Setup', startTime: 500, endTime: 1500 }
        ]
      };

      const sortedGroups = sortGroupsByEarliestStart(jobGroups);
      
      assert.deepStrictEqual(sortedGroups, ['Post-merge', 'CI-validations', 'Release']);
    });

    test('should handle jobs with same name in group', () => {
      const timeline = [
        { name: 'Release / Setup', startTime: 1000, endTime: 2000 },
        { name: 'Release / Setup', startTime: 2000, endTime: 3000 }, // Same name, different run
        { name: 'Release / Publish', startTime: 1500, endTime: 2500 }
      ];

      const groups = groupJobsByPrefix(timeline);
      const sortedGroups = sortGroupsByEarliestStart(groups);
      
      assert.strictEqual(groups['Release'].length, 3);
      assert.strictEqual(sortedGroups[0], 'Release');
    });

    test('should handle empty timeline', () => {
      const timeline = [];
      const groups = groupJobsByPrefix(timeline);
      const sortedGroups = sortGroupsByEarliestStart(groups);
      
      assert.strictEqual(Object.keys(groups).length, 0);
      assert.strictEqual(sortedGroups.length, 0);
    });

    test('should handle single job', () => {
      const timeline = [
        { name: 'Single Job', startTime: 1000, endTime: 2000 }
      ];

      const groups = groupJobsByPrefix(timeline);
      const sortedGroups = sortGroupsByEarliestStart(groups);
      
      assert.strictEqual(Object.keys(groups).length, 1);
      assert.strictEqual(groups['Single Job'].length, 1);
      assert.strictEqual(sortedGroups[0], 'Single Job');
    });

    test('should handle complex job names with multiple slashes', () => {
      const timeline = [
        { name: 'Post-merge / Build / (Dry run) Build on Mariner2', startTime: 1000, endTime: 2000 },
        { name: 'Post-merge / Setup / (Dry run) Setup', startTime: 500, endTime: 1500 },
        { name: 'CI-validations / Build: ci-mariner2', startTime: 2000, endTime: 3000 }
      ];

      const groups = groupJobsByPrefix(timeline);
      const sortedGroups = sortGroupsByEarliestStart(groups);
      
      assert.strictEqual(Object.keys(groups).length, 2);
      assert.strictEqual(groups['Post-merge'].length, 2);
      assert.strictEqual(groups['CI-validations'].length, 1);
      assert.deepStrictEqual(sortedGroups, ['Post-merge', 'CI-validations']);
    });

    test('should sort jobs within groups by start time', () => {
      const timeline = [
        { name: 'Release / Publish', startTime: 2000, endTime: 3000 },
        { name: 'Release / Setup', startTime: 1000, endTime: 2000 },
        { name: 'Release / Teardown', startTime: 3000, endTime: 4000 }
      ];

      const groups = groupJobsByPrefix(timeline);
      const releaseJobs = groups['Release'];
      const sortedReleaseJobs = releaseJobs.sort((a, b) => a.startTime - b.startTime);
      
      assert.strictEqual(sortedReleaseJobs[0].name, 'Release / Setup');
      assert.strictEqual(sortedReleaseJobs[1].name, 'Release / Publish');
      assert.strictEqual(sortedReleaseJobs[2].name, 'Release / Teardown');
    });
  });
}); 