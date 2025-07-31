import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert';
import { createGitHubMock } from './github-mock.mjs';

describe('GitHub Actions Profiler - Core Functionality', () => {
  let githubMock;

  beforeEach(() => {
    githubMock = createGitHubMock();
  });

  afterEach(() => {
    githubMock.cleanup();
  });

  describe('URL Parsing', () => {
    test('should parse valid GitHub PR URL', () => {
      const url = 'https://github.com/owner/repo/pull/123';
      const parsed = new URL(url);
      const pathParts = parsed.pathname.split('/').filter(Boolean);
      
      assert.strictEqual(pathParts[0], 'owner');
      assert.strictEqual(pathParts[1], 'repo');
      assert.strictEqual(pathParts[2], 'pull');
      assert.strictEqual(pathParts[3], '123');
    });

    test('should handle URLs with trailing slash', () => {
      const url = 'https://github.com/owner/repo/pull/456/';
      const parsed = new URL(url);
      const pathParts = parsed.pathname.split('/').filter(Boolean);
      
      assert.strictEqual(pathParts[0], 'owner');
      assert.strictEqual(pathParts[1], 'repo');
      assert.strictEqual(pathParts[2], 'pull');
      assert.strictEqual(pathParts[3], '456');
    });

    test('should reject invalid URL format', () => {
      const url = 'https://github.com/owner/repo/issues/123';
      const parsed = new URL(url);
      const pathParts = parsed.pathname.split('/').filter(Boolean);
      
      assert.notStrictEqual(pathParts[2], 'pull');
    });
  });

  describe('Step Categorization', () => {
    test('should categorize common step types', () => {
      const categorizeStep = (stepName) => {
        const name = stepName.toLowerCase();
        
        if (name.includes('checkout') || name.includes('clone')) return 'step_checkout';
        if (name.includes('setup') || name.includes('install') || name.includes('cache')) return 'step_setup';
        if (name.includes('build') || name.includes('compile') || name.includes('make')) return 'step_build';
        if (name.includes('test') || name.includes('spec') || name.includes('coverage')) return 'step_test';
        if (name.includes('lint') || name.includes('format') || name.includes('check')) return 'step_lint';
        if (name.includes('deploy') || name.includes('publish') || name.includes('release')) return 'step_deploy';
        
        return 'step_other';
      };

      assert.strictEqual(categorizeStep('Checkout code'), 'step_checkout');
      assert.strictEqual(categorizeStep('Setup Node.js'), 'step_setup');
      assert.strictEqual(categorizeStep('Build application'), 'step_build');
      assert.strictEqual(categorizeStep('Run tests'), 'step_test');
      assert.strictEqual(categorizeStep('Run linting'), 'step_lint');
      assert.strictEqual(categorizeStep('Deploy to production'), 'step_deploy');
      assert.strictEqual(categorizeStep('Random step'), 'step_other');
    });
  });

  describe('Step Icon Assignment', () => {
    test('should assign appropriate icons based on step type and conclusion', () => {
      const getStepIcon = (stepName, conclusion) => {
        const name = stepName.toLowerCase();
        
        // Failure/success override icons
        if (conclusion === 'failure') return '❌';
        if (conclusion === 'cancelled') return '🚫';
        if (conclusion === 'skipped') return '⏭️';
        
        // Category-based icons
        if (name.includes('checkout') || name.includes('clone')) return '📥';
        if (name.includes('setup') || name.includes('install')) return '⚙️';
        if (name.includes('build') || name.includes('compile')) return '🔨';
        if (name.includes('test') || name.includes('spec')) return '🧪';
        if (name.includes('lint') || name.includes('format')) return '🔍';
        if (name.includes('deploy') || name.includes('publish')) return '🚀';
        
        return '▶️'; // Default step icon
      };

      assert.strictEqual(getStepIcon('Build application', 'failure'), '❌');
      assert.strictEqual(getStepIcon('Checkout code', 'success'), '📥');
      assert.strictEqual(getStepIcon('Setup Node.js', 'success'), '⚙️');
      assert.strictEqual(getStepIcon('Run tests', 'success'), '🧪');
      assert.strictEqual(getStepIcon('Unknown step', 'success'), '▶️');
    });
  });

  describe('Metrics Calculation', () => {
    test('should calculate success rates correctly', () => {
      const calculateSuccessRate = (successful, total) => {
        return total > 0 ? (successful / total * 100).toFixed(1) : '0.0';
      };

      assert.strictEqual(calculateSuccessRate(8, 10), '80.0');
      assert.strictEqual(calculateSuccessRate(0, 5), '0.0');
      assert.strictEqual(calculateSuccessRate(5, 5), '100.0');
      assert.strictEqual(calculateSuccessRate(0, 0), '0.0');
    });

    test('should calculate average durations', () => {
      const calculateAverage = (durations) => {
        return durations.length > 0 ? 
          durations.reduce((a, b) => a + b, 0) / durations.length : 0;
      };

      assert.strictEqual(calculateAverage([1000, 2000, 3000]), 2000);
      assert.strictEqual(calculateAverage([]), 0);
      assert.strictEqual(calculateAverage([5000]), 5000);
    });
  });

  describe('Trace Event Generation', () => {
    test('should generate valid Chrome Tracing format structure', () => {
      const generateTraceEvents = (jobs, steps) => {
        const events = [];
        
        // Add metadata
        events.push({
          name: 'process_name',
          ph: 'M',
          pid: 1,
          tid: 0,
          args: { name: 'GitHub Actions Pipeline' }
        });
        
        // Add job events
        jobs.forEach((job, index) => {
          events.push({
            name: `Job: ${job.name}`,
            ph: 'X',
            pid: 1,
            tid: index + 1,
            ts: job.startTime * 1000, // Convert to microseconds
            dur: (job.endTime - job.startTime) * 1000,
            args: { conclusion: job.conclusion }
          });
        });
        
        return {
          displayTimeUnit: 'ms',
          traceEvents: events,
          otherData: { totalJobs: jobs.length, totalSteps: steps.length }
        };
      };

      const jobs = [
        { name: 'lint', startTime: 1000, endTime: 3000, conclusion: 'success' },
        { name: 'test', startTime: 2000, endTime: 8000, conclusion: 'success' }
      ];
      
      const steps = [
        { name: 'Checkout', duration: 1000 },
        { name: 'Setup', duration: 2000 }
      ];

      const trace = generateTraceEvents(jobs, steps);
      
      assert.strictEqual(trace.displayTimeUnit, 'ms');
      assert(Array.isArray(trace.traceEvents));
      assert(trace.traceEvents.length > 0);
      assert(trace.otherData);
      assert.strictEqual(trace.otherData.totalJobs, 2);
      assert.strictEqual(trace.otherData.totalSteps, 2);
      
      // Verify event structure
      const jobEvents = trace.traceEvents.filter(e => e.name?.startsWith('Job:'));
      assert.strictEqual(jobEvents.length, 2);
      
      jobEvents.forEach(event => {
        assert.strictEqual(event.ph, 'X');
        assert(typeof event.ts === 'number');
        assert(typeof event.dur === 'number');
        assert(event.dur > 0);
      });
    });
  });

  describe('Performance Analysis', () => {
    test('should identify slowest jobs', () => {
      const findSlowestJobs = (jobs, limit = 3) => {
        return jobs
          .sort((a, b) => b.duration - a.duration)
          .slice(0, limit);
      };

      const jobs = [
        { name: 'lint', duration: 5000 },
        { name: 'test', duration: 15000 },
        { name: 'build', duration: 3000 },
        { name: 'deploy', duration: 8000 }
      ];

      const slowest = findSlowestJobs(jobs, 2);
      
      assert.strictEqual(slowest.length, 2);
      assert.strictEqual(slowest[0].name, 'test');
      assert.strictEqual(slowest[0].duration, 15000);
      assert.strictEqual(slowest[1].name, 'deploy');
      assert.strictEqual(slowest[1].duration, 8000);
    });

    test('should calculate concurrency', () => {
      const calculateMaxConcurrency = (jobs) => {
        if (jobs.length === 0) return 0;
        
        const events = [];
        jobs.forEach(job => {
          events.push({ time: job.startTime, type: 'start' });
          events.push({ time: job.endTime, type: 'end' });
        });
        
        events.sort((a, b) => a.time - b.time);
        
        let currentConcurrency = 0;
        let maxConcurrency = 0;
        
        for (const event of events) {
          if (event.type === 'start') {
            currentConcurrency++;
            maxConcurrency = Math.max(maxConcurrency, currentConcurrency);
          } else {
            currentConcurrency--;
          }
        }
        
        return maxConcurrency;
      };

      const jobs = [
        { startTime: 1000, endTime: 3000 },
        { startTime: 2000, endTime: 4000 }, // Overlaps with first job
        { startTime: 5000, endTime: 6000 }  // No overlap
      ];

      const maxConcurrency = calculateMaxConcurrency(jobs);
      assert.strictEqual(maxConcurrency, 2); // Two jobs overlap at peak
    });
  });

  describe('Error Handling', () => {
    test('should validate PR URL format', () => {
      const validatePRUrl = (url) => {
        try {
          const parsed = new URL(url);
          const pathParts = parsed.pathname.split('/').filter(Boolean);
          return pathParts.length === 4 && pathParts[2] === 'pull';
        } catch {
          return false;
        }
      };

      assert.strictEqual(validatePRUrl('https://github.com/owner/repo/pull/123'), true);
      assert.strictEqual(validatePRUrl('https://github.com/owner/repo/issues/123'), false);
      assert.strictEqual(validatePRUrl('invalid-url'), false);
      assert.strictEqual(validatePRUrl('https://github.com/owner/repo'), false);
    });

    test('should validate required parameters', () => {
      const validateParams = (prUrl, token) => {
        const errors = [];
        
        if (!prUrl) errors.push('PR URL is required');
        if (!token) errors.push('GitHub token is required');
        
        return {
          isValid: errors.length === 0,
          errors
        };
      };

      assert.strictEqual(validateParams('https://github.com/owner/repo/pull/123', 'token').isValid, true);
      assert.strictEqual(validateParams('', 'token').isValid, false);
      assert.strictEqual(validateParams('https://github.com/owner/repo/pull/123', '').isValid, false);
      assert.strictEqual(validateParams('', '').isValid, false);
    });
  });
}); 
