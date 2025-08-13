/**
 * Command Line Interface Module
 * Handles all command line argument parsing and usage display
 */

import { parseArgs } from 'node:util';

export function parseCommandLineArgs() {
  const {
    values: { perfetto, 'open-in-perfetto': openInPerfetto, help },
    positionals
  } = parseArgs({
    options: {
      perfetto: { type: 'string' },
      'open-in-perfetto': { type: 'boolean' },
      help: { type: 'boolean', short: 'h' }
    },
    allowPositionals: true
  });
  
  if (help) {
    return { showHelp: true };
  }
  
  // Extract GitHub URLs and token from positionals
  const githubUrls = [];
  let githubToken = null;
  
  for (const arg of positionals) {
    if (arg.startsWith('http')) {
      githubUrls.push(arg);
    } else if (!githubToken) {
      githubToken = arg;
    }
  }
  
  // Fallback to environment variable if no token provided
  if (!githubToken) {
    githubToken = process.env.GITHUB_TOKEN;
  }
  
  return {
    githubUrls,
    githubToken,
    perfettoFile: perfetto || null,
    openInPerfetto: openInPerfetto || false,
    showHelp: false
  };
}

export function showUsage() {
  console.error('GitHub Actions Performance Analyzer');
  console.error('');
  console.error('Usage: node main.mjs <github_url1> [github_url2] ... [token] [options]');
  console.error('');
  console.error('Arguments:');
  console.error('  <github_urls>    One or more GitHub PR or commit URLs');
  console.error('  [token]          GitHub token (optional if GITHUB_TOKEN env var is set)');
  console.error('');
  console.error('Options:');
  console.error('  --perfetto=<file>     Save trace to file for Perfetto analysis');
  console.error('  --open-in-perfetto    Automatically open trace in Perfetto UI');
  console.error('  -h, --help            Show this help message');
  console.error('');
  console.error('Supported URL formats:');
  console.error('  PR: https://github.com/owner/repo/pull/123');
  console.error('  Commit: https://github.com/owner/repo/commit/abc123...');
  console.error('');
  console.error('Examples:');
  console.error('  Single URL: node main.mjs https://github.com/owner/repo/pull/123');
  console.error('  Multiple URLs: node main.mjs https://github.com/owner/repo/pull/123 https://github.com/owner/repo/commit/abc123');
  console.error('  With token: node main.mjs https://github.com/owner/repo/pull/123 your_token');
  console.error('  With perfetto output: node main.mjs https://github.com/owner/repo/pull/123 --perfetto=trace.json');
  console.error('  Auto-open in perfetto: node main.mjs https://github.com/owner/repo/pull/123 --perfetto=trace.json --open-in-perfetto');
}
