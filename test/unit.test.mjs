#!/usr/bin/env node

/**
 * Comprehensive Unit Tests
 * 
 * Tests all individual methods with concise, human-readable tests that:
 * - De-risk critical functionality
 * - Enable confidence iteration
 * - Focus on "less is more" - simple, clear test cases
 * - Cover edge cases and error conditions
 */

import { test, describe } from 'node:test';
import assert from 'node:assert';
import fs from 'fs';
import path from 'path';
import os from 'os';

// Import all functions for testing
import { 
  parseGitHubUrl, 
  getJobGroup, 
  calculateCombinedMetrics, 
  calculateCombinedSuccessRate, 
  calculateCombinedJobSuccessRate,
  findBottleneckJobs,
  humanizeTime,
  initializeMetrics,
  findEarliestTimestamp,
  calculateMaxConcurrency,
  calculateFinalMetrics,
  analyzeSlowJobs,
  analyzeSlowSteps,
  findOverlappingJobs,
  categorizeStep,
  getStepIcon,
  makeClickableLink,
  grayText,
  greenText,
  redText,
  yellowText,
  blueText,
  ProgressBar
} from '../main.mjs';

// Mock the main module to test internal functions
import { createRequire } from 'module';
const require = createRequire(import.meta.url);

// We'll need to test internal functions by importing them differently
// For now, let's create focused tests for the most critical functions

describe('Core Data Processing Functions', () => {
  
  describe('Metrics Initialization', () => {
    test('should initialize empty metrics object', () => {
      const metrics = initializeMetrics();
      
      assert.strictEqual(metrics.totalRuns, 0);
      assert.strictEqual(metrics.jobDurations.length, 0);
      assert.strictEqual(metrics.longestJob.duration, 0);
      assert.strictEqual(metrics.shortestJob.duration, Infinity);
      assert(metrics.runnerTypes instanceof Set);
      assert.strictEqual(metrics.jobTimeline.length, 0);
    });
  });

  describe('Timeline Processing', () => {
    test('should find earliest timestamp from runs', () => {
      const mockRuns = [
        { created_at: '2024-01-01T10:00:00Z' },
        { created_at: '2024-01-01T09:00:00Z' },
        { created_at: '2024-01-01T11:00:00Z' }
      ];
      
      const earliest = findEarliestTimestamp(mockRuns);
      assert.strictEqual(earliest, new Date('2024-01-01T09:00:00Z').getTime());
    });

    test('should handle empty runs array', () => {
      const mockRuns = [];
      const earliest = findEarliestTimestamp(mockRuns);
      assert.strictEqual(earliest, Infinity);
    });
  });

  describe('Concurrency Calculation', () => {
    test('should calculate max concurrency correctly', () => {
      const jobStartTimes = [
        { ts: 1000, type: 'start' },
        { ts: 2000, type: 'start' },
        { ts: 3000, type: 'start' }
      ];
      const jobEndTimes = [
        { ts: 4000, type: 'end' },
        { ts: 3500, type: 'end' },
        { ts: 5000, type: 'end' }
      ];
      
      const maxConcurrency = calculateMaxConcurrency(jobStartTimes, jobEndTimes);
      assert.strictEqual(maxConcurrency, 3);
    });

    test('should handle overlapping jobs', () => {
      const jobStartTimes = [
        { ts: 1000, type: 'start' },
        { ts: 1500, type: 'start' },
        { ts: 2000, type: 'start' }
      ];
      const jobEndTimes = [
        { ts: 3000, type: 'end' },
        { ts: 2500, type: 'end' },
        { ts: 3500, type: 'end' }
      ];
      
      const maxConcurrency = calculateMaxConcurrency(jobStartTimes, jobEndTimes);
      assert.strictEqual(maxConcurrency, 3);
    });
  });
});

describe('GitHub API Integration Functions', () => {
  
  describe('URL Parsing Edge Cases', () => {
    test('should handle GitHub enterprise URLs', () => {
      const enterpriseUrl = 'https://github.company.com/owner/repo/pull/123';
      
      // The current implementation accepts enterprise URLs with correct path structure
      const result = parseGitHubUrl(enterpriseUrl);
      assert.strictEqual(result.owner, 'owner');
      assert.strictEqual(result.repo, 'repo');
      assert.strictEqual(result.type, 'pr');
      assert.strictEqual(result.identifier, '123');
    });

    test('should handle malformed URLs', () => {
      const malformedUrls = [
        'not-a-url',
        'https://github.com/',
        'https://github.com/owner',
        'https://github.com/owner/repo',
        'https://github.com/owner/repo/pull',
        'https://github.com/owner/repo/pull/',
        'https://github.com/owner/repo/commit',
        'https://github.com/owner/repo/commit/'
      ];
      
      malformedUrls.forEach(url => {
        assert.throws(() => {
          parseGitHubUrl(url);
        }, /Invalid URL|Invalid GitHub URL/);
      });
    });

    test('should handle URLs with extra path segments', () => {
      const extraPathUrl = 'https://github.com/owner/repo/pull/123/files';
      
      assert.throws(() => {
        parseGitHubUrl(extraPathUrl);
      }, /Invalid GitHub URL/);
    });
  });


});

describe('Visualization & Output Functions', () => {
  
  describe('Step Categorization', () => {
    test('should categorize steps correctly', () => {
      assert.strictEqual(categorizeStep('Checkout code'), 'step_checkout');
      assert.strictEqual(categorizeStep('Setup Node.js'), 'step_setup');
      assert.strictEqual(categorizeStep('Build project'), 'step_build');
      assert.strictEqual(categorizeStep('Run tests'), 'step_test');
      assert.strictEqual(categorizeStep('Lint code'), 'step_lint');
      assert.strictEqual(categorizeStep('Deploy to production'), 'step_deploy');
      assert.strictEqual(categorizeStep('Upload artifacts'), 'step_artifact');
      assert.strictEqual(categorizeStep('Security scan'), 'step_security');
      assert.strictEqual(categorizeStep('Send notification'), 'step_notify');
      assert.strictEqual(categorizeStep('Custom step'), 'step_other');
    });
  });

  describe('Step Icon Selection', () => {
    test('should select appropriate icons', () => {
      // Test conclusion overrides
      assert.strictEqual(getStepIcon('Any step', 'failure'), '❌');
      assert.strictEqual(getStepIcon('Any step', 'cancelled'), '🚫');
      assert.strictEqual(getStepIcon('Any step', 'skipped'), '⏭️');
      
      // Test category-based icons
      assert.strictEqual(getStepIcon('Checkout code', 'success'), '📥');
      assert.strictEqual(getStepIcon('Setup Node.js', 'success'), '⚙️');
      assert.strictEqual(getStepIcon('Build project', 'success'), '🔨');
      assert.strictEqual(getStepIcon('Run tests', 'success'), '🧪');
      assert.strictEqual(getStepIcon('Lint code', 'success'), '🔍');
      assert.strictEqual(getStepIcon('Deploy to production', 'success'), '🚀');
      assert.strictEqual(getStepIcon('Upload artifacts', 'success'), '📤');
      assert.strictEqual(getStepIcon('Security scan', 'success'), '🔒');
      assert.strictEqual(getStepIcon('Send notification', 'success'), '📢');
      assert.strictEqual(getStepIcon('Custom step', 'success'), '▶️');
    });
  });

  describe('Clickable Link Generation', () => {
    test('should generate ANSI clickable links', () => {
      const link = makeClickableLink('https://github.com/test', 'Test Link');
      assert(link.includes('\u001b]8;;https://github.com/test\u0007Test Link\u001b]8;;\u0007'));
    });

    test('should use URL as text when no text provided', () => {
      const link = makeClickableLink('https://github.com/test');
      assert(link.includes('https://github.com/test'));
    });
  });

  describe('Color Text Functions', () => {
    test('should apply ANSI color codes', () => {
      assert(grayText('test').includes('\u001b[90m'));
      assert(greenText('test').includes('\u001b[32m'));
      assert(redText('test').includes('\u001b[31m'));
      assert(yellowText('test').includes('\u001b[33m'));
      assert(blueText('test').includes('\u001b[34m'));
      
      // All should end with reset
      assert(grayText('test').endsWith('\u001b[0m'));
      assert(greenText('test').endsWith('\u001b[0m'));
      assert(redText('test').endsWith('\u001b[0m'));
      assert(yellowText('test').endsWith('\u001b[0m'));
      assert(blueText('test').endsWith('\u001b[0m'));
    });
  });
});

describe('Job Analysis Functions', () => {
  
  describe('Slow Job Analysis', () => {
    test('should identify slowest jobs', () => {
      const mockMetrics = {
        jobDurations: [1000, 5000, 2000, 8000, 3000],
        jobNames: ['Job1', 'Job2', 'Job3', 'Job4', 'Job5'],
        jobUrls: ['url1', 'url2', 'url3', 'url4', 'url5']
      };
      
      const slowJobs = analyzeSlowJobs(mockMetrics, 3);
      
      assert.strictEqual(slowJobs.length, 3);
      assert.strictEqual(slowJobs[0].name, 'Job4');
      assert.strictEqual(slowJobs[0].duration, 8000);
      assert.strictEqual(slowJobs[1].name, 'Job2');
      assert.strictEqual(slowJobs[1].duration, 5000);
      assert.strictEqual(slowJobs[2].name, 'Job5');
      assert.strictEqual(slowJobs[2].duration, 3000);
    });

    test('should handle empty job data', () => {
      const mockMetrics = {
        jobDurations: [],
        jobNames: [],
        jobUrls: []
      };
      
      const slowJobs = analyzeSlowJobs(mockMetrics);
      assert.strictEqual(slowJobs.length, 0);
    });
  });

  describe('Slow Step Analysis', () => {
    test('should identify slowest steps', () => {
      const mockMetrics = {
        stepDurations: [
          { name: 'Step1', duration: 1000 },
          { name: 'Step2', duration: 5000 },
          { name: 'Step3', duration: 2000 },
          { name: 'Step4', duration: 8000 }
        ]
      };

      const slowSteps = analyzeSlowSteps(mockMetrics, 2);

      assert.strictEqual(slowSteps.length, 2);
      assert.strictEqual(slowSteps[0].name, 'Step4');
      assert.strictEqual(slowSteps[0].duration, 8000);
      assert.strictEqual(slowSteps[1].name, 'Step2');
      assert.strictEqual(slowSteps[1].duration, 5000);
    });

    test('should handle empty input', () => {
      const mockMetrics = { stepDurations: [] };
      const slowSteps = analyzeSlowSteps(mockMetrics);
      assert.strictEqual(slowSteps.length, 0);
    });
  });

  describe('Job Overlap Detection', () => {
    test('should find overlapping jobs', () => {
      const jobs = [
        { name: 'Job1', startTime: 1000, endTime: 3000 },
        { name: 'Job2', startTime: 2000, endTime: 4000 }, // Overlaps with Job1
        { name: 'Job3', startTime: 5000, endTime: 7000 }  // No overlap
      ];
      
      const overlaps = findOverlappingJobs(jobs);
      
      assert.strictEqual(overlaps.length, 1);
      assert.strictEqual(overlaps[0][0].name, 'Job1');
      assert.strictEqual(overlaps[0][1].name, 'Job2');
    });

    test('should handle non-overlapping jobs', () => {
      const jobs = [
        { name: 'Job1', startTime: 1000, endTime: 2000 },
        { name: 'Job2', startTime: 3000, endTime: 4000 },
        { name: 'Job3', startTime: 5000, endTime: 6000 }
      ];
      
      const overlaps = findOverlappingJobs(jobs);
      assert.strictEqual(overlaps.length, 0);
    });
  });
});

describe('Progress Bar Class', () => {
  test('should track progress correctly', () => {
    const progressBar = new ProgressBar(2, 10);
    
    assert.strictEqual(progressBar.totalUrls, 2);
    assert.strictEqual(progressBar.totalRuns, 10);
    assert.strictEqual(progressBar.currentUrl, 0);
    assert.strictEqual(progressBar.isProcessing, false);
    
    progressBar.startUrl(0, 'url1');
    assert.strictEqual(progressBar.currentUrl, 1);
    assert.strictEqual(progressBar.isProcessing, true);
    
    progressBar.setUrlRuns(5);
    assert.strictEqual(progressBar.currentUrlRuns, 5);
    
    progressBar.processRun();
    assert.strictEqual(progressBar.currentRun, 1);
    
    progressBar.finish();
    assert.strictEqual(progressBar.isProcessing, false);
  });
});



  describe('Final Metrics Calculation', () => {
    test('should calculate final metrics correctly', () => {
      const mockMetrics = {
        totalRuns: 10,
        successfulRuns: 8,
        failedRuns: 2,
        totalJobs: 20,
        failedJobs: 3,
        totalSteps: 100,
        failedSteps: 5,
        jobDurations: [1000, 2000, 3000, 4000, 5000],
        stepDurations: [
          { duration: 100 },
          { duration: 200 },
          { duration: 300 }
        ]
      };
      
      const finalMetrics = calculateFinalMetrics(mockMetrics, 10, [], []);
      
      assert.strictEqual(finalMetrics.avgJobDuration, 3000); // (1000+2000+3000+4000+5000)/5
      assert.strictEqual(finalMetrics.avgStepDuration, 200); // (100+200+300)/3
      assert.strictEqual(finalMetrics.successRate, '80.0'); // 8/10 * 100
      assert.strictEqual(finalMetrics.jobSuccessRate, '85.0'); // (20-3)/20 * 100
    });

    test('should handle zero values gracefully', () => {
      const mockMetrics = {
        totalRuns: 0,
        successfulRuns: 0,
        failedRuns: 0,
        totalJobs: 0,
        failedJobs: 0,
        totalSteps: 0,
        failedSteps: 0,
        jobDurations: [],
        stepDurations: []
      };
      
      const finalMetrics = calculateFinalMetrics(mockMetrics, 0, [], []);
      
      assert.strictEqual(finalMetrics.avgJobDuration, 0);
      assert.strictEqual(finalMetrics.avgStepDuration, 0);
      assert.strictEqual(finalMetrics.successRate, 0);
      assert.strictEqual(finalMetrics.jobSuccessRate, 0);
    });
  });
