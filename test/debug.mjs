import { createGitHubMock } from './github-mock.mjs';

// Simple debug test
async function debugTest() {
  console.log('Setting up GitHub mock...');
  const githubMock = createGitHubMock();
  githubMock.mockSuccessfulPipeline('test-owner', 'test-repo', 123);
  
  console.log('Mock endpoints set up:');
  console.log('- PR: /repos/test-owner/test-repo/pulls/123');
  console.log('- Runs: /repos/test-owner/test-repo/actions/runs?head_sha=abc123def456');
  console.log('- Jobs: /repos/test-owner/test-repo/actions/runs/12345/jobs');
  
  console.log('\nTry running:');
  console.log('node main.mjs https://github.com/test-owner/test-repo/pull/123 fake-token');
}

debugTest().catch(console.error); 
