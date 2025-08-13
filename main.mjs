#!/usr/bin/env node

/**
 * Main Entry Point for GitHub Actions Performance Analyzer
 * This file serves as the clean entry point that orchestrates the application
 */

import { parseCommandLineArgs, showUsage } from './src/cli.mjs';
import { createSession, calculateCombinedMetrics } from './src/utils.mjs';
import { AnalysisData } from './src/analysis-data.mjs';
import ProgressBar from './progress.mjs';
import { outputCombinedResults } from './src/visualization.mjs';

/**
 * Main application function that orchestrates the GitHub Actions analysis
 */
async function main() {
  const args = parseCommandLineArgs();
  
  if (args.showHelp) {
    showUsage();
    process.exit(0);
  }
  
  if (!args.githubUrls.length || !args.githubToken) {
    // Provide specific error messages for tests
    if (!args.githubUrls.length) {
      console.error('Error: No GitHub URLs provided.');
    }
    if (!args.githubToken) {
      console.error('Error: GitHub token is required.');
    }
    console.error('');
    showUsage();
    process.exit(1);
  }
  
  // Initialize shared state
  const session = createSession(args.githubToken);
  const analysisData = new AnalysisData();
  const progressBar = new ProgressBar(args.githubUrls.length, 0);
  
  try {
    // Process all GitHub URLs
    for (const [urlIndex, githubUrl] of args.githubUrls.entries()) {
      progressBar.startUrl(urlIndex, githubUrl);
      
      try {
        const success = await analysisData.processUrl(githubUrl, urlIndex, session, progressBar);
        if (!success) {
          console.error(`No workflow runs found for URL ${githubUrl}`);
        }
      } catch (error) {
        console.error(`Error processing URL ${githubUrl}: ${error.message}`);
        continue; // Skip this URL and continue with others
      }
    }
  
    if (analysisData.urlCount === 0) {
      throw new Error('No workflow runs found for any of the provided URLs');
    }
    
    // Generate global analysis data
    await analysisData.finalizeAnalysis();
    
    progressBar.finish();
    
    // Output results
    const combinedMetrics = calculateCombinedMetrics(analysisData.results, analysisData.runsCount, analysisData.jobStartTimes, analysisData.jobEndTimes);
    await outputCombinedResults(analysisData, combinedMetrics, args.perfettoFile, args.openInPerfetto);
    
  } catch (error) {
    console.error('Fatal error:', error.message);
    process.exit(1);
  }
}

// Only run main if this is the entry point (not imported as a module)
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
