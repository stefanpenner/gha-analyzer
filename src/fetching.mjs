/**
 * GitHub API fetching utilities
 * Handles authentication, error handling, and pagination for GitHub API requests
 */

/**
 * Handle GitHub API error responses with detailed error messages
 * @param {Response} response - Fetch response object
 * @param {string} requestUrl - The URL that was requested
 * @throws {Error} Detailed error message with troubleshooting steps
 */
export async function handleGithubError(response, requestUrl) {
  const status = response.status;
  const statusText = response.statusText || '';

  let bodyText = '';
  let message = '';
  let documentationUrl = '';
  try {
    const contentType = response.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
      const data = await response.json();
      message = data?.message || '';
      documentationUrl = data?.documentation_url || '';
    } else {
      bodyText = await response.text();
    }
  } catch {
    // ignore parse errors
  }

  const ssoHeader = response.headers.get('x-github-sso');
  const oauthScopes = response.headers.get('x-oauth-scopes');
  const acceptedScopes = response.headers.get('x-accepted-oauth-scopes');
  const rateRemaining = response.headers.get('x-ratelimit-remaining');

  const base = `Error fetching ${requestUrl}: ${status} ${statusText}`;
  const detail = message || bodyText || '';

  if (status === 401) {
    const lines = [
      base + (detail ? ` - ${detail}` : ''),
      '➡️  Authentication failed. Ensure a valid token (env GITHUB_TOKEN or CLI arg).',
      '   - Fine-grained PAT: grant repository access and Read for: Contents, Actions, Pull requests, Checks.',
      '   - Classic PAT: include repo scope for private repos.'
    ];
    if (documentationUrl) lines.push(`   Docs: ${documentationUrl}`);
    throw new Error(lines.join('\n'));
  }

  if (status === 403) {
    if (ssoHeader && /required/i.test(ssoHeader)) {
      const match = ssoHeader.match(/url=([^;\s]+)/i);
      const ssoUrl = match ? match[1] : null;
      const lines = [
        base + (detail ? ` - ${detail}` : ''),
        '❌ GitHub API request forbidden due to SSO requirement for this token.'
      ];
      if (ssoUrl) {
        lines.push(`➡️  Authorize SSO for this token by visiting:\n   ${ssoUrl}\nThen re-run the command.`);
      } else {
        lines.push('➡️  Authorize SSO for this token in your organization, then re-run.');
      }
      throw new Error(lines.join('\n'));
    }

    if (rateRemaining === '0') {
      const lines = [
        base + (detail ? ` - ${detail}` : ''),
        '➡️  API rate limit reached. Wait for reset or use an authenticated token with higher limits.'
      ];
      throw new Error(lines.join('\n'));
    }

    const lines = [
      base + (detail ? ` - ${detail}` : ''),
      '➡️  Permission issue. Verify token access to this repository and required scopes.',
    ];
    if (acceptedScopes) lines.push(`   Required scopes (server hint): ${acceptedScopes}`);
    if (oauthScopes) lines.push(`   Your token scopes: ${oauthScopes || '(none reported)'}`);
    lines.push('   - Fine-grained PAT: grant repo access and Read for Contents, Actions, Pull requests, Checks.');
    lines.push('   - Classic PAT: include repo scope for private repos.');
    if (documentationUrl) lines.push(`   Docs: ${documentationUrl}`);
    throw new Error(lines.join('\n'));
  }

  if (status === 404) {
    const lines = [
      base + (detail ? ` - ${detail}` : ''),
      '➡️  Not found. On private repos, 404 can indicate insufficient token access. Check repository access and scopes.'
    ];
    throw new Error(lines.join('\n'));
  }

  throw new Error(base + (detail ? ` - ${detail}` : ''));
}

/**
 * Fetch from GitHub API with authentication
 * @param {string} url - The URL to fetch from
 * @param {Object} context - Context object containing githubToken
 * @returns {Promise<Object>} JSON response data
 */
export async function fetchWithAuth(url, context) {
  const headers = {
    Authorization: `token ${context.githubToken}`,
    Accept: 'application/vnd.github.v3+json',
    'User-Agent': 'Node'
  };
  
  const response = await fetch(url, { headers });
  if (!response.ok) {
    throw await handleGithubError(response, url);
  }
  return response.json();
}

/**
 * Fetch workflow runs for a specific commit
 * @param {string} baseUrl - Base repository API URL
 * @param {string} headSha - Commit SHA to fetch runs for
 * @param {Object} context - Context object containing githubToken
 * @param {Object} options - Additional options (branch, event)
 * @returns {Promise<Array>} Array of workflow runs
 */
export async function fetchWorkflowRuns(baseUrl, headSha, context, options = {}) {
  const params = new URLSearchParams();
  params.set('head_sha', headSha);
  params.set('per_page', '100');
  if (options.branch) {
    params.set('branch', options.branch);
  }
  if (options.event) {
    params.set('event', options.event);
  }
  const commitRunsUrl = `${baseUrl}/actions/runs?${params.toString()}`;
  return await fetchWithPagination(commitRunsUrl, context);
}

/**
 * Fetch repository information
 * @param {string} baseUrl - Base repository API URL
 * @param {Object} context - Context object containing githubToken
 * @returns {Promise<Object>} Repository data
 */
export async function fetchRepository(baseUrl, context) {
  return await fetchWithAuth(baseUrl, context);
}

/**
 * Fetch PRs associated with a commit
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} sha - Commit SHA
 * @param {Object} context - Context object containing githubToken
 * @returns {Promise<Array>} Array of associated PRs
 */
export async function fetchCommitAssociatedPRs(owner, repo, sha, context) {
  const endpoint = `https://api.github.com/repos/${owner}/${repo}/commits/${sha}/pulls?per_page=100`;
  const headers = {
    Authorization: `token ${context.githubToken}`,
    Accept: 'application/vnd.github+json',
    'User-Agent': 'Node'
  };
  const response = await fetch(endpoint, { headers });
  if (!response.ok) {
    await handleGithubError(response, endpoint);
  }
  return await response.json();
}

/**
 * Fetch commit information
 * @param {string} baseUrl - Base repository API URL
 * @param {string} sha - Commit SHA
 * @param {Object} context - Context object containing githubToken
 * @returns {Promise<Object>} Commit data
 */
export async function fetchCommit(baseUrl, sha, context) {
  const commitUrl = `${baseUrl}/commits/${sha}`;
  return await fetchWithAuth(commitUrl, context);
}

/**
 * Fetch PR reviews
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @param {string} prNumber - PR number
 * @param {Object} context - Context object containing githubToken
 * @returns {Promise<Array>} Array of PR reviews
 */
export async function fetchPRReviews(owner, repo, prNumber, context) {
  const reviewsUrl = `https://api.github.com/repos/${owner}/${repo}/pulls/${prNumber}/reviews?per_page=100`;
  return await fetchWithPagination(reviewsUrl, context);
}

/**
 * Fetch from GitHub API with pagination support
 * @param {string} url - The URL to fetch from
 * @param {Object} context - Context object containing githubToken
 * @returns {Promise<Array>} Array of all items from all pages
 */
export async function fetchWithPagination(url, context) {
  const headers = {
    Authorization: `token ${context.githubToken}`,
    Accept: 'application/vnd.github.v3+json',
    'User-Agent': 'Node'
  };
  
  const allItems = [];
  let currentUrl = url;
  
  while (currentUrl) {
    const response = await fetch(currentUrl, { headers });
    if (!response.ok) {
      throw await handleGithubError(response, currentUrl);
    }
    
    const data = await response.json();
    const items = Array.isArray(data) ? data : data.workflow_runs || data.jobs || [];
    allItems.push(...items);
    
    const linkHeader = response.headers.get('Link');
    currentUrl = linkHeader?.match(/<([^>]+)>;\s*rel="next"/)?.[1] || null;
  }
  
  return allItems;
}
