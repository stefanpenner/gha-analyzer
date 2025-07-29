import { test, describe } from 'node:test';
import assert from 'node:assert';

// Import helper functions from main.mjs
// Note: We'll need to refactor main.mjs to export these functions for testing
// For now, we'll test the logic by duplicating the functions here

describe('Helper Functions', () => {
  describe('parsePRUrl', () => {
    function parsePRUrl(prUrl) {
      const parsed = new URL(prUrl);
      const pathParts = parsed.pathname.split('/').filter(Boolean);
      if (pathParts.length !== 4 || pathParts[2] !== 'pull') {
        throw new Error('Invalid PR URL');
      }
      return {
        owner: pathParts[0],
        repo: pathParts[1],
        prNumber: pathParts[3]
      };
    }

    test('should parse valid GitHub PR URL', () => {
      const result = parsePRUrl('https://github.com/owner/repo/pull/123');
      
      assert.strictEqual(result.owner, 'owner');
      assert.strictEqual(result.repo, 'repo');
      assert.strictEqual(result.prNumber, '123');
    });

    test('should handle URLs with trailing slash', () => {
      const result = parsePRUrl('https://github.com/owner/repo/pull/456/');
      
      assert.strictEqual(result.owner, 'owner');
      assert.strictEqual(result.repo, 'repo');
      assert.strictEqual(result.prNumber, '456');
    });

    test('should throw error for invalid URL format', () => {
      assert.throws(() => {
        parsePRUrl('https://github.com/owner/repo/issues/123');
      }, /Invalid PR URL/);
    });

    test('should throw error for incomplete URL', () => {
      assert.throws(() => {
        parsePRUrl('https://github.com/owner/repo');
      }, /Invalid PR URL/);
    });

    test('should parse non-GitHub URLs (no hostname validation)', () => {
      // Note: The current implementation doesn't validate hostname
      const result = parsePRUrl('https://gitlab.com/owner/repo/pull/123');
      assert.strictEqual(result.owner, 'owner');
      assert.strictEqual(result.repo, 'repo');
      assert.strictEqual(result.prNumber, '123');
    });
  });

  describe('categorizeStep', () => {
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

    test('should categorize checkout steps', () => {
      assert.strictEqual(categorizeStep('Checkout code'), 'step_checkout');
      assert.strictEqual(categorizeStep('Clone repository'), 'step_checkout');
      assert.strictEqual(categorizeStep('actions/checkout@v4'), 'step_checkout');
    });

    test('should categorize setup steps', () => {
      assert.strictEqual(categorizeStep('Setup Node.js'), 'step_setup');
      assert.strictEqual(categorizeStep('Install dependencies'), 'step_setup');
      assert.strictEqual(categorizeStep('Cache node modules'), 'step_setup');
    });

    test('should categorize build steps', () => {
      assert.strictEqual(categorizeStep('Build application'), 'step_build');
      assert.strictEqual(categorizeStep('Compile TypeScript'), 'step_build');
      assert.strictEqual(categorizeStep('Make dist'), 'step_build');
    });

    test('should categorize test steps', () => {
      assert.strictEqual(categorizeStep('Run tests'), 'step_test');
      assert.strictEqual(categorizeStep('Unit test spec'), 'step_test');
      assert.strictEqual(categorizeStep('Generate coverage'), 'step_test');
    });

    test('should categorize lint steps', () => {
      assert.strictEqual(categorizeStep('Run linting'), 'step_lint');
      assert.strictEqual(categorizeStep('Format code'), 'step_lint');
      assert.strictEqual(categorizeStep('Type check'), 'step_lint');
    });

    test('should categorize deploy steps', () => {
      assert.strictEqual(categorizeStep('Deploy to production'), 'step_deploy');
      assert.strictEqual(categorizeStep('Publish package'), 'step_deploy');
      assert.strictEqual(categorizeStep('Create release'), 'step_deploy');
    });

    test('should categorize other steps', () => {
      assert.strictEqual(categorizeStep('Random step name'), 'step_other');
      assert.strictEqual(categorizeStep('Custom action'), 'step_other');
    });

    test('should be case insensitive', () => {
      assert.strictEqual(categorizeStep('CHECKOUT CODE'), 'step_checkout');
      assert.strictEqual(categorizeStep('Setup NODE.JS'), 'step_setup');
    });
  });

  describe('getStepIcon', () => {
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

    test('should prioritize conclusion over step name', () => {
      assert.strictEqual(getStepIcon('Build application', 'failure'), '❌');
      assert.strictEqual(getStepIcon('Run tests', 'cancelled'), '🚫');
      assert.strictEqual(getStepIcon('Deploy', 'skipped'), '⏭️');
    });

    test('should return category-based icons for successful steps', () => {
      assert.strictEqual(getStepIcon('Checkout code', 'success'), '📥');
      assert.strictEqual(getStepIcon('Setup Node.js', 'success'), '⚙️');
      assert.strictEqual(getStepIcon('Build app', 'success'), '🔨');
      assert.strictEqual(getStepIcon('Run tests', 'success'), '🧪');
      assert.strictEqual(getStepIcon('Deploy', 'success'), '🚀');
    });

    test('should return default icon for unknown steps', () => {
      assert.strictEqual(getStepIcon('Unknown step', 'success'), '▶️');
      assert.strictEqual(getStepIcon('Custom action', 'success'), '▶️');
    });

    test('should handle special categories', () => {
      assert.strictEqual(getStepIcon('Cache dependencies', 'success'), '💾');
      assert.strictEqual(getStepIcon('Build docker', 'success'), '🔨'); // build takes precedence over docker
      assert.strictEqual(getStepIcon('Security scan', 'success'), '🔒');
      assert.strictEqual(getStepIcon('Database migration', 'success'), '🗄️');
    });
  });

  describe('initializeMetrics', () => {
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
        stepDurations: [],
        runnerTypes: new Set(),
        totalDuration: 0,
        longestJob: { name: '', duration: 0 },
        shortestJob: { name: '', duration: Infinity }
      };
    }

    test('should initialize all metrics to zero/empty', () => {
      const metrics = initializeMetrics();
      
      assert.strictEqual(metrics.totalRuns, 0);
      assert.strictEqual(metrics.successfulRuns, 0);
      assert.strictEqual(metrics.failedRuns, 0);
      assert.strictEqual(metrics.totalJobs, 0);
      assert.strictEqual(metrics.failedJobs, 0);
      assert.strictEqual(metrics.totalSteps, 0);
      assert.strictEqual(metrics.failedSteps, 0);
      assert.strictEqual(metrics.totalDuration, 0);
      
      assert(Array.isArray(metrics.jobDurations));
      assert.strictEqual(metrics.jobDurations.length, 0);
      
      assert(Array.isArray(metrics.stepDurations));
      assert.strictEqual(metrics.stepDurations.length, 0);
      
      assert(metrics.runnerTypes instanceof Set);
      assert.strictEqual(metrics.runnerTypes.size, 0);
      
      assert.strictEqual(metrics.longestJob.name, '');
      assert.strictEqual(metrics.longestJob.duration, 0);
      assert.strictEqual(metrics.shortestJob.name, '');
      assert.strictEqual(metrics.shortestJob.duration, Infinity);
    });
  });

  describe('findEarliestTimestamp', () => {
    function findEarliestTimestamp(allRuns) {
      let earliest = Infinity;
      for (const run of allRuns) {
        const runTime = new Date(run.created_at).getTime();
        earliest = Math.min(earliest, runTime);
      }
      return earliest;
    }

    test('should find earliest timestamp from multiple runs', () => {
      const runs = [
        { created_at: '2024-01-01T10:00:00Z' },
        { created_at: '2024-01-01T09:00:00Z' }, // Earliest
        { created_at: '2024-01-01T11:00:00Z' }
      ];
      
      const earliest = findEarliestTimestamp(runs);
      const expected = new Date('2024-01-01T09:00:00Z').getTime();
      
      assert.strictEqual(earliest, expected);
    });

    test('should handle single run', () => {
      const runs = [
        { created_at: '2024-01-01T10:00:00Z' }
      ];
      
      const earliest = findEarliestTimestamp(runs);
      const expected = new Date('2024-01-01T10:00:00Z').getTime();
      
      assert.strictEqual(earliest, expected);
    });

    test('should return Infinity for empty runs', () => {
      const earliest = findEarliestTimestamp([]);
      assert.strictEqual(earliest, Infinity);
    });
  });

  describe('calculateFinalMetrics', () => {
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
      
      const maxConcurrency = calculateMaxConcurrency(jobStartTimes || [], jobEndTimes || []);
      
      return {
        ...metrics,
        avgJobDuration,
        avgStepDuration,
        successRate,
        jobSuccessRate,
        maxConcurrency
      };
    }

    test('should calculate averages correctly', () => {
      const metrics = {
        totalRuns: 2,
        successfulRuns: 1,
        totalJobs: 3,
        failedJobs: 1,
        jobDurations: [1000, 2000, 3000],
        stepDurations: [
          { duration: 500 },
          { duration: 1000 },
          { duration: 1500 }
        ]
      };
      
      // Mock overlapping job timing for concurrency test
      const jobStartTimes = [
        { ts: 1000, type: 'start' },
        { ts: 1500, type: 'start' },
        { ts: 2000, type: 'start' }
      ];
      const jobEndTimes = [
        { ts: 3000, type: 'end' },
        { ts: 3500, type: 'end' },
        { ts: 4000, type: 'end' }
      ];
      
      const final = calculateFinalMetrics(metrics, 2, jobStartTimes, jobEndTimes);
      
      assert.strictEqual(final.avgJobDuration, 2000); // (1000+2000+3000)/3
      assert.strictEqual(final.avgStepDuration, 1000); // (500+1000+1500)/3
      assert.strictEqual(final.successRate, '50.0'); // 1/2 * 100
      assert.strictEqual(final.jobSuccessRate, '66.7'); // 2/3 * 100
      assert.strictEqual(final.maxConcurrency, 3); // All 3 jobs overlap
    });

    test('should handle empty arrays', () => {
      const metrics = {
        totalRuns: 0,
        successfulRuns: 0,
        totalJobs: 0,
        failedJobs: 0,
        jobDurations: [],
        stepDurations: []
      };
      
      const final = calculateFinalMetrics(metrics, 0, [], []);
      
      assert.strictEqual(final.avgJobDuration, 0);
      assert.strictEqual(final.avgStepDuration, 0);
      assert.strictEqual(final.successRate, 0);
      assert.strictEqual(final.jobSuccessRate, 0);
      assert.strictEqual(final.maxConcurrency, 0);
    });

    test('should calculate max concurrency correctly', () => {
      // Test scenario: 3 jobs with varying overlap
      const jobStartTimes = [
        { ts: 1000, type: 'start' }, // Job 1 starts
        { ts: 1500, type: 'start' }, // Job 2 starts (2 concurrent)
        { ts: 2000, type: 'start' }  // Job 3 starts (3 concurrent - peak)
      ];
      const jobEndTimes = [
        { ts: 2500, type: 'end' },   // Job 1 ends (back to 2 concurrent)
        { ts: 3000, type: 'end' },   // Job 2 ends (back to 1 concurrent)
        { ts: 3500, type: 'end' }    // Job 3 ends (0 concurrent)
      ];
      
      const metrics = { totalJobs: 3, jobDurations: [], stepDurations: [] };
      const final = calculateFinalMetrics(metrics, 1, jobStartTimes, jobEndTimes);
      
      assert.strictEqual(final.maxConcurrency, 3, 'Should detect peak concurrency of 3');
    });
  });

  describe('analyzeSlowJobs', () => {
    function analyzeSlowJobs(metrics, limit = 5) {
      const jobData = [];
      for (let i = 0; i < metrics.jobDurations.length; i++) {
        jobData.push({
          name: metrics.jobNames ? metrics.jobNames[i] : `Job ${i + 1}`,
          duration: metrics.jobDurations[i],
          url: metrics.jobUrls ? metrics.jobUrls[i] : null
        });
      }
      
      return jobData
        .sort((a, b) => b.duration - a.duration)
        .slice(0, limit);
    }

    test('should identify slowest jobs correctly', () => {
      const metrics = {
        jobDurations: [5000, 15000, 3000, 20000, 8000],
        jobNames: ['lint', 'test', 'build', 'e2e', 'deploy'],
        jobUrls: ['url1', 'url2', 'url3', 'url4', 'url5']
      };

      const slowJobs = analyzeSlowJobs(metrics, 3);
      
      assert.strictEqual(slowJobs.length, 3);
      assert.strictEqual(slowJobs[0].name, 'e2e');
      assert.strictEqual(slowJobs[0].duration, 20000);
      assert.strictEqual(slowJobs[1].name, 'test');
      assert.strictEqual(slowJobs[1].duration, 15000);
      assert.strictEqual(slowJobs[2].name, 'deploy');
      assert.strictEqual(slowJobs[2].duration, 8000);
    });

    test('should handle missing job names', () => {
      const metrics = {
        jobDurations: [5000, 15000],
        // No jobNames array
        jobUrls: ['url1', 'url2']
      };

      const slowJobs = analyzeSlowJobs(metrics, 2);
      
      assert.strictEqual(slowJobs[0].name, 'Job 2'); // Slowest gets index 2
      assert.strictEqual(slowJobs[1].name, 'Job 1');
    });
  });

  describe('analyzeSlowSteps', () => {
    function analyzeSlowSteps(metrics, limit = 5) {
      return metrics.stepDurations
        .sort((a, b) => b.duration - a.duration)
        .slice(0, limit);
    }

    test('should identify slowest steps correctly', () => {
      const metrics = {
        stepDurations: [
          { name: '📥 Checkout code', duration: 2000 },
          { name: '🧪 Run tests', duration: 45000 },
          { name: '⚙️ Setup Node.js', duration: 8000 },
          { name: '🔨 Build app', duration: 25000 },
          { name: '📤 Upload artifacts', duration: 3000 }
        ]
      };

      const slowSteps = analyzeSlowSteps(metrics, 3);
      
      assert.strictEqual(slowSteps.length, 3);
      assert.strictEqual(slowSteps[0].name, '🧪 Run tests');
      assert.strictEqual(slowSteps[0].duration, 45000);
      assert.strictEqual(slowSteps[1].name, '🔨 Build app');
      assert.strictEqual(slowSteps[1].duration, 25000);
      assert.strictEqual(slowSteps[2].name, '⚙️ Setup Node.js');
      assert.strictEqual(slowSteps[2].duration, 8000);
    });

    test('should respect limit parameter', () => {
      const metrics = {
        stepDurations: [
          { name: 'Step 1', duration: 1000 },
          { name: 'Step 2', duration: 2000 },
          { name: 'Step 3', duration: 3000 },
          { name: 'Step 4', duration: 4000 },
          { name: 'Step 5', duration: 5000 }
        ]
      };

      const slowSteps = analyzeSlowSteps(metrics, 2);
      
      assert.strictEqual(slowSteps.length, 2);
      assert.strictEqual(slowSteps[0].name, 'Step 5');
      assert.strictEqual(slowSteps[1].name, 'Step 4');
    });
  });

  describe('makeClickableLink', () => {
    function makeClickableLink(url, text = null) {
      const displayText = text || url;
      return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
    }

    test('should create clickable link with URL as text', () => {
      const url = 'https://github.com/owner/repo';
      const result = makeClickableLink(url);
      
      assert.strictEqual(result, `\u001b]8;;${url}\u0007${url}\u001b]8;;\u0007`);
    });

    test('should create clickable link with custom text', () => {
      const url = 'https://github.com/owner/repo/actions/runs/123';
      const text = 'View Job Logs';
      const result = makeClickableLink(url, text);
      
      assert.strictEqual(result, `\u001b]8;;${url}\u0007${text}\u001b]8;;\u0007`);
    });

    test('should fallback to URL when text is empty', () => {
      const url = 'https://example.com';
      const result = makeClickableLink(url, '');
      
      // Empty string falls back to URL due to || operator
      assert.strictEqual(result, `\u001b]8;;${url}\u0007${url}\u001b]8;;\u0007`);
    });
  });
}); 
