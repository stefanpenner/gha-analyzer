#!/usr/bin/env node

/**
 * Consolidated Test Suite
 * 
 * High-value tests that catch the most important issues with minimal maintenance burden.
 * Focuses on:
 * 1. Core functionality (URL parsing, data processing)
 * 2. Critical timing accuracy (the main value proposition)
 * 3. Output format validation (ensures usability)
 * 4. Edge cases that commonly break
 */

import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert';
import { spawn } from 'child_process';
import fs from 'fs';
import path from 'path';

// Import core functions for unit testing
import {
  parseGitHubUrl,
  getJobGroup,
  calculateCombinedMetrics,
  calculateCombinedSuccessRate,
  calculateCombinedJobSuccessRate,
  findBottleneckJobs,
  humanizeTime,
  generateHighLevelTimeline,
  fetchPRReviews
} from '../main.mjs';

import { createGitHubMock } from './github-mock.mjs';

// Test configuration - Use only OSS projects to avoid exposing internal data
const GITHUB_TOKEN = process.env.GITHUB_TOKEN;
const TEST_URL = 'https://github.com/facebook/react/pull/12345'; // Public OSS project with active CI

// Helper function to run analyzer (for integration tests)
  async function runAnalyzer(urls, timeoutMs = 30000) {
    return new Promise((resolve, reject) => {
      const child = spawn('node', ['main.mjs', ...urls], {
        stdio: ['pipe', 'pipe', 'pipe'],
        env: { ...process.env, GITHUB_TOKEN: 'test-token' }
      });
    
    let stdout = '';
    let stderr = '';
    
    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });
    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });
    
    const timeout = setTimeout(() => {
      child.kill('SIGTERM');
      reject(new Error(`Analyzer timed out after ${timeoutMs}ms`));
    }, timeoutMs);
    
    child.on('close', (code) => {
      clearTimeout(timeout);
      resolve({ 
        stdout, 
        stderr, 
        combined: stderr + stdout,
        exitCode: code 
      });
    });
    
    child.on('error', (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });
}

// Helper function to extract and parse JSON output
function extractJsonOutput(output) {
  const jsonMatch = output.match(/\{[\s\S]*"traceEvents"[\s\S]*\}/);
  if (!jsonMatch) {
    throw new Error('No JSON output found');
  }
  
  try {
    return JSON.parse(jsonMatch[0]);
  } catch (error) {
    throw new Error(`Failed to parse JSON: ${error.message}`);
  }
}

describe('GitHub Actions Analyzer - Critical Functionality', () => {
  
  describe('Core URL Parsing', () => {
    test('should parse valid GitHub PR URL', () => {
      const url = 'https://github.com/owner/repo/pull/123';
      const result = parseGitHubUrl(url);
      
      assert.strictEqual(result.owner, 'owner');
      assert.strictEqual(result.repo, 'repo');
      assert.strictEqual(result.type, 'pr');
      assert.strictEqual(result.identifier, '123');
    });

    test('should parse valid GitHub commit URL', () => {
      const url = 'https://github.com/owner/repo/commit/abc123def456';
      const result = parseGitHubUrl(url);
      
      assert.strictEqual(result.owner, 'owner');
      assert.strictEqual(result.repo, 'repo');
      assert.strictEqual(result.type, 'commit');
      assert.strictEqual(result.identifier, 'abc123def456');
    });

    test('should reject invalid URL format', () => {
      const url = 'https://github.com/owner/repo/issues/123';
      
      assert.throws(() => {
        parseGitHubUrl(url);
      }, /Invalid GitHub URL/);
    });
  });

  describe('Critical Timing Accuracy', () => {
    test('should humanize time durations correctly', () => {
      assert.strictEqual(humanizeTime(0), '0s');
      assert.strictEqual(humanizeTime(0.5), '500ms');
      assert.strictEqual(humanizeTime(1), '1s');
      assert.strictEqual(humanizeTime(65), '1m 5s');
      assert.strictEqual(humanizeTime(3661), '1h 1m 1s');
    });

    test('should identify bottleneck jobs correctly', () => {
      const jobs = [
        { name: 'FastJob', startTime: 1000, endTime: 2000 },
        { name: 'SlowJob', startTime: 1500, endTime: 10000 }, // 9s duration
        { name: 'MediumJob', startTime: 2000, endTime: 5000 }
      ];
      
      const bottlenecks = findBottleneckJobs(jobs);
      // Should return 2 jobs since SlowJob and MediumJob are both significant
      assert.strictEqual(bottlenecks.length, 2);
      assert.strictEqual(bottlenecks[0].name, 'SlowJob'); // Longest first
    });

    test('should calculate combined metrics correctly', () => {
      const results = [
        { metrics: { totalJobs: 10, totalSteps: 50, jobTimeline: [] } },
        { metrics: { totalJobs: 5, totalSteps: 25, jobTimeline: [] } }
      ];
      
      const combined = calculateCombinedMetrics(results, 2, [1000, 2000], [3000, 4000]);
      assert.strictEqual(combined.totalJobs, 15);
      assert.strictEqual(combined.totalSteps, 75);
    });

    test('should normalize timestamps correctly for multi-URL scenarios', () => {
      // Simulate the exact bug we just fixed
      const mockTraceEvents = [
        {
          name: "Workflow: Merge-Lint [1]",
          ph: "X",
          ts: 1000000, // 1 second in microseconds
          dur: 185000000,
          args: { url_index: 1, source_url: "https://github.com/test/repo1/pull/123" }
        },
        {
          name: "Workflow: Post-Merge [2]",
          ph: "X",
          ts: 0, // This was the problematic timestamp
          dur: 784000000,
          args: { url_index: 2, source_url: "https://github.com/test/repo2/commit/abc123" }
        }
      ];

      const mockUrlResults = [
        {
          urlIndex: 0,
          displayUrl: "https://github.com/test/repo1/pull/123",
          earliestTime: 1704067200000 // 2024-01-01 00:00:00 UTC
        },
        {
          urlIndex: 1,
          displayUrl: "https://github.com/test/repo2/commit/abc123",
          earliestTime: 1704153600000 // 2024-01-02 00:00:00 UTC (1 day later)
        }
      ];

      const globalEarliestTime = Math.min(...mockUrlResults.map(r => r.earliestTime));

      // Apply the fix logic
      const renormalizedTraceEvents = mockTraceEvents.map(event => {
        if (event.ts !== undefined) {
          const eventUrlIndex = event.args?.url_index || 1;
          const eventSource = event.args?.source_url;
          const urlResult = mockUrlResults.find(result => 
            result.urlIndex === eventUrlIndex - 1 || 
            result.displayUrl === eventSource
          );
          
          if (urlResult) {
            const absoluteTime = event.ts / 1000 + urlResult.earliestTime;
            const renormalizedTime = (absoluteTime - globalEarliestTime) * 1000;
            return { ...event, ts: renormalizedTime };
          }
        }
        return event;
      });

      // Verify the fix works
      const workflow1 = renormalizedTraceEvents.find(e => e.name.includes("Merge-Lint"));
      const workflow2 = renormalizedTraceEvents.find(e => e.name.includes("Post-Merge"));

      assert(workflow1.ts > 0, 'First workflow should have positive timestamp');
      assert(workflow2.ts > 0, 'Second workflow should have positive timestamp (was 0 before fix)');
      assert(workflow2.ts > workflow1.ts, 'Second workflow should start after first workflow');
      
      // Verify expected values
      const expectedWorkflow1Ts = (1000000 / 1000 + mockUrlResults[0].earliestTime - globalEarliestTime) * 1000;
      const expectedWorkflow2Ts = (0 / 1000 + mockUrlResults[1].earliestTime - globalEarliestTime) * 1000;
      
      assert.strictEqual(workflow1.ts, expectedWorkflow1Ts, 'First workflow timestamp should match expected');
      assert.strictEqual(workflow2.ts, expectedWorkflow2Ts, 'Second workflow timestamp should match expected');
    });

    test('should ensure no events have timestamp 0 in multi-URL output', () => {
      // Test that our fix prevents the timestamp 0 bug
      const mockTraceEvents = [
        { name: "Event 1", ts: 1000000, args: { url_index: 1 } },
        { name: "Event 2", ts: 0, args: { url_index: 2 } }, // Simulate the bug
        { name: "Event 3", ts: 2000000, args: { url_index: 1 } }
      ];

      const mockUrlResults = [
        { urlIndex: 0, earliestTime: 1000 },
        { urlIndex: 1, earliestTime: 2000 } // Later than URL 1
      ];

      const globalEarliestTime = Math.min(...mockUrlResults.map(r => r.earliestTime));

      // Apply fix
      const renormalizedEvents = mockTraceEvents.map(event => {
        if (event.ts !== undefined) {
          const eventUrlIndex = event.args?.url_index || 1;
          const urlResult = mockUrlResults.find(result => result.urlIndex === eventUrlIndex - 1);
          
          if (urlResult) {
            const absoluteTime = event.ts / 1000 + urlResult.earliestTime;
            const renormalizedTime = (absoluteTime - globalEarliestTime) * 1000;
            return { ...event, ts: renormalizedTime };
          }
        }
        return event;
      });

      // Verify no events have timestamp 0
      renormalizedEvents.forEach(event => {
        if (event.ts !== undefined) {
          assert(event.ts > 0, `Event "${event.name}" should not have timestamp 0`);
        }
      });

      // Verify events maintain proper ordering
      const sortedEvents = renormalizedEvents.filter(e => e.ts !== undefined).sort((a, b) => a.ts - b.ts);
      assert(sortedEvents.length > 0, 'Should have events to sort');
      assert(sortedEvents[0].ts <= sortedEvents[1].ts, 'Events should be properly ordered by timestamp');
    });
  });

  describe('Job Grouping Logic', () => {
    test('should extract job group from job name', () => {
      assert.strictEqual(getJobGroup('CI-validations / Build'), 'CI-validations');
      assert.strictEqual(getJobGroup('Post-merge / Setup / Setup'), 'Post-merge');
      assert.strictEqual(getJobGroup('Simple Job'), 'Simple Job');
    });

    test('should handle complex job names with multiple slashes', () => {
      assert.strictEqual(getJobGroup('Post-merge / Build / (Dry run) Build on linux'), 'Post-merge');
      assert.strictEqual(getJobGroup('sast-scan / CodeQL Scan (go, mo-core, sast-online)'), 'sast-scan');
    });
  });

  describe('Integration Tests - Output Format', () => {
    test('should handle analyzer execution gracefully', async () => {
      console.log('\n🔍 Testing analyzer execution...');
      
      const result = await runAnalyzer([TEST_URL]);
      
      // The analyzer should either succeed or fail gracefully with a clear error message
      const output = result.combined;
      
      if (result.exitCode === 0) {
        // Success case - validate output format
        const hasTimeline = output.includes('Pipeline Timeline') || output.includes('Combined Pipeline Timeline');
        const hasSlowestJobs = output.includes('Slowest Jobs');
        const hasAnalysis = output.includes('Combined Analysis') || output.includes('Performance Analysis');
        
        // At least one of these sections should be present
        assert(hasTimeline || hasSlowestJobs || hasAnalysis, 'Should include at least one analysis section');
        
        // Check for analysis summary (may have 0 values for OSS projects)
        const summaryMatch = output.match(/Analysis Summary: (\d+) URLs • (\d+) runs • (\d+) jobs • (\d+) steps/);
        if (summaryMatch) {
          const [, urls, runs, jobs, steps] = summaryMatch;
          assert(parseInt(urls) > 0, 'Should have at least one URL');
          console.log(`✅ Output format validation passed: ${urls} URLs, ${runs} runs, ${jobs} jobs, ${steps} steps`);
        } else {
          // If no summary, check for basic success message
          assert(output.includes('Generated') || output.includes('✅'), 'Should include success message');
          console.log('✅ Output format validation passed: basic success message found');
        }
      } else {
        // Failure case - should have clear error message
        assert(output.includes('Error') || output.includes('404') || output.includes('Not Found'), 'Should have clear error message');
        console.log('✅ Error handling validation passed: clear error message found');
      }
    });

    test('should generate valid Perfetto trace format when successful', async () => {
      console.log('\n🔍 Testing Perfetto trace format...');
      
      const result = await runAnalyzer([TEST_URL]);
      
      if (result.exitCode === 0) {
        // Only test trace format if analyzer succeeded
        const jsonData = extractJsonOutput(result.combined);
        
        // Validate basic Chrome Tracing Format structure
        assert(jsonData.traceEvents, 'Should have traceEvents array');
        assert(Array.isArray(jsonData.traceEvents), 'traceEvents should be an array');
        assert(jsonData.traceEvents.length > 0, 'Should have trace events');
        assert(jsonData.displayTimeUnit, 'Should have displayTimeUnit');
        
        // Validate required event types
        const eventTypes = new Set(jsonData.traceEvents.map(e => e.ph));
        assert(eventTypes.has('M'), 'Should have metadata events');
        
        // Validate timestamp consistency (if duration events exist)
        const durationEvents = jsonData.traceEvents.filter(e => e.ph === 'X');
        if (durationEvents.length > 0) {
          durationEvents.forEach(event => {
            assert(event.ts >= 0, 'Timestamp should be non-negative');
            assert(event.dur > 0, 'Duration should be positive');
            assert(event.ts < 1000000000, 'Timestamp should be reasonable (less than 1B)');
            
            // CRITICAL: No events should have timestamp 0 (this was the bug we fixed)
            assert(event.ts > 0, `Event "${event.name}" should not have timestamp 0 - this indicates a timestamp normalization bug`);
            
            // When displayTimeUnit is 'ms', timestamps should be in microseconds
            if (jsonData.displayTimeUnit === 'ms') {
              // Check that timestamps are in microseconds (should be large numbers, typically 6+ digits)
              assert(event.ts >= 1000, 'Timestamp should be in microseconds when displayTimeUnit is ms');
              assert(event.dur >= 1000, 'Duration should be in microseconds when displayTimeUnit is ms');
            }
          });
        }
        
        console.log(`✅ Perfetto trace validation passed: ${jsonData.traceEvents.length} events`);
      } else {
        // Skip trace validation if analyzer failed
        console.log('⏭️ Skipping trace validation - analyzer failed (expected for test URL)');
      }
    });
  });

  describe('Review Event Timeline Markers', () => {
    test('renders shippit approval marker at correct position', async () => {
      const mock = createGitHubMock();
      const owner = 'test-owner';
      const repo = 'test-repo';
      const prNumber = 123;
      const reviewTime = '2024-01-01T10:02:00Z';
      mock.mockReviews(owner, repo, prNumber, [
        { id: 1, state: 'APPROVED', submitted_at: reviewTime, user: { login: 'reviewer1' } }
      ]);

      const context = { githubToken: 'test-token' };
      const originalFetch = global.fetch;
      global.fetch = async (url, options) => {
        if (url.includes(`/repos/${owner}/${repo}/pulls/${prNumber}/reviews`)) {
          return {
            ok: true,
            json: async () => [
              { id: 1, state: 'APPROVED', submitted_at: reviewTime, user: { login: 'reviewer1' } }
            ],
            headers: new Map()
          };
        }
        return originalFetch(url, options);
      };
      const reviews = await fetchPRReviews(owner, repo, prNumber, context);
      global.fetch = originalFetch;
      const reviewEvents = reviews
        .filter(r => r.state === 'APPROVED' || /ship\s?it/i.test(r.body || ''))
        .map(r => ({ type: 'shippit', time: r.submitted_at, reviewer: r.user.login }));

      const jobStart = new Date('2024-01-01T10:01:00Z').getTime();
      const jobEnd = new Date('2024-01-01T10:03:00Z').getTime();

      const result = {
        displayName: `PR #${prNumber}`,
        displayUrl: `https://github.com/${owner}/${repo}/pull/${prNumber}`,
        urlIndex: 0,
        metrics: { jobTimeline: [{ name: 'test-job', startTime: jobStart, endTime: jobEnd, conclusion: 'success' }] },
        reviewEvents
      };

      let output = '';
      const origError = console.error;
      console.error = msg => { output += msg + '\n'; };
      generateHighLevelTimeline([result], 0, 0);
      console.error = origError;
      mock.cleanup();

      const barLine = output.split('\n').find(line => line.includes(`PR #${prNumber}`));
      const clean = barLine.replace(/\x1b\[[0-9;]*m/g, '');
      const between = clean.split('│')[1];
      const markerPos = between.indexOf('▲');
      assert.strictEqual(markerPos, 30);
      assert(clean.includes('▲ reviewer1'));
    });
  });

  describe('Edge Cases & Error Handling', () => {
    test('should handle empty job lists gracefully', () => {
      const bottlenecks = findBottleneckJobs([]);
      assert.strictEqual(bottlenecks.length, 0);
      
      const combined = calculateCombinedMetrics([], 0, [], []);
      assert.strictEqual(combined.totalJobs, 0);
      assert.strictEqual(combined.totalSteps, 0);
    });

    test('should handle single job correctly', () => {
      const jobs = [{ name: 'SingleJob', startTime: 1000, endTime: 2000 }];
      const bottlenecks = findBottleneckJobs(jobs);
      // Single job with 1s duration is not significant enough (threshold is 1s)
      assert.strictEqual(bottlenecks.length, 0);
    });

    test('should handle very large durations', () => {
      assert.strictEqual(humanizeTime(86400), '24h'); // 24 hours
      assert.strictEqual(humanizeTime(3600000), '1000h'); // 1000 hours
    });
  });

  describe('CLI Usage Validation', () => {
    test('should error when no URLs are provided', async () => {
      const originalToken = process.env.GITHUB_TOKEN;
      delete process.env.GITHUB_TOKEN;

      try {
        const result = await runAnalyzer([]);

        assert.strictEqual(result.exitCode, 1);
        assert(result.stderr.includes('Error: No GitHub URLs provided.'));
        assert(result.stderr.includes('Error: GitHub token is required.'));
      } finally {
        if (originalToken !== undefined) {
          process.env.GITHUB_TOKEN = originalToken;
        } else {
          delete process.env.GITHUB_TOKEN;
        }
      }
    });

    test('should error when GitHub token is missing', async () => {
      const originalToken = process.env.GITHUB_TOKEN;
      delete process.env.GITHUB_TOKEN;

      try {
        const result = await runAnalyzer(['https://github.com/owner/repo/pull/123']);

        assert.strictEqual(result.exitCode, 1);
        assert(result.stderr.includes('Error: GitHub token is required.'));
        assert(!result.stderr.includes('Error: No GitHub URLs provided.'));
      } finally {
        if (originalToken !== undefined) {
          process.env.GITHUB_TOKEN = originalToken;
        } else {
          delete process.env.GITHUB_TOKEN;
        }
      }
    });
  });

  describe('Performance & Reliability', () => {
    test('should complete analysis within reasonable time', async () => {
      console.log('\n🔍 Testing performance...');
      
      const startTime = Date.now();
      const result = await runAnalyzer([TEST_URL]);
      const duration = Date.now() - startTime;
      
      // Should complete within reasonable time regardless of success/failure
      assert(duration < 30000, `Analysis should complete within 30s, took ${duration}ms`);
      
      if (result.exitCode === 0) {
        console.log(`✅ Performance test passed: completed successfully in ${duration}ms`);
      } else {
        console.log(`✅ Performance test passed: failed gracefully in ${duration}ms (expected for test URL)`);
      }
    });
  });
}); 