#!/usr/bin/env node

import { test, describe, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert';
import {
  handleGithubError,
  fetchWithAuth,
  fetchWorkflowRuns,
  fetchRepository,
  fetchCommitAssociatedPRs,
  fetchCommit,
  fetchPRReviews,
  fetchWithPagination
} from '../src/fetching.mjs';

// Mock fetch for testing
let mockFetch;
let mockResponses = new Map();

function setupMockFetch() {
  mockFetch = async (url, options = {}) => {
    const mockResponse = mockResponses.get(url);
    if (mockResponse) {
      return mockResponse;
    }
    
    // Default mock response
    return {
      ok: true,
      status: 200,
      statusText: 'OK',
      headers: {
        get: (name) => {
          if (name === 'content-type') return 'application/json';
          return null;
        }
      },
      json: async () => ({}),
      text: async () => ''
    };
  };
  
  // Replace global fetch
  global.fetch = mockFetch;
}

function teardownMockFetch() {
  if (global.fetch === mockFetch) {
    global.fetch = undefined;
  }
  mockResponses.clear();
}

function mockResponse(url, response) {
  mockResponses.set(url, response);
}

describe('Fetching Module Tests', () => {
  beforeEach(() => {
    setupMockFetch();
  });

  afterEach(() => {
    teardownMockFetch();
  });

  describe('handleGithubError', () => {
    test('should handle 401 authentication errors', async () => {
      const response = {
        status: 401,
        statusText: 'Unauthorized',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ message: 'Bad credentials' })
      };

      await assert.rejects(
        async () => await handleGithubError(response, 'https://api.github.com/test'),
        /Authentication failed/
      );
    });

    test('should handle 403 SSO required errors', async () => {
      const response = {
        status: 403,
        statusText: 'Forbidden',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            if (name === 'x-github-sso') return 'required; url=https://github.com/orgs/test/sso';
            return null;
          }
        },
        json: async () => ({ message: 'SSO required' })
      };

      await assert.rejects(
        async () => await handleGithubError(response, 'https://api.github.com/test'),
        /SSO requirement/
      );
    });

    test('should handle 403 rate limit errors', async () => {
      const response = {
        status: 403,
        statusText: 'Forbidden',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            if (name === 'x-ratelimit-remaining') return '0';
            return null;
          }
        },
        json: async () => ({ message: 'Rate limit exceeded' })
      };

      await assert.rejects(
        async () => await handleGithubError(response, 'https://api.github.com/test'),
        /API rate limit reached/
      );
    });

    test('should handle 404 not found errors', async () => {
      const response = {
        status: 404,
        statusText: 'Not Found',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ message: 'Not found' })
      };

      await assert.rejects(
        async () => await handleGithubError(response, 'https://api.github.com/test'),
        /Not found/
      );
    });

    test('should handle generic errors', async () => {
      const response = {
        status: 500,
        statusText: 'Internal Server Error',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ message: 'Internal error' })
      };

      await assert.rejects(
        async () => await handleGithubError(response, 'https://api.github.com/test'),
        /Error fetching https:\/\/api\.github\.com\/test: 500 Internal Server Error/
      );
    });
  });

  describe('fetchWithAuth', () => {
    test('should make authenticated request with correct headers', async () => {
      const context = { githubToken: 'test-token' };
      const url = 'https://api.github.com/repos/owner/repo';
      
      mockResponse(url, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ name: 'test-repo' })
      });

      const result = await fetchWithAuth(url, context);
      
      assert.strictEqual(result.name, 'test-repo');
    });

    test('should throw error for non-ok response', async () => {
      const context = { githubToken: 'test-token' };
      const url = 'https://api.github.com/repos/owner/repo';
      
      mockResponse(url, {
        ok: false,
        status: 404,
        statusText: 'Not Found',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ message: 'Not found' })
      });

      await assert.rejects(
        async () => await fetchWithAuth(url, context),
        /Not found/
      );
    });
  });

  describe('fetchWorkflowRuns', () => {
    test('should fetch workflow runs with correct parameters', async () => {
      const baseUrl = 'https://api.github.com/repos/owner/repo';
      const headSha = 'abc123';
      const context = { githubToken: 'test-token' };
      const options = { branch: 'main', event: 'push' };
      
      const expectedUrl = `${baseUrl}/actions/runs?head_sha=${headSha}&per_page=100&branch=main&event=push`;
      
      mockResponse(expectedUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ id: 1, name: 'test-workflow' }]
      });

      const result = await fetchWorkflowRuns(baseUrl, headSha, context, options);
      
      assert.strictEqual(result.length, 1);
      assert.strictEqual(result[0].id, 1);
      assert.strictEqual(result[0].name, 'test-workflow');
    });

    test('should fetch workflow runs without options', async () => {
      const baseUrl = 'https://api.github.com/repos/owner/repo';
      const headSha = 'abc123';
      const context = { githubToken: 'test-token' };
      
      const expectedUrl = `${baseUrl}/actions/runs?head_sha=${headSha}&per_page=100`;
      
      mockResponse(expectedUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ id: 1, name: 'test-workflow' }]
      });

      const result = await fetchWorkflowRuns(baseUrl, headSha, context);
      
      assert.strictEqual(result.length, 1);
    });
  });

  describe('fetchRepository', () => {
    test('should fetch repository information', async () => {
      const baseUrl = 'https://api.github.com/repos/owner/repo';
      const context = { githubToken: 'test-token' };
      
      mockResponse(baseUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ name: 'test-repo', default_branch: 'main' })
      });

      const result = await fetchRepository(baseUrl, context);
      
      assert.strictEqual(result.name, 'test-repo');
      assert.strictEqual(result.default_branch, 'main');
    });
  });

  describe('fetchCommitAssociatedPRs', () => {
    test('should fetch PRs associated with commit', async () => {
      const owner = 'owner';
      const repo = 'repo';
      const sha = 'abc123';
      const context = { githubToken: 'test-token' };
      
      const expectedUrl = `https://api.github.com/repos/${owner}/${repo}/commits/${sha}/pulls?per_page=100`;
      
      mockResponse(expectedUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ number: 123, title: 'Test PR' }]
      });

      const result = await fetchCommitAssociatedPRs(owner, repo, sha, context);
      
      assert.strictEqual(result.length, 1);
      assert.strictEqual(result[0].number, 123);
      assert.strictEqual(result[0].title, 'Test PR');
    });

    test('should handle error response', async () => {
      const owner = 'owner';
      const repo = 'repo';
      const sha = 'abc123';
      const context = { githubToken: 'test-token' };
      
      const expectedUrl = `https://api.github.com/repos/${owner}/${repo}/commits/${sha}/pulls?per_page=100`;
      
      mockResponse(expectedUrl, {
        ok: false,
        status: 404,
        statusText: 'Not Found',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ message: 'Not found' })
      });

      await assert.rejects(
        async () => await fetchCommitAssociatedPRs(owner, repo, sha, context),
        /Not found/
      );
    });
  });

  describe('fetchCommit', () => {
    test('should fetch commit information', async () => {
      const baseUrl = 'https://api.github.com/repos/owner/repo';
      const sha = 'abc123';
      const context = { githubToken: 'test-token' };
      
      const expectedUrl = `${baseUrl}/commits/${sha}`;
      
      mockResponse(expectedUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ sha: 'abc123', message: 'Test commit' })
      });

      const result = await fetchCommit(baseUrl, sha, context);
      
      assert.strictEqual(result.sha, 'abc123');
      assert.strictEqual(result.message, 'Test commit');
    });
  });

  describe('fetchPRReviews', () => {
    test('should fetch PR reviews', async () => {
      const owner = 'owner';
      const repo = 'repo';
      const prNumber = '123';
      const context = { githubToken: 'test-token' };
      
      const expectedUrl = `https://api.github.com/repos/${owner}/${repo}/pulls/${prNumber}/reviews?per_page=100`;
      
      mockResponse(expectedUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ id: 1, state: 'APPROVED', user: { login: 'reviewer' } }]
      });

      const result = await fetchPRReviews(owner, repo, prNumber, context);
      
      assert.strictEqual(result.length, 1);
      assert.strictEqual(result[0].state, 'APPROVED');
      assert.strictEqual(result[0].user.login, 'reviewer');
    });
  });

  describe('fetchWithPagination', () => {
    test('should handle single page response', async () => {
      const url = 'https://api.github.com/repos/owner/repo/actions/runs';
      const context = { githubToken: 'test-token' };
      
      mockResponse(url, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ id: 1, name: 'workflow-1' }]
      });

      const result = await fetchWithPagination(url, context);
      
      assert.strictEqual(result.length, 1);
      assert.strictEqual(result[0].id, 1);
    });

    test('should handle paginated response', async () => {
      const url = 'https://api.github.com/repos/owner/repo/actions/runs';
      const context = { githubToken: 'test-token' };
      const nextUrl = 'https://api.github.com/repos/owner/repo/actions/runs?page=2';
      
      // First page
      mockResponse(url, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            if (name === 'Link') {
              return '<' + nextUrl + '>; rel="next"';
            }
            return null;
          }
        },
        json: async () => [{ id: 1, name: 'workflow-1' }]
      });

      // Second page
      mockResponse(nextUrl, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ id: 2, name: 'workflow-2' }]
      });

      const result = await fetchWithPagination(url, context);
      
      assert.strictEqual(result.length, 2);
      assert.strictEqual(result[0].id, 1);
      assert.strictEqual(result[1].id, 2);
    });

    test('should handle array response', async () => {
      const url = 'https://api.github.com/repos/owner/repo/actions/runs';
      const context = { githubToken: 'test-token' };
      
      mockResponse(url, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => [{ id: 1 }, { id: 2 }]
      });

      const result = await fetchWithPagination(url, context);
      
      assert.strictEqual(result.length, 2);
    });

    test('should handle object response with workflow_runs', async () => {
      const url = 'https://api.github.com/repos/owner/repo/actions/runs';
      const context = { githubToken: 'test-token' };
      
      mockResponse(url, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ workflow_runs: [{ id: 1 }, { id: 2 }] })
      });

      const result = await fetchWithPagination(url, context);
      
      assert.strictEqual(result.length, 2);
    });

    test('should handle object response with jobs', async () => {
      const url = 'https://api.github.com/repos/owner/repo/actions/runs/1/jobs';
      const context = { githubToken: 'test-token' };
      
      mockResponse(url, {
        ok: true,
        status: 200,
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ jobs: [{ id: 1 }, { id: 2 }] })
      });

      const result = await fetchWithPagination(url, context);
      
      assert.strictEqual(result.length, 2);
    });

    test('should throw error for non-ok response', async () => {
      const url = 'https://api.github.com/repos/owner/repo/actions/runs';
      const context = { githubToken: 'test-token' };
      
      mockResponse(url, {
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
        headers: {
          get: (name) => {
            if (name === 'content-type') return 'application/json';
            return null;
          }
        },
        json: async () => ({ message: 'Internal error' })
      });

      await assert.rejects(
        async () => await fetchWithPagination(url, context),
        /Internal error/
      );
    });
  });
});
