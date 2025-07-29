import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert';
import { spawn } from 'node:child_process';
import { createGitHubMock } from './github-mock.mjs';

describe('GitHub Actions Profiler', () => {
  let githubMock;

  beforeEach(() => {
    githubMock = createGitHubMock();
  });

  afterEach(() => {
    githubMock.cleanup();
  });

  describe('Successful Pipeline Analysis', () => {
    test('should analyze a successful pipeline and generate trace events', async () => {
      // Setup mock
      githubMock.mockSuccessfulPipeline('test-owner', 'test-repo', 123);

      // Run the script
      const result = await runScript('https://github.com/test-owner/test-repo/pull/123', 'fake-token');

      // Verify the output
      assert.strictEqual(result.exitCode, 0, 'Script should exit successfully');
      
      const output = JSON.parse(result.stdout);
      assert.strictEqual(output.displayTimeUnit, 'ms');
      assert(Array.isArray(output.traceEvents), 'Should have traceEvents array');
      assert(output.traceEvents.length > 0, 'Should generate trace events');
      
      // Verify trace event structure
      const workflowEvent = output.traceEvents.find(e => e.name?.startsWith('Workflow:'));
      assert(workflowEvent, 'Should have workflow event');
      assert.strictEqual(workflowEvent.ph, 'X', 'Workflow should be complete event');
      assert(typeof workflowEvent.ts === 'number', 'Should have timestamp');
      assert(typeof workflowEvent.dur === 'number', 'Should have duration');
      
      const jobEvents = output.traceEvents.filter(e => e.name?.startsWith('Job:'));
      assert(jobEvents.length >= 2, 'Should have multiple job events');
      
      const stepEvents = output.traceEvents.filter(e => e.cat?.startsWith('step_'));
      assert(stepEvents.length >= 4, 'Should have step events');
      
      // Verify metadata
      assert(output.otherData, 'Should have metadata');
      assert.strictEqual(output.otherData.pr_number, 123);
      assert(output.otherData.head_sha, 'Should have head SHA');
      
      // Verify console output contains expected report
      assert(result.stderr.includes('GitHub Actions Performance Report'), 'Should show report header');
      assert(result.stderr.includes('test-owner/test-repo'), 'Should show repository');
      assert(result.stderr.includes('Pull Request: #123'), 'Should show PR number');
      assert(result.stderr.includes('Success Rate:'), 'Should show success rate');
    });

    test('should handle multiple workflow runs', async () => {
      githubMock.mockMultipleRuns('test-owner', 'test-repo', 125);

      const result = await runScript('https://github.com/test-owner/test-repo/pull/125', 'fake-token');
      
      assert.strictEqual(result.exitCode, 0);
      
      const output = JSON.parse(result.stdout);
      const workflowEvents = output.traceEvents.filter(e => e.name?.startsWith('Workflow:'));
      assert(workflowEvents.length >= 2, 'Should have multiple workflow events');
      
      // Verify threads are different
      const threadIds = new Set(workflowEvents.map(e => e.tid));
      assert(threadIds.size >= 2, 'Should use different threads for different runs');
    });
  });

  describe('Failed Pipeline Analysis', () => {
    test('should analyze a failed pipeline correctly', async () => {
      githubMock.mockFailedPipeline('test-owner', 'test-repo', 124);

      const result = await runScript('https://github.com/test-owner/test-repo/pull/124', 'fake-token');
      
      assert.strictEqual(result.exitCode, 0, 'Script should still succeed for failed pipelines');
      
      const output = JSON.parse(result.stdout);
      
      // Should still generate events
      assert(output.traceEvents.length > 0, 'Should generate events for failed pipeline');
      
      // Check for failure indicators in events
      const failedStepEvents = output.traceEvents.filter(e => 
        e.name?.includes('❌') || e.args?.conclusion === 'failure'
      );
      assert(failedStepEvents.length > 0, 'Should mark failed steps');
      
      // Verify success rate reflects failure
      assert(result.stderr.includes('Success Rate:'), 'Should show success rate');
    });
  });

  describe('Error Handling', () => {
    test('should handle invalid PR URL', async () => {
      const result = await runScript('invalid-url', 'fake-token');
      
      assert.notStrictEqual(result.exitCode, 0, 'Should exit with error code');
      assert(result.stderr.includes('Invalid PR URL'), 'Should show error message');
    });

    test('should handle missing token', async () => {
      const result = await runScript('https://github.com/test-owner/test-repo/pull/123');
      
      assert.notStrictEqual(result.exitCode, 0, 'Should exit with error code');
      assert(result.stderr.includes('Usage:'), 'Should show usage message');
    });

    test('should handle PR not found', async () => {
      // Don't mock the PR endpoint, so it will return 404
      const result = await runScript('https://github.com/test-owner/test-repo/pull/999', 'fake-token');
      
      assert.notStrictEqual(result.exitCode, 0, 'Should exit with error code');
      assert(result.stderr.includes('Error fetching'), 'Should show fetch error');
    });

    test('should handle no workflow runs', async () => {
      githubMock
        .mockPullRequest('test-owner', 'test-repo', 126)
        .mockWorkflowRuns('test-owner', 'test-repo', 'abc123def456', []); // Empty runs

      const result = await runScript('https://github.com/test-owner/test-repo/pull/126', 'fake-token');
      
      assert.notStrictEqual(result.exitCode, 0, 'Should exit with error code');
      assert(result.stderr.includes('No workflow runs found'), 'Should show no runs message');
    });
  });

  describe('Trace Event Validation', () => {
    test('should generate valid Chrome Tracing format', async () => {
      githubMock.mockSuccessfulPipeline();

      const result = await runScript('https://github.com/test-owner/test-repo/pull/123', 'fake-token');
      const output = JSON.parse(result.stdout);

      // Validate required Chrome Tracing fields
      assert.strictEqual(output.displayTimeUnit, 'ms');
      assert(Array.isArray(output.traceEvents));
      assert(typeof output.otherData === 'object');

      // Validate event structure
      output.traceEvents.forEach(event => {
        assert(typeof event.name === 'string', 'Event should have name');
        assert(typeof event.ph === 'string', 'Event should have phase');
        assert(typeof event.ts === 'number', 'Event should have timestamp');
        assert(typeof event.pid === 'number', 'Event should have process ID');
        assert(typeof event.tid === 'number', 'Event should have thread ID');
        
        if (event.ph === 'X') {
          assert(typeof event.dur === 'number', 'Complete events should have duration');
          assert(event.dur >= 0, 'Duration should be non-negative');
        }
      });

      // Validate timing consistency
      const completeEvents = output.traceEvents.filter(e => e.ph === 'X');
      completeEvents.forEach(event => {
        assert(event.ts >= 0, 'Timestamps should be non-negative');
        assert(event.dur > 0, 'Durations should be positive');
      });
    });

    test('should generate proper thread hierarchy', async () => {
      githubMock.mockSuccessfulPipeline();

      const result = await runScript('https://github.com/test-owner/test-repo/pull/123', 'fake-token');
      const output = JSON.parse(result.stdout);

      // Check for thread metadata
      const threadNameEvents = output.traceEvents.filter(e => e.name === 'thread_name');
      assert(threadNameEvents.length > 0, 'Should have thread name metadata');

      const threadSortEvents = output.traceEvents.filter(e => e.name === 'thread_sort_index');
      assert(threadSortEvents.length > 0, 'Should have thread sort metadata');

      // Verify thread organization
      const workflowThreads = threadNameEvents.filter(e => 
        e.args?.name?.includes('CI Pipeline')
      );
      assert(workflowThreads.length > 0, 'Should have workflow threads');

      const stepThreads = threadNameEvents.filter(e => 
        e.args?.name?.includes('Steps')
      );
      assert(stepThreads.length > 0, 'Should have step threads');
    });
  });

  describe('Metrics Calculation', () => {
    test('should calculate success rates correctly', async () => {
      // Create a mixed success/failure scenario
      const headSha = 'mixed123abc';
      githubMock
        .mockPullRequest('test-owner', 'test-repo', 127, { headSha })
        .mockWorkflowRuns('test-owner', 'test-repo', headSha, [
          githubMock.createMockRun({ id: 1, conclusion: 'success' }),
          githubMock.createMockRun({ id: 2, conclusion: 'failure' })
        ]);

      // Mock jobs for both runs
      githubMock
        .mockJobsForRun('test-owner', 'test-repo', 1, [
          githubMock.createMockJob({ id: 1, conclusion: 'success' })
        ])
        .mockJobsForRun('test-owner', 'test-repo', 2, [
          githubMock.createMockJob({ id: 2, conclusion: 'failure' })
        ]);

      const result = await runScript('https://github.com/test-owner/test-repo/pull/127', 'fake-token');
      
      assert.strictEqual(result.exitCode, 0);
      
      // Check that success rate is calculated (should be 50%)
      assert(result.stderr.includes('Success Rate: 50.0%'), 'Should show 50% workflow success rate');
    });
  });
});

// Helper function to run the main script
async function runScript(prUrl, token = null) {
  return new Promise((resolve) => {
    const args = ['-c', `cd ${process.cwd()} && node main.mjs ${prUrl}${token ? ` ${token}` : ''}`];
    const child = spawn('/bin/sh', args, {
      stdio: 'pipe',
      env: { ...process.env }
    });

    let stdout = '';
    let stderr = '';

    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    child.on('close', (code) => {
      resolve({
        exitCode: code,
        stdout: stdout.trim(),
        stderr: stderr.trim()
      });
    });

    // Set a timeout to prevent hanging tests
    setTimeout(() => {
      child.kill('SIGTERM');
      resolve({
        exitCode: -1,
        stdout: stdout.trim(),
        stderr: stderr.trim() + '\nTest timeout'
      });
    }, 10000); // 10 second timeout
  });
} 
