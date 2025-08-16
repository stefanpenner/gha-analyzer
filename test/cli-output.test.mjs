/**
 * Tests for CLI Output Manager
 * Tests all output functions and preview capabilities
 */

import { test, describe } from 'node:test';
import assert from 'node:assert';
import { CLIOutput } from '../src/cli-output.mjs';

describe('CLI Output Manager', () => {
  describe('Basic Output Functions', () => {
    test('should format text colors correctly', () => {
      const output = new CLIOutput();
      const gray = output.grayText('test');
      const blue = output.blueText('test');
      const yellow = output.yellowText('test');
      const red = output.redText('test');
      const green = output.greenText('test');

      assert(gray.includes('\u001b[90m'));
      assert(blue.includes('\u001b[34m'));
      assert(yellow.includes('\u001b[33m'));
      assert(red.includes('\u001b[31m'));
      assert(green.includes('\u001b[32m'));
    });

    test('should create clickable links', () => {
      const output = new CLIOutput();
      const link = output.makeClickableLink('https://example.com', 'Example');
      assert(link.includes('https://example.com'));
      assert(link.includes('Example'));
      assert(link.includes('\u001b]8;;'));
    });

    test('should use URL as text when no text provided', () => {
      const output = new CLIOutput();
      const link = output.makeClickableLink('https://example.com');
      assert(link.includes('https://example.com'));
    });
  });

  describe('Progress Output', () => {
    test('should show progress line', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showProgress('⠋', 'URL 1/2 ██████████░░░░░░░░░░ 50.0%', ' | Runs 0/1', '⏱️ 1.2s');
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('⠋ Processing:'));
      assert(captured.stderr.includes('URL 1/2'));
      assert(captured.stderr.includes('Runs 0/1'));
      assert(captured.stderr.includes('⏱️ 1.2s'));
    });

    test('should show progress completion', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showProgressCompletion(2, 5000, 10);
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('✅ Analysis Complete!'));
      assert(captured.stderr.includes('Processed 2 URLs'));
      assert(captured.stderr.includes('Total workflow runs analyzed: 10'));
    });
  });

  describe('Report Output', () => {
    test('should show trace generation info', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showTraceGeneration(25, null);
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('Generated 25 trace events'));
      assert(captured.stderr.includes('Use --perfetto='));
    });

    test('should show trace generation with file', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showTraceGeneration(25, 'trace.json');
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('Generated 25 trace events'));
      assert(captured.stderr.includes('Open in Perfetto.dev'));
    });

    test('should show report header', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showReportHeader();
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('='.repeat(80)));
      assert(captured.stderr.includes('GitHub Actions Performance Report'));
      assert(captured.stderr.includes('https://ui.perfetto.dev'));
    });

    test('should show analysis summary', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showAnalysisSummary(2, 5, 12, 45, 80.0, 75.0, 3);
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('Analysis Summary: 2 URLs'));
      assert(captured.stderr.includes('5 runs'));
      assert(captured.stderr.includes('12 jobs'));
      assert(captured.stderr.includes('45 steps'));
      assert(captured.stderr.includes('Success Rate: 80% workflows'));
      assert(captured.stderr.includes('75% jobs'));
      assert(captured.stderr.includes('Peak Concurrency: 3'));
    });
  });

  describe('Job and Run Output', () => {
    test('should show pending jobs', () => {
      const output = new CLIOutput();
      const pendingJobs = [
        { url: 'https://example.com/job1', name: 'Build Job', status: 'running', sourceName: 'PR #123' },
        { url: 'https://example.com/job2', name: 'Test Job', status: 'queued', sourceName: 'PR #123' }
      ];

      output.startCapture();
      output.showPendingJobs(pendingJobs);
      const captured = output.stopCapture();
      
      // The text is there but split by emoji and color codes
      assert(captured.stderr.includes('Pending Jobs Detected'));
      assert(captured.stderr.includes('2 jobs still running'));
      assert(captured.stderr.includes('Build Job'));
      assert(captured.stderr.includes('Test Job'));
      assert(captured.stderr.includes('Note: Timeline shows current progress'));
    });

    test('should show combined analysis', () => {
      const output = new CLIOutput();
      const sortedResults = [
        { owner: 'owner1', repo: 'repo1', type: 'pr', displayUrl: 'https://github.com/owner1/repo1/pull/123', displayName: 'PR #123', branchName: 'feature-branch', urlIndex: 0 },
        { owner: 'owner2', repo: 'repo2', type: 'commit', displayUrl: 'https://github.com/owner2/repo2/commit/abc123', displayName: 'commit abc123', urlIndex: 1 }
      ];

      output.startCapture();
      output.showCombinedAnalysis(sortedResults);
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('Combined Analysis'));
      assert(captured.stderr.includes('Included URLs (ordered by start time)'));
      assert(captured.stderr.includes('PR #123'));
      assert(captured.stderr.includes('commit abc123'));
      assert(captured.stderr.includes('Combined Pipeline Timeline'));
    });

    test('should show run summary', () => {
      const output = new CLIOutput();
      const results = [
        {
          urlIndex: 0,
          displayName: 'PR #123',
          metrics: {
            totalRuns: 2,
            jobTimeline: [
              { startTime: 1000, endTime: 5000 },
              { startTime: 2000, endTime: 6000 }
            ]
          },
          reviewEvents: [
            { type: 'shippit' },
            { type: 'merged' }
          ]
        }
      ];

      output.startCapture();
      output.showRunSummary(results);
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('Run Summary:'));
      assert(captured.stderr.includes('[1] PR #123: runs=2'));
      assert(captured.stderr.includes('wall='));
      assert(captured.stderr.includes('compute='));
      assert(captured.stderr.includes('approvals=2'));
      assert(captured.stderr.includes('merged=yes'));
    });
  });

  describe('Utility Functions', () => {
    test('should humanize time correctly', () => {
      const output = new CLIOutput();
      assert.strictEqual(output.humanizeTime(0.5), '500ms');
      assert.strictEqual(output.humanizeTime(30), '30.0s');
      assert.strictEqual(output.humanizeTime(90), '1.5m');
      assert.strictEqual(output.humanizeTime(7200), '2.0h');
    });

    test('should format duration correctly', () => {
      const output = new CLIOutput();
      assert.strictEqual(output.formatDuration(500), '500ms');
      assert.strictEqual(output.formatDuration(30000), '30.0s');
      assert.strictEqual(output.formatDuration(180000), '3.0m');
      assert.strictEqual(output.formatDuration(7200000), '2.0h');
    });
  });

  describe('Error and Warning Output', () => {
    test('should show errors', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showError('Something went wrong');
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('❌'));
      assert(captured.stderr.includes('Something went wrong'));
    });

    test('should show warnings', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showWarning('Be careful');
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('⚠️'));
      assert(captured.stderr.includes('Be careful'));
    });

    test('should show info messages', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showInfo('Just FYI');
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('ℹ️'));
      assert(captured.stderr.includes('Just FYI'));
    });

    test('should show success messages', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showSuccess('Great job!');
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('✅'));
      assert(captured.stderr.includes('Great job!'));
    });
  });

  describe('Capture and Preview', () => {
    test('should capture output correctly', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showInfo('Test message');
      const captured = output.stopCapture();
      
      assert.strictEqual(captured.stderr, 'ℹ️  Test message\n');
      assert.strictEqual(captured.stdout, '');
    });

    test('should preview with mock data', () => {
      const output = new CLIOutput();
      const preview = output.previewWithMockData();
      
      assert(preview.stderr.includes('Generated 25 trace events'));
      assert(preview.stderr.includes('Analysis Summary: 2 URLs'));
      assert(preview.stderr.includes('PR #123'));
      assert(preview.stderr.includes('commit abc123'));
    });

    test('should clear capture buffer', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showInfo('Test message');
      output.clearCapture();
      const captured = output.stopCapture();
      
      assert.strictEqual(captured.stderr, '');
      assert.strictEqual(captured.stdout, '');
    });
  });

  describe('Edge Cases', () => {
    test('should handle empty pending jobs', () => {
      const output = new CLIOutput();
      output.startCapture();
      output.showPendingJobs([]);
      const captured = output.stopCapture();
      
      assert.strictEqual(captured.stderr, '');
    });

    test('should handle single URL in combined analysis', () => {
      const output = new CLIOutput();
      const singleResult = [
        { owner: 'owner1', repo: 'repo1', type: 'pr', displayUrl: 'https://github.com/owner1/repo1/pull/123', displayName: 'PR #123', branchName: 'feature-branch', urlIndex: 0 }
      ];

      output.startCapture();
      output.showCombinedAnalysis(singleResult);
      const captured = output.stopCapture();
      
      assert.strictEqual(captured.stderr, '');
    });

    test('should handle missing metrics gracefully', () => {
      const output = new CLIOutput();
      const results = [
        {
          urlIndex: 0,
          displayName: 'PR #123',
          metrics: {},
          reviewEvents: []
        }
      ];

      output.startCapture();
      output.showRunSummary(results);
      const captured = output.stopCapture();
      
      assert(captured.stderr.includes('[1] PR #123: runs=0'));
      assert(captured.stderr.includes('wall=0ms'));
      assert(captured.stderr.includes('compute=0ms'));
    });
  });
});
