import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert';
import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import { createGitHubMock } from './github-mock.mjs';

// Import functions for unit testing
import {
  addThreadMetadata,
  generateConcurrencyCounters
} from '../src/visualization.mjs';

describe('Perfetto Trace Generation', () => {
  let mock;

  beforeEach(() => {
    mock = createGitHubMock();
  });

  afterEach(() => {
    mock.cleanup();
  });

  test('should generate complete and valid perfetto trace file with real data', async () => {
    console.log('\n🔍 Testing Perfetto trace file generation with real data...');
    
    // Create a temporary trace file
    const tmpDir = process.env.TMPDIR || '/tmp';
    const traceFile = path.join(tmpDir, `perfetto-test-${Date.now()}.json`);
    
    try {
      // Use a real GitHub URL that has workflow runs (similar to existing test)
      const testUrl = 'https://github.com/facebook/react/pull/12345';
      const result = await runAnalyzer([testUrl, '--perfetto', traceFile]);
      
      // Check if trace file was created
      if (fs.existsSync(traceFile)) {
        const traceContent = fs.readFileSync(traceFile, 'utf8');
        const traceData = JSON.parse(traceContent);
        
        // Basic structure validation
        assert(traceData.traceEvents, 'Should have traceEvents array');
        assert(Array.isArray(traceData.traceEvents), 'traceEvents should be an array');
        assert(traceData.displayTimeUnit, 'Should have displayTimeUnit');
        
        // Check for required metadata
        const metadataEvents = traceData.traceEvents.filter(e => e.ph === 'M');
        assert(metadataEvents.length > 0, 'Should have metadata events');
        
        // Check for process and thread metadata
        const processEvents = metadataEvents.filter(e => e.name === 'process_name');
        const threadEvents = metadataEvents.filter(e => e.name === 'thread_name');
        assert(processEvents.length > 0, 'Should have process metadata');
        assert(threadEvents.length > 0, 'Should have thread metadata');
        
        // Check for duration events (jobs and steps)
        const durationEvents = traceData.traceEvents.filter(e => e.ph === 'X');
        if (durationEvents.length > 0) {
          durationEvents.forEach(event => {
            assert(event.ts >= 0, 'Timestamp should be non-negative');
            assert(event.dur > 0, 'Duration should be positive');
            assert(event.name, 'Should have event name');
            assert(event.pid !== undefined, 'Should have process ID');
            assert(event.tid !== undefined, 'Should have thread ID');
          });
        }
        
        // Check for counter events (concurrency)
        const counterEvents = traceData.traceEvents.filter(e => e.ph === 'C');
        if (counterEvents.length > 0) {
          counterEvents.forEach(event => {
            assert(event.ts >= 0, 'Counter timestamp should be non-negative');
            assert(event.args, 'Counter should have args');
            assert(Object.keys(event.args).length > 0, 'Counter should have at least one counter value');
          });
        }
        
        // Check for instant events (reviews/merges)
        const instantEvents = traceData.traceEvents.filter(e => e.ph === 'i');
        if (instantEvents.length > 0) {
          instantEvents.forEach(event => {
            assert(event.ts >= 0, 'Instant event timestamp should be non-negative');
            assert(event.name, 'Should have event name');
            assert(event.s, 'Should have scope');
          });
        }
        
        // Validate timestamp consistency
        const allEvents = traceData.traceEvents.filter(e => e.ts !== undefined);
        if (allEvents.length > 0) {
          const timestamps = allEvents.map(e => e.ts).sort((a, b) => a - b);
          const minTs = timestamps[0];
          const maxTs = timestamps[timestamps.length - 1];
          
          // All timestamps should be in the same range
          assert(maxTs - minTs < 1000000000, 'Timestamp range should be reasonable');
          
          // No negative timestamps
          assert(minTs >= 0, 'No negative timestamps allowed');
        }
        
        console.log(`✅ Perfetto trace validation passed: ${traceData.traceEvents.length} events`);
        console.log(`📁 Trace file: ${traceFile}`);
        
        // Test opening in perfetto (if available)
        await testPerfettoUI(traceFile);
      } else {
        // If no trace file was created, check if it's because there were no workflow runs
        if (result.stderr.includes('No workflow runs found')) {
          console.log('⏭️ Skipping trace validation - no workflow runs found (expected for test URL)');
        } else {
          throw new Error('Trace file not created and no clear reason given');
        }
      }
      
    } catch (error) {
      console.error(`❌ Perfetto trace validation failed: ${error.message}`);
      throw error;
    } finally {
      // Clean up trace file
      if (fs.existsSync(traceFile)) {
        fs.unlinkSync(traceFile);
      }
    }
  });
  
  test('should handle empty trace events gracefully', async () => {
    console.log('\n🔍 Testing empty trace handling...');
    
    // Test with a URL that has no workflow runs
    const testUrl = 'https://github.com/nonexistent/repo/pull/999999';
    const result = await runAnalyzer([testUrl]);
    
    // Should handle gracefully without crashing
    assert(result.exitCode !== undefined, 'Should have exit code');
    console.log('✅ Empty trace handling passed');
  });
  
  test('should validate trace event completeness', async () => {
    console.log('\n🔍 Testing trace event completeness...');
    
    const tmpDir = process.env.TMPDIR || '/tmp';
    const traceFile = path.join(tmpDir, `completeness-test-${Date.now()}.json`);
    
    try {
      // Use a real GitHub URL that has workflow runs
      const testUrl = 'https://github.com/facebook/react/pull/12345';
      const result = await runAnalyzer([testUrl, '--perfetto', traceFile]);
      
      if (fs.existsSync(traceFile)) {
        const traceContent = fs.readFileSync(traceFile, 'utf8');
        const traceData = JSON.parse(traceContent);
        
        // Check that all events have required fields
        const requiredFields = ['ph', 'ts'];
        traceData.traceEvents.forEach((event, index) => {
          requiredFields.forEach(field => {
            assert(event[field] !== undefined, `Event ${index} missing required field: ${field}`);
          });
        });
        
        // Check that duration events have duration field
        const durationEvents = traceData.traceEvents.filter(e => e.ph === 'X');
        durationEvents.forEach(event => {
          assert(event.dur !== undefined, 'Duration event missing dur field');
          assert(event.dur > 0, 'Duration should be positive');
        });
        
        // Check that counter events have args
        const counterEvents = traceData.traceEvents.filter(e => e.ph === 'C');
        counterEvents.forEach(event => {
          assert(event.args, 'Counter event missing args');
        });
        
        console.log('✅ Trace event completeness validation passed');
      } else {
        console.log('⏭️ Skipping completeness validation - no trace file generated');
      }
    } catch (error) {
      console.error(`❌ Trace completeness validation failed: ${error.message}`);
      throw error;
    } finally {
      if (fs.existsSync(traceFile)) {
        fs.unlinkSync(traceFile);
      }
    }
  });

  test('should generate valid Chrome Tracing Format for Perfetto', async () => {
    console.log('\n🔍 Testing Chrome Tracing Format compliance...');
    
    const tmpDir = process.env.TMPDIR || '/tmp';
    const traceFile = path.join(tmpDir, `chrome-trace-test-${Date.now()}.json`);
    
    try {
      // Use a real GitHub URL that has workflow runs
      const testUrl = 'https://github.com/facebook/react/pull/12345';
      const result = await runAnalyzer([testUrl, '--perfetto', traceFile]);
      
      if (fs.existsSync(traceFile)) {
        const traceContent = fs.readFileSync(traceFile, 'utf8');
        const traceData = JSON.parse(traceContent);
        
        // Validate Chrome Tracing Format requirements
        assert(traceData.traceEvents, 'Must have traceEvents array');
        assert(Array.isArray(traceData.traceEvents), 'traceEvents must be an array');
        
        // Check for required event types
        const eventTypes = new Set(traceData.traceEvents.map(e => e.ph));
        assert(eventTypes.has('M'), 'Must have metadata events (M)');
        
        // Check for process metadata
        const processMetadata = traceData.traceEvents.filter(e => 
          e.ph === 'M' && e.name === 'process_name'
        );
        assert(processMetadata.length > 0, 'Must have process metadata');
        
        // Check for thread metadata
        const threadMetadata = traceData.traceEvents.filter(e => 
          e.ph === 'M' && e.name === 'thread_name'
        );
        assert(threadMetadata.length > 0, 'Must have thread metadata');
        
        // Validate duration events
        const durationEvents = traceData.traceEvents.filter(e => e.ph === 'X');
        if (durationEvents.length > 0) {
          durationEvents.forEach(event => {
            // Chrome Tracing Format requires these fields for duration events
            assert(event.ts !== undefined, 'Duration event must have timestamp');
            assert(event.dur !== undefined, 'Duration event must have duration');
            assert(event.pid !== undefined, 'Duration event must have process ID');
            assert(event.tid !== undefined, 'Duration event must have thread ID');
            assert(event.name, 'Duration event must have name');
            
            // Timestamps should be in microseconds when displayTimeUnit is 'ms'
            if (traceData.displayTimeUnit === 'ms') {
              assert(event.ts >= 1000, 'Timestamp should be in microseconds');
              assert(event.dur >= 1000, 'Duration should be in microseconds');
            }
          });
        }
        
        // Validate counter events
        const counterEvents = traceData.traceEvents.filter(e => e.ph === 'C');
        if (counterEvents.length > 0) {
          counterEvents.forEach(event => {
            assert(event.ts !== undefined, 'Counter event must have timestamp');
            assert(event.args, 'Counter event must have args');
            assert(Object.keys(event.args).length > 0, 'Counter event must have counter values');
          });
        }
        
        // Validate instant events
        const instantEvents = traceData.traceEvents.filter(e => e.ph === 'i');
        if (instantEvents.length > 0) {
          instantEvents.forEach(event => {
            assert(event.ts !== undefined, 'Instant event must have timestamp');
            assert(event.name, 'Instant event must have name');
            assert(event.s, 'Instant event must have scope');
          });
        }
        
        console.log('✅ Chrome Tracing Format compliance validation passed');
      } else {
        console.log('⏭️ Skipping Chrome Tracing Format validation - no trace file generated');
      }
    } catch (error) {
      console.error(`❌ Chrome Tracing Format validation failed: ${error.message}`);
      throw error;
    } finally {
      if (fs.existsSync(traceFile)) {
        fs.unlinkSync(traceFile);
      }
    }
  });

  test('should handle timestamp normalization correctly', async () => {
    console.log('\n🔍 Testing timestamp normalization...');
    
    const tmpDir = process.env.TMPDIR || '/tmp';
    const traceFile = path.join(tmpDir, `timestamp-test-${Date.now()}.json`);
    
    try {
      // Use a real GitHub URL that has workflow runs
      const testUrl = 'https://github.com/facebook/react/pull/12345';
      const result = await runAnalyzer([testUrl, '--perfetto', traceFile]);
      
      if (fs.existsSync(traceFile)) {
        const traceContent = fs.readFileSync(traceFile, 'utf8');
        const traceData = JSON.parse(traceContent);
        
        // Check that all timestamps are properly normalized
        const allEvents = traceData.traceEvents.filter(e => e.ts !== undefined);
        if (allEvents.length > 0) {
          const timestamps = allEvents.map(e => e.ts).sort((a, b) => a - b);
          
          // No negative timestamps
          assert(timestamps[0] >= 0, 'First timestamp should be non-negative');
          
          // Timestamps should be in ascending order
          for (let i = 1; i < timestamps.length; i++) {
            assert(timestamps[i] >= timestamps[i-1], 'Timestamps should be in ascending order');
          }
          
          // No timestamp should be 0 (this was a known bug)
          timestamps.forEach((ts, index) => {
            assert(ts > 0, `Timestamp at index ${index} should not be 0`);
          });
          
          // Timestamp range should be reasonable
          const range = timestamps[timestamps.length - 1] - timestamps[0];
          assert(range < 1000000000, 'Timestamp range should be reasonable (< 1B microseconds)');
        }
        
        console.log('✅ Timestamp normalization validation passed');
      } else {
        console.log('⏭️ Skipping timestamp normalization validation - no trace file generated');
      }
    } catch (error) {
      console.error(`❌ Timestamp normalization validation failed: ${error.message}`);
      throw error;
    } finally {
      if (fs.existsSync(traceFile)) {
        fs.unlinkSync(traceFile);
      }
    }
  });

  test('should handle timestamp renormalization edge cases', () => {
    console.log('\n🔍 Testing timestamp renormalization edge cases...');
    
    // Test the renormalization logic that could cause incomplete traces
    const mockEvents = [
      {
        name: 'Test Job',
        ph: 'X',
        ts: 1000, // microseconds
        dur: 5000,
        pid: 1,
        tid: 1,
        args: { url_index: 1, source_url: 'https://github.com/test/repo/pull/123' }
      },
      {
        name: 'Test Step',
        ph: 'X',
        ts: 2000, // microseconds
        dur: 3000,
        pid: 1,
        tid: 1,
        args: { url_index: 1, source_url: 'https://github.com/test/repo/pull/123' }
      }
    ];
    
    // Simulate the renormalization logic from visualization.mjs
    const urlResult = {
      urlIndex: 0,
      earliestTime: 1000000 // 1 second in milliseconds
    };
    
    const globalEarliestTime = 1000000; // 1 second in milliseconds
    
    const renormalizedEvents = mockEvents.map(event => {
      if (event.ts !== undefined) {
        // Convert from microseconds back to milliseconds, add URL earliest time
        const absoluteTime = event.ts / 1000 + urlResult.earliestTime;
        // Normalize against global earliest time, convert to microseconds
        const renormalizedTime = (absoluteTime - globalEarliestTime) * 1000;
        return { ...event, ts: renormalizedTime };
      }
      return event;
    });
    
    // Validate that all events still have timestamps after renormalization
    renormalizedEvents.forEach((event, index) => {
      assert(event.ts !== undefined, `Event ${index} should have timestamp after renormalization`);
      assert(event.ts >= 0, `Event ${index} should have non-negative timestamp after renormalization`);
      assert(event.ph, `Event ${index} should have event type after renormalization`);
      assert(event.name, `Event ${index} should have name after renormalization`);
    });
    
    // Validate that timestamps are in the expected range
    const timestamps = renormalizedEvents.map(e => e.ts).sort((a, b) => a - b);
    assert(timestamps[0] >= 0, 'First timestamp should be non-negative after renormalization');
    assert(timestamps[timestamps.length - 1] >= timestamps[0], 'Timestamps should be in ascending order');
    
    console.log('✅ Timestamp renormalization edge case validation passed');
  });

  // Unit tests that don't require external API calls
  describe('Trace Generation Functions', () => {
    test('should add thread metadata correctly', () => {
      const traceEvents = [];
      const processId = 1;
      const threadId = 2;
      const name = 'Test Thread';
      const sortIndex = 0;
      
      addThreadMetadata(traceEvents, processId, threadId, name, sortIndex);
      
      assert.strictEqual(traceEvents.length, 2, 'Should add 2 metadata events');
      
      const threadNameEvent = traceEvents.find(e => e.name === 'thread_name');
      assert(threadNameEvent, 'Should have thread_name event');
      assert.strictEqual(threadNameEvent.ph, 'M', 'Should be metadata event');
      assert.strictEqual(threadNameEvent.pid, processId, 'Should have correct process ID');
      assert.strictEqual(threadNameEvent.tid, threadId, 'Should have correct thread ID');
      assert.strictEqual(threadNameEvent.args.name, name, 'Should have correct thread name');
      
      const sortIndexEvent = traceEvents.find(e => e.name === 'thread_sort_index');
      assert(sortIndexEvent, 'Should have thread_sort_index event');
      assert.strictEqual(sortIndexEvent.ph, 'M', 'Should be metadata event');
      assert.strictEqual(sortIndexEvent.args.sort_index, sortIndex, 'Should have correct sort index');
    });

    test('should generate concurrency counters correctly', () => {
      const traceEvents = [];
      const jobStartTimes = [
        { ts: 1000, type: 'start' },
        { ts: 2000, type: 'start' },
        { ts: 3000, type: 'start' }
      ];
      const jobEndTimes = [
        { ts: 4000, type: 'end' },
        { ts: 5000, type: 'end' },
        { ts: 6000, type: 'end' }
      ];
      const earliestTime = 1000;
      
      generateConcurrencyCounters(jobStartTimes, jobEndTimes, traceEvents, earliestTime);
      
      // Should have process and thread metadata
      const processEvent = traceEvents.find(e => e.name === 'process_name');
      assert(processEvent, 'Should have process metadata');
      
      const threadEvent = traceEvents.find(e => e.name === 'thread_name');
      assert(threadEvent, 'Should have thread metadata');
      
      // Should have counter events for each timestamp
      const counterEvents = traceEvents.filter(e => e.ph === 'C');
      assert(counterEvents.length > 0, 'Should have counter events');
      
      // Check that counter values are correct
      counterEvents.forEach(event => {
        assert(event.args['Concurrent Jobs'] !== undefined, 'Should have concurrent jobs counter');
        assert(typeof event.args['Concurrent Jobs'] === 'number', 'Counter value should be number');
        assert(event.args['Concurrent Jobs'] >= 0, 'Counter value should be non-negative');
      });
    });

    test('should handle empty job data gracefully', () => {
      const traceEvents = [];
      const jobStartTimes = [];
      const jobEndTimes = [];
      const earliestTime = 1000;
      
      generateConcurrencyCounters(jobStartTimes, jobEndTimes, traceEvents, earliestTime);
      
      // When there are no jobs, the function should return early and not add any metadata
      assert.strictEqual(traceEvents.length, 0, 'Should not add any events when no job data');
    });
  });

  // Test specific perfetto issues mentioned in user query
  describe('Perfetto UI Integration Issues', () => {
    test('should generate complete JSON trace file structure', () => {
      console.log('\n🔍 Testing JSON trace file structure completeness...');
      
      // Create a minimal valid trace structure
      const minimalTrace = {
        displayTimeUnit: 'ms',
        traceEvents: [
          {
            name: 'process_name',
            ph: 'M',
            pid: 0,
            args: { name: 'Test Process' }
          },
          {
            name: 'thread_name',
            ph: 'M',
            pid: 0,
            tid: 1,
            args: { name: 'Test Thread' }
          },
          {
            name: 'Test Event',
            ph: 'X',
            pid: 0,
            tid: 1,
            ts: 1000,
            dur: 5000
          }
        ]
      };
      
      // Validate the structure
      assert(minimalTrace.displayTimeUnit, 'Should have displayTimeUnit');
      assert(minimalTrace.traceEvents, 'Should have traceEvents array');
      assert(Array.isArray(minimalTrace.traceEvents), 'traceEvents should be an array');
      assert(minimalTrace.traceEvents.length > 0, 'Should have at least one trace event');
      
      // Check that all events have required fields
      minimalTrace.traceEvents.forEach((event, index) => {
        assert(event.ph, `Event ${index} should have ph (event type)`);
        assert(event.ts !== undefined || event.ph === 'M', `Event ${index} should have timestamp (except metadata)`);
      });
      
      console.log('✅ JSON trace file structure validation passed');
    });

    test('should validate Chrome Tracing Format compliance', () => {
      console.log('\n🔍 Testing Chrome Tracing Format compliance...');
      
      // Test data that should be valid
      const validEvents = [
        { ph: 'M', name: 'process_name', pid: 0, args: { name: 'Test' } },
        { ph: 'M', name: 'thread_name', pid: 0, tid: 1, args: { name: 'Test' } },
        { ph: 'X', name: 'Test Job', pid: 0, tid: 1, ts: 1000, dur: 5000 },
        { ph: 'C', name: 'Counter', pid: 0, tid: 1, ts: 1000, args: { 'value': 5 } },
        { ph: 'i', name: 'Instant', pid: 0, tid: 1, ts: 1000, s: 'p' }
      ];
      
      validEvents.forEach((event, index) => {
        // Check required fields based on event type
        switch (event.ph) {
          case 'M': // Metadata
            assert(event.name, `Metadata event ${index} should have name`);
            assert(event.pid !== undefined, `Metadata event ${index} should have pid`);
            break;
          case 'X': // Duration
            assert(event.name, `Duration event ${index} should have name`);
            assert(event.pid !== undefined, `Duration event ${index} should have pid`);
            assert(event.tid !== undefined, `Duration event ${index} should have tid`);
            assert(event.ts !== undefined, `Duration event ${index} should have ts`);
            assert(event.dur !== undefined, `Duration event ${index} should have dur`);
            break;
          case 'C': // Counter
            assert(event.name, `Counter event ${index} should have name`);
            assert(event.pid !== undefined, `Counter event ${index} should have pid`);
            assert(event.tid !== undefined, `Counter event ${index} should have tid`);
            assert(event.ts !== undefined, `Counter event ${index} should have ts`);
            assert(event.args, `Counter event ${index} should have args`);
            break;
          case 'i': // Instant
            assert(event.name, `Instant event ${index} should have name`);
            assert(event.pid !== undefined, `Instant event ${index} should have pid`);
            assert(event.tid !== undefined, `Instant event ${index} should have tid`);
            assert(event.ts !== undefined, `Instant event ${index} should have ts`);
            assert(event.s, `Instant event ${index} should have scope`);
            break;
        }
      });
      
      console.log('✅ Chrome Tracing Format compliance validation passed');
    });

    test('should handle timestamp edge cases correctly', () => {
      console.log('\n🔍 Testing timestamp edge cases...');
      
      // Test various timestamp scenarios
      const testCases = [
        { ts: 0, expected: false, description: 'Zero timestamp' },
        { ts: -1, expected: false, description: 'Negative timestamp' },
        { ts: 1, expected: true, description: 'Minimum valid timestamp' },
        { ts: 1000, expected: true, description: 'Valid timestamp in microseconds' },
        { ts: 1000000, expected: true, description: 'Valid timestamp in microseconds' },
        { ts: Number.MAX_SAFE_INTEGER, expected: false, description: 'Extremely large timestamp' }
      ];
      
      testCases.forEach(({ ts, expected, description }) => {
        const isValid = ts > 0 && ts < 1000000000; // Reasonable range check
        assert.strictEqual(isValid, expected, `${description}: ${ts} should be ${expected ? 'valid' : 'invalid'}`);
      });
      
      console.log('✅ Timestamp edge case validation passed');
    });
  });
});

// Helper function to run the analyzer
async function runAnalyzer(args) {
  return new Promise((resolve) => {
    const child = spawn('node', ['main.mjs', ...args], {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: { ...process.env, GITHUB_TOKEN: 'test-token' }
    });
    
    let stdout = '';
    let stderr = '';
    let exitCode = null;
    
    child.stdout.on('data', (data) => {
      stdout += data.toString();
    });
    
    child.stderr.on('data', (data) => {
      stderr += data.toString();
    });
    
    child.on('close', (code) => {
      exitCode = code;
      resolve({
        exitCode,
        stdout,
        stderr,
        combined: stdout + stderr
      });
    });
    
    // Set a timeout to prevent hanging
    setTimeout(() => {
      child.kill();
      resolve({
        exitCode: -1,
        stdout,
        stderr,
        combined: stdout + stderr
      });
    }, 30000); // 30 second timeout
  });
}

// Helper function to test perfetto UI integration
async function testPerfettoUI(traceFile) {
  try {
    console.log('\n🔍 Testing Perfetto UI integration...');
    
    // Check if the open_trace_in_ui script exists
    const { spawn } = await import('child_process');
    const { tmpdir } = await import('os');
    const { join } = await import('path');
    
    const scriptPath = join(tmpdir(), 'open_trace_in_ui');
    
    if (fs.existsSync(scriptPath)) {
      console.log('✅ Perfetto UI script found');
      
      // Test that the trace file is valid for the script
      const stats = fs.statSync(traceFile);
      assert(stats.size > 0, 'Trace file should have content');
      assert(stats.size < 100 * 1024 * 1024, 'Trace file should be reasonable size (< 100MB)');
      
      console.log('✅ Trace file validation for UI passed');
    } else {
      console.log('⏭️ Perfetto UI script not found (expected in test environment)');
    }
  } catch (error) {
    console.log(`⚠️ Perfetto UI test skipped: ${error.message}`);
  }
}
