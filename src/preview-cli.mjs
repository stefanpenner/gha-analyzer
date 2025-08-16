#!/usr/bin/env node

/**
 * CLI Output Preview Script
 * Demonstrates the CLI output manager with different data scenarios
 * Useful for testing and previewing output without running the full analyzer
 */

import { CLIOutput } from './cli-output.mjs';

// Mock data for different scenarios
const mockScenarios = {
  singlePR: {
    name: 'Single PR Analysis',
    data: {
      urlCount: 1,
      totalRuns: 3,
      totalJobs: 8,
      totalSteps: 24,
      successRate: 100.0,
      jobSuccessRate: 87.5,
      maxConcurrency: 2,
      results: [
        {
          owner: 'stefanpenner',
          repo: 'gha-analyzer',
          type: 'pr',
          displayUrl: 'https://github.com/stefanpenner/gha-analyzer/pull/123',
          displayName: 'PR #123: Add new feature',
          branchName: 'feature/new-feature',
          urlIndex: 0,
          metrics: {
            totalRuns: 3,
            totalJobs: 8,
            jobTimeline: [
              { startTime: 1000, endTime: 5000, name: 'Setup', url: 'https://github.com/actions/setup-node' },
              { startTime: 2000, endTime: 8000, name: 'Build', url: 'https://github.com/actions/build' },
              { startTime: 3000, endTime: 6000, name: 'Test', url: 'https://github.com/actions/test' }
            ]
          },
          reviewEvents: [
            { type: 'shippit', reviewer: 'alice', time: '2024-01-15T10:00:00Z', url: 'https://github.com/stefanpenner/gha-analyzer/pull/123#pullrequestreview-123' },
            { type: 'merged', mergedBy: 'bob', time: '2024-01-15T14:30:00Z', url: 'https://github.com/stefanpenner/gha-analyzer/pull/123' }
          ]
        }
      ]
    }
  },

  multipleURLs: {
    name: 'Multiple URLs Analysis',
    data: {
      urlCount: 3,
      totalRuns: 8,
      totalJobs: 24,
      totalSteps: 72,
      successRate: 75.0,
      jobSuccessRate: 70.8,
      maxConcurrency: 4,
      results: [
        {
          owner: 'stefanpenner',
          repo: 'gha-analyzer',
          type: 'pr',
          displayUrl: 'https://github.com/stefanpenner/gha-analyzer/pull/123',
          displayName: 'PR #123: Add new feature',
          branchName: 'feature/new-feature',
          urlIndex: 0,
          metrics: { totalRuns: 3, totalJobs: 8, jobTimeline: [] },
          reviewEvents: []
        },
        {
          owner: 'stefanpenner',
          repo: 'gha-analyzer',
          type: 'commit',
          displayUrl: 'https://github.com/stefanpenner/gha-analyzer/commit/abc123',
          displayName: 'commit abc123: Fix bug',
          urlIndex: 1,
          metrics: { totalRuns: 2, totalJobs: 6, jobTimeline: [] },
          reviewEvents: []
        },
        {
          owner: 'stefanpenner',
          repo: 'gha-analyzer',
          type: 'pr',
          displayUrl: 'https://github.com/stefanpenner/gha-analyzer/pull/124',
          displayName: 'PR #124: Performance improvements',
          branchName: 'perf/improvements',
          urlIndex: 2,
          metrics: { totalRuns: 3, totalJobs: 10, jobTimeline: [] },
          reviewEvents: []
        }
      ]
    }
  },

  withPendingJobs: {
    name: 'Analysis with Pending Jobs',
    data: {
      urlCount: 2,
      totalRuns: 4,
      totalJobs: 12,
      totalSteps: 36,
      successRate: 50.0,
      jobSuccessRate: 58.3,
      maxConcurrency: 3,
      pendingJobs: [
        { url: 'https://github.com/actions/build', name: 'Build Job', status: 'running', sourceName: 'PR #123' },
        { url: 'https://github.com/actions/test', name: 'Integration Tests', status: 'queued', sourceName: 'PR #123' }
      ],
      results: [
        {
          owner: 'stefanpenner',
          repo: 'gha-analyzer',
          type: 'pr',
          displayUrl: 'https://github.com/stefanpenner/gha-analyzer/pull/123',
          displayName: 'PR #123: Add new feature',
          branchName: 'feature/new-feature',
          urlIndex: 0,
          metrics: { totalRuns: 2, totalJobs: 6, jobTimeline: [] },
          reviewEvents: []
        },
        {
          owner: 'stefanpenner',
          repo: 'gha-analyzer',
          type: 'commit',
          displayUrl: 'https://github.com/stefanpenner/gha-analyzer/commit/abc123',
          displayName: 'commit abc123: Fix bug',
          urlIndex: 1,
          metrics: { totalRuns: 2, totalJobs: 6, jobTimeline: [] },
          reviewEvents: []
        }
      ]
    }
  },

  errorScenario: {
    name: 'Error Handling',
    data: {
      errors: [
        'No workflow runs found for URL https://github.com/owner/repo/pull/999',
        'GitHub API rate limit exceeded',
        'Repository access denied'
      ],
      warnings: [
        'Some jobs are still pending',
        'Timeline data may be incomplete'
      ]
    }
  }
};

// Preview functions
function previewScenario(scenarioName) {
  const scenario = mockScenarios[scenarioName];
  if (!scenario) {
    console.error(`Unknown scenario: ${scenarioName}`);
    return;
  }

  console.log(`\n${'='.repeat(80)}`);
  console.log(`📋 Preview: ${scenario.name}`);
  console.log(`${'='.repeat(80)}`);

  const output = new CLIOutput();
  output.startCapture();

  if (scenarioName === 'errorScenario') {
    // Show error scenario
    scenario.data.errors.forEach(error => output.showError(error));
    scenario.data.warnings.forEach(warning => output.showWarning(warning));
  } else {
    // Show normal analysis scenario
    const data = scenario.data;
    
    output.showTraceGeneration(data.totalJobs * 3, null);
    output.showReportHeader();
    output.showAnalysisSummary(
      data.urlCount,
      data.totalRuns,
      data.totalJobs,
      data.totalSteps,
      data.successRate,
      data.jobSuccessRate,
      data.maxConcurrency
    );

    if (data.pendingJobs) {
      output.showPendingJobs(data.pendingJobs);
    }

    if (data.results.length > 1) {
      output.showCombinedAnalysis(data.results);
    }

    output.showRunSummary(data.results);
    output.showPipelineTimelines(data.results);
  }

  const captured = output.stopCapture();
  console.log(captured.stderr);
}

function showAllScenarios() {
  console.log('Available CLI Output Preview Scenarios:');
  console.log('');
  
  Object.keys(mockScenarios).forEach((key, index) => {
    const scenario = mockScenarios[key];
    console.log(`${index + 1}. ${key} - ${scenario.name}`);
  });
  
  console.log('\nUsage:');
  console.log('  node src/preview-cli.mjs <scenario>');
  console.log('  node src/preview-cli.mjs all');
  console.log('');
  console.log('Examples:');
  console.log('  node src/preview-cli.mjs singlePR');
  console.log('  node src/preview-cli.mjs multipleURLs');
  console.log('  node src/preview-cli.mjs withPendingJobs');
  console.log('  node src/preview-cli.mjs errorScenario');
}

function showInteractivePreview() {
  console.log('\n🎯 Interactive CLI Output Preview');
  console.log('This demonstrates the CLI output manager capabilities.');
  console.log('');
  
  const output = new CLIOutput();
  
  // Show progress simulation
  console.log('📊 Progress Bar Simulation:');
  output.startCapture();
  
  const spinnerFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];
  for (let i = 0; i < 5; i++) {
    const spinner = spinnerFrames[i % spinnerFrames.length];
    const progress = `URL ${i + 1}/5 █${'█'.repeat(i)}${'░'.repeat(4 - i)}  ${((i + 1) / 5 * 100).toFixed(0)}%`;
    const runs = ` | Runs ${i * 2}/${i * 2 + 2}`;
    const timing = `⏱️ ${i * 2}s`;
    
    output.showProgress(spinner, progress, runs, timing);
    
    // Simulate some delay
    const captured = output.stopCapture();
    process.stdout.write(captured.stderr);
    
    if (i < 4) {
      output.startCapture();
      // Small delay to show progress
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 200);
    }
  }
  
  // Show completion
  output.showProgressCompletion(5, 10000, 25);
  const finalCapture = output.stopCapture();
  console.log(finalCapture.stderr);
  
  // Show some sample output
  console.log('\n📋 Sample Report Output:');
  output.startCapture();
  output.showInfo('Starting analysis...');
  output.showSuccess('Analysis completed successfully!');
  output.showWarning('Some jobs are still pending');
  output.showError('Failed to fetch some data');
  
  const sampleOutput = output.stopCapture();
  console.log(sampleOutput.stderr);
}

// Main execution
const scenario = process.argv[2];

if (!scenario || scenario === 'help') {
  showAllScenarios();
} else if (scenario === 'all') {
  Object.keys(mockScenarios).forEach(key => {
    previewScenario(key);
  });
} else if (scenario === 'interactive') {
  showInteractivePreview();
} else if (mockScenarios[scenario]) {
  previewScenario(scenario);
} else {
  console.error(`Unknown scenario: ${scenario}`);
  showAllScenarios();
}

