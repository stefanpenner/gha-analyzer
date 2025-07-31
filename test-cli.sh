#!/bin/bash

# Test script for the updated GitHub Actions Profiler CLI
# Demonstrates both CLI output and Perfetto file output

set -e

echo "🚀 Testing Updated GitHub Actions Profiler CLI"
echo "=============================================="

# Check if GITHUB_TOKEN is set
if [ -z "$GITHUB_TOKEN" ]; then
    echo "❌ Error: GITHUB_TOKEN environment variable is not set"
    echo ""
    echo "Please set your GitHub token:"
    echo "export GITHUB_TOKEN=ghp_your_token_here"
    echo ""
    echo "You can create a token at: https://github.com/settings/tokens"
    exit 1
fi

# Check if PR URL is provided
if [ -z "$1" ]; then
    echo "❌ Error: No PR URL provided"
    echo ""
    echo "Usage: ./test-cli.sh <pr_url>"
    echo ""
    echo "Example:"
    echo "  ./test-cli.sh https://github.com/your-org/your-repo/pull/123"
    exit 1
fi

PR_URL="$1"

echo "📊 Testing CLI with PR: $PR_URL"
echo ""

echo "🔍 Test 1: Default CLI output (stdout)"
echo "----------------------------------------"
node main.mjs "$PR_URL"
echo ""

echo "🔍 Test 2: CLI output + Perfetto file"
echo "--------------------------------------"
node main.mjs "$PR_URL" --perfetto trace.json
echo ""

echo "🔍 Test 3: CLI output + Perfetto file with custom name"
echo "------------------------------------------------------"
node main.mjs "$PR_URL" --perfetto custom-trace.json
echo ""

echo "✅ All tests completed!"
echo ""
echo "📈 Results:"
echo "  - CLI analysis shown above"
echo "  - Perfetto traces saved to: trace.json and custom-trace.json"
echo ""
echo "🌐 To view traces in Perfetto:"
echo "  1. Go to https://perfetto.dev"
echo "  2. Click 'Open trace file'"
echo "  3. Select one of the generated .json files" 