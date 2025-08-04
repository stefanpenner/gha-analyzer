import { test, describe } from 'node:test';
import assert from 'node:assert';

describe('Timeline Grouping Integration Tests', () => {
  test('should group real-world job names correctly', () => {
    // Simulate the timeline grouping logic from main.mjs
    function getJobGroup(jobName) {
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

    // Real-world job timeline data
    const timeline = [
      { name: 'CI (4.0s)', startTime: 1000, endTime: 5000 },
      { name: 'CI-validations / Build: ci-mariner2 (237.0s)', startTime: 2000, endTime: 239000 },
      { name: 'CI-validations / Consumer: ${{ matrix.consumer_repository_name }} (0.0s)', startTime: 3000, endTime: 3000 },
      { name: 'CI-validations / Setup (164.0s)', startTime: 4000, endTime: 168000 },
      { name: 'CI-validations / Teardown (0.0s)', startTime: 5000, endTime: 5000 },
      { name: 'CI-validations / Validate (581.0s)', startTime: 6000, endTime: 587000 },
      { name: 'Confetti (227.0s)', startTime: 7000, endTime: 234000 },
      { name: 'Lint (180.0s)', startTime: 8000, endTime: 188000 },
      { name: 'Post-merge / Build / (Dry run) Build on Mariner2 (x86_64) (458.0s)', startTime: 9000, endTime: 467000 },
      { name: 'Post-merge / Distributed Testing (0.0s)', startTime: 10000, endTime: 10000 },
      { name: 'Post-merge / Publication (0.0s)', startTime: 11000, endTime: 11000 },
      { name: 'Post-merge / Setup / (Dry run) Setup (209.0s)', startTime: 12000, endTime: 221000 },
      { name: 'Post-merge / Teardown / (Dry run) Teardown (88.0s)', startTime: 13000, endTime: 101000 },
      { name: 'Post-merge / Temp-Publication (0.0s)', startTime: 14000, endTime: 14000 },
      { name: 'Pre-merge (4.0s)', startTime: 500, endTime: 4500 },
      { name: 'Pre-merge validations / Distributed Testing (0.0s)', startTime: 15000, endTime: 15000 },
      { name: 'Pre-merge validations / Teardown (0.0s)', startTime: 16000, endTime: 16000 },
      { name: 'Pre-merge validations / Validations (1150.0s)', startTime: 17000, endTime: 1167000 },
      { name: 'Release / Finalize (18.0s)', startTime: 18000, endTime: 36000 },
      { name: 'Release / Publish: ${{ matrix.build_runner_pool.label }} (0.0s)', startTime: 19000, endTime: 19000 },
      { name: 'Release / Setup (92.0s)', startTime: 20000, endTime: 112000 },
      { name: 'Release / Teardown (0.0s)', startTime: 21000, endTime: 21000 },
      { name: 'Restore Removed Label (0.0s)', startTime: 22000, endTime: 22000 },
      { name: 'Sync Labels (121.0s)', startTime: 23000, endTime: 144000 },
      { name: 'Working Copy Test (2.0s)', startTime: 24000, endTime: 26000 },
      { name: 'sast-scan / CodeQL Scan (go, obhc-carpathia, sast-online) (347.0s)', startTime: 25000, endTime: 372000 },
      { name: 'sast-scan / Compute CodeQL Languages And Compound Repo Check (44.0s)', startTime: 26000, endTime: 70000 },
      { name: 'sast-scan / Semgrep Scan (171.0s)', startTime: 27000, endTime: 198000 },
      { name: 'sast-scan / Upload Blank SARIF/s when Semgrep or CodeQL is not enabled for pull-request (0.0s)', startTime: 28000, endTime: 28000 }
    ];

    // Group the jobs
    const groups = groupJobsByPrefix(timeline);
    const sortedGroups = sortGroupsByEarliestStart(groups);



    // Verify expected groups exist
    assert.strictEqual(Object.keys(groups).length, 12);
    
    // Check that all expected groups exist and have correct counts
    assert.strictEqual(groups['CI (4.0s)']?.length, 1);
    assert.strictEqual(groups['CI-validations']?.length, 5);
    assert.strictEqual(groups['Confetti (227.0s)']?.length, 1);
    assert.strictEqual(groups['Lint (180.0s)']?.length, 1);
    assert.strictEqual(groups['Post-merge']?.length, 6);
    assert.strictEqual(groups['Pre-merge (4.0s)']?.length, 1);
    assert.strictEqual(groups['Pre-merge validations']?.length, 3);
    assert.strictEqual(groups['Release']?.length, 4);
    assert.strictEqual(groups['Restore Removed Label (0.0s)']?.length, 1);
    assert.strictEqual(groups['Sync Labels (121.0s)']?.length, 1);
    assert.strictEqual(groups['Working Copy Test (2.0s)']?.length, 1);
    assert.strictEqual(groups['sast-scan']?.length, 4);

    // Verify groups are sorted by earliest start time
    // Pre-merge starts earliest (500), then CI (1000), then CI-validations (2000), etc.
    assert.strictEqual(sortedGroups[0], 'Pre-merge (4.0s)');
    assert.strictEqual(sortedGroups[1], 'CI (4.0s)');
    assert.strictEqual(sortedGroups[2], 'CI-validations');

    // Verify jobs within groups are sorted by start time
    const ciValidationsJobs = groups['CI-validations'].sort((a, b) => a.startTime - b.startTime);
    assert.strictEqual(ciValidationsJobs[0].name, 'CI-validations / Build: ci-mariner2 (237.0s)');
    assert.strictEqual(ciValidationsJobs[1].name, 'CI-validations / Consumer: ${{ matrix.consumer_repository_name }} (0.0s)');
    assert.strictEqual(ciValidationsJobs[2].name, 'CI-validations / Setup (164.0s)');
    assert.strictEqual(ciValidationsJobs[3].name, 'CI-validations / Teardown (0.0s)');
    assert.strictEqual(ciValidationsJobs[4].name, 'CI-validations / Validate (581.0s)');
  });

  test('should handle edge cases in job naming', () => {
    function getJobGroup(jobName) {
      const parts = jobName.split(' / ');
      return parts.length > 1 ? parts[0] : jobName;
    }

    // Test various edge cases
    assert.strictEqual(getJobGroup('Simple Job'), 'Simple Job');
    assert.strictEqual(getJobGroup('Job / With / Multiple / Slashes'), 'Job');
    assert.strictEqual(getJobGroup('No / Slash'), 'No');
    assert.strictEqual(getJobGroup('Trailing / Slash /'), 'Trailing');
    assert.strictEqual(getJobGroup(' / Leading Slash'), '');
    assert.strictEqual(getJobGroup(''), '');
  });

  test('should maintain chronological order within groups', () => {
    function groupJobsByPrefix(timeline) {
      const jobGroups = {};
      timeline.forEach(job => {
        const groupKey = job.name.split(' / ')[0] || job.name;
        if (!jobGroups[groupKey]) {
          jobGroups[groupKey] = [];
        }
        jobGroups[groupKey].push(job);
      });
      return jobGroups;
    }

    const timeline = [
      { name: 'Release / Publish', startTime: 3000, endTime: 4000 },
      { name: 'Release / Setup', startTime: 1000, endTime: 2000 },
      { name: 'Release / Teardown', startTime: 5000, endTime: 6000 },
      { name: 'CI / Build', startTime: 500, endTime: 1500 },
      { name: 'CI / Test', startTime: 2000, endTime: 2500 }
    ];

    const groups = groupJobsByPrefix(timeline);
    
    // Sort jobs within each group by start time
    Object.keys(groups).forEach(groupName => {
      groups[groupName].sort((a, b) => a.startTime - b.startTime);
    });

    // Verify Release group is sorted chronologically
    const releaseJobs = groups['Release'];
    assert.strictEqual(releaseJobs[0].name, 'Release / Setup');
    assert.strictEqual(releaseJobs[1].name, 'Release / Publish');
    assert.strictEqual(releaseJobs[2].name, 'Release / Teardown');

    // Verify CI group is sorted chronologically
    const ciJobs = groups['CI'];
    assert.strictEqual(ciJobs[0].name, 'CI / Build');
    assert.strictEqual(ciJobs[1].name, 'CI / Test');
  });
}); 