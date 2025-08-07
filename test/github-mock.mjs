import nock from 'nock';

// Mock GitHub API responses
export class GitHubMock {
  constructor() {
    this.baseUrl = 'https://api.github.com';
    this.scope = nock(this.baseUrl);
  }

  mockPullRequest(owner, repo, prNumber, options = {}) {
    const defaultPR = {
      number: prNumber,
      head: {
        ref: options.branchName || 'feature-branch',
        sha: options.headSha || 'abc123def456'
      },
      base: {
        ref: 'main',
        sha: 'def456abc123'
      }
    };

    this.scope
      .get(`/repos/${owner}/${repo}/pulls/${prNumber}`)
      .reply(200, { ...defaultPR, ...options.prData });

    return this;
  }

  mockWorkflowRuns(owner, repo, headSha, runs = []) {
    const defaultRuns = runs.length > 0 ? runs : [this.createMockRun()];
    
    this.scope
      .get(`/repos/${owner}/${repo}/actions/runs`)
      .query({ head_sha: headSha, per_page: 100 })
      .reply(200, { workflow_runs: defaultRuns });

    return this;
  }

  mockJobsForRun(owner, repo, runId, jobs = []) {
    const defaultJobs = jobs.length > 0 ? jobs : [this.createMockJob()];
    
    this.scope
      .get(`/repos/${owner}/${repo}/actions/runs/${runId}/jobs`)
      .query({ per_page: 100 })
      .reply(200, { jobs: defaultJobs });

    return this;
  }

  mockReviews(owner, repo, prNumber, reviews = []) {
    const defaultReviews = reviews.length > 0 ? reviews : [
      { id: 1, state: 'APPROVED', submitted_at: new Date().toISOString(), user: { login: 'reviewer' } }
    ];

    this.scope
      .get(`/repos/${owner}/${repo}/pulls/${prNumber}/reviews`)
      .query({ per_page: 100 })
      .reply(200, defaultReviews);

    return this;
  }

  createMockRun(options = {}) {
    const baseTime = new Date('2024-01-01T10:00:00Z');
    return {
      id: options.id || 12345,
      name: options.name || 'CI',
      status: options.status || 'completed',
      conclusion: options.conclusion || 'success',
      created_at: options.created_at || baseTime.toISOString(),
      updated_at: options.updated_at || new Date(baseTime.getTime() + 300000).toISOString(), // +5 minutes
      repository: {
        owner: { login: 'test-owner' },
        name: 'test-repo'
      },
      ...options
    };
  }

  createMockJob(options = {}) {
    const baseTime = new Date('2024-01-01T10:01:00Z');
    const endTime = new Date(baseTime.getTime() + (options.durationMs || 120000)); // +2 minutes default
    
    return {
      id: options.id || 67890,
      name: options.name || 'test-job',
      status: options.status || 'completed',
      conclusion: options.conclusion || 'success',
      started_at: options.started_at || baseTime.toISOString(),
      completed_at: options.completed_at || endTime.toISOString(),
      runner_name: options.runner_name || 'GitHub Actions 2',
      html_url: `https://github.com/test-owner/test-repo/actions/runs/12345/job/${options.id || 67890}`,
      steps: options.steps || [
        this.createMockStep({ name: 'Checkout code', durationMs: 5000 }),
        this.createMockStep({ name: 'Setup Node.js', durationMs: 15000 }),
        this.createMockStep({ name: 'Run tests', durationMs: 90000 }),
        this.createMockStep({ name: 'Upload artifacts', durationMs: 10000 })
      ],
      ...options
    };
  }

  createMockStep(options = {}) {
    const baseTime = new Date('2024-01-01T10:01:30Z');
    const endTime = new Date(baseTime.getTime() + (options.durationMs || 30000)); // +30s default
    
    return {
      name: options.name || 'Test Step',
      status: options.status || 'completed',
      conclusion: options.conclusion || 'success',
      number: options.number || 1,
      started_at: options.started_at || baseTime.toISOString(),
      completed_at: options.completed_at || endTime.toISOString(),
      ...options
    };
  }

  // Scenario builders for common test cases
  mockSuccessfulPipeline(owner = 'test-owner', repo = 'test-repo', prNumber = 123) {
    const headSha = 'abc123def456';
    
    this.mockPullRequest(owner, repo, prNumber, { headSha });
    
    const run = this.createMockRun({
      id: 12345,
      name: 'CI Pipeline',
      status: 'completed',
      conclusion: 'success'
    });
    
    this.mockWorkflowRuns(owner, repo, headSha, [run]);
    
    const jobs = [
      this.createMockJob({
        id: 1,
        name: 'lint',
        durationMs: 30000,
        steps: [
          this.createMockStep({ name: 'Checkout code', durationMs: 5000 }),
          this.createMockStep({ name: 'Setup Node.js', durationMs: 10000 }),
          this.createMockStep({ name: 'Run linting', durationMs: 15000 })
        ]
      }),
      this.createMockJob({
        id: 2,
        name: 'test',
        durationMs: 120000,
        steps: [
          this.createMockStep({ name: 'Checkout code', durationMs: 5000 }),
          this.createMockStep({ name: 'Setup Node.js', durationMs: 10000 }),
          this.createMockStep({ name: 'Run tests', durationMs: 100000 }),
          this.createMockStep({ name: 'Upload coverage', durationMs: 5000 })
        ]
      })
    ];
    
    this.mockJobsForRun(owner, repo, run.id, jobs);
    return this;
  }

  mockFailedPipeline(owner = 'test-owner', repo = 'test-repo', prNumber = 124) {
    const headSha = 'def456abc789';
    
    this.mockPullRequest(owner, repo, prNumber, { headSha });
    
    const run = this.createMockRun({
      id: 12346,
      name: 'CI Pipeline',
      status: 'completed',
      conclusion: 'failure'
    });
    
    this.mockWorkflowRuns(owner, repo, headSha, [run]);
    
    const jobs = [
      this.createMockJob({
        id: 3,
        name: 'test',
        status: 'completed',
        conclusion: 'failure',
        durationMs: 60000,
        steps: [
          this.createMockStep({ name: 'Checkout code', durationMs: 5000 }),
          this.createMockStep({ name: 'Setup Node.js', durationMs: 10000 }),
          this.createMockStep({ 
            name: 'Run tests', 
            durationMs: 45000,
            conclusion: 'failure'
          })
        ]
      })
    ];
    
    this.mockJobsForRun(owner, repo, run.id, jobs);
    return this;
  }

  mockMultipleRuns(owner = 'test-owner', repo = 'test-repo', prNumber = 125) {
    const headSha = 'multi123abc456';
    
    this.mockPullRequest(owner, repo, prNumber, { headSha });
    
    const runs = [
      this.createMockRun({
        id: 10001,
        name: 'CI',
        status: 'completed',
        conclusion: 'success',
        created_at: '2024-01-01T09:00:00Z',
        updated_at: '2024-01-01T09:05:00Z'
      }),
      this.createMockRun({
        id: 10002,
        name: 'Security Scan',
        status: 'completed',
        conclusion: 'success',
        created_at: '2024-01-01T09:10:00Z',
        updated_at: '2024-01-01T09:15:00Z'
      })
    ];
    
    this.mockWorkflowRuns(owner, repo, headSha, runs);
    
    // Mock jobs for each run
    runs.forEach((run, index) => {
      const jobs = [
        this.createMockJob({
          id: 20000 + index,
          name: `job-${index + 1}`,
          durationMs: 60000 + (index * 30000)
        })
      ];
      this.mockJobsForRun(owner, repo, run.id, jobs);
    });
    
    return this;
  }

  cleanup() {
    nock.cleanAll();
  }
}

export function createGitHubMock() {
  return new GitHubMock();
} 
