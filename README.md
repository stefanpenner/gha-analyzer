# GitHub Actions Performance Profiler

Analyze GitHub Actions workflow performance and generate Chrome Tracing format output for [ui.Perfetto.dev](https://ui.perfetto.dev) visualization.

## Features

- Timeline visualization with job duration bars
- Critical path analysis and bottleneck identification  
- Interactive terminal output with clickable links
- Chrome Tracing format for ui.Perfetto.dev analysis

## TODO
- [ ] release as npm module, so it can be used via npx
- [ ] provide clickable perfetto example
- [ ] see if it's easy enough to bundle perfetto (or similar) UI
- [ ] even more actionable output
- [ ] explore ability to compare multiple runs and provide deeper repo-wide insights
- [ ] explore ability to make this a GHA action itself, so that all PR can surface these details
- [ ] ???

## 📊 Sample Output


### Perfetto UI
Example from one of Nodejs's GHA runs

<img width="1626" height="1042" alt="image" src="https://github.com/user-attachments/assets/7ebf33cb-5caf-4233-9e0d-c5f562a8e6ef" />

### Terminal Analysis
```bash
GitHub Actions Performance Report
================================================================================
Repository: your-org/your-repo
Pull Request: #123 (feature/new-functionality)
Commit: abc123def
Analysis: 3 runs • 22 jobs (peak concurrency: 8) • 156 steps
Success Rate: 100.0% workflows, 86.4% jobs

Pipeline Timeline (22 jobs):
┌──────────────────────────────────────────────────────────────┐
│██                                                            │ Setup  (176.0s)
│█                                                             │  Tests (131.0s)
│███████████████████████████████████████████████████████████   │ Scan (4953.0s)
│ ████                                                         │ Build (415.0s)
└──────────────────────────────────────────────────────────────┘
Legend: █ Success  ▓ Failed  ░ Cancelled/Skipped
Critical path: 1 job, 4953.0s total

Slowest Jobs:
  1. 4953.0s - Scan
  2. 661.0s - Validation
  3. 538.0s - PBuild

Slowest Steps:
  1. 4953.0s - 🔍 Analysis
  2. 496.0s - 🧪 Integration Tests
  3. 422.0s - 🔨 Compile Sources
```

### Perfetto Trace
- **Process-based organization**: Each workflow run as a separate process
- **Hierarchical view**: Jobs and steps properly nested
- **Clickable URLs**: Direct links to GitHub Actions from any event
- **Rich metadata**: PR URLs, commit info, and performance data
- **Professional naming**: "GitHub Actions: your-org/your-repo PR #123"

## 🛠 Installation

```bash
# Clone the repository
git clone https://github.com/your-org/github-actions-profiler.git
cd github-actions-profiler

# Install dependencies (if any are added)
npm install

# Make executable
chmod +x main.mjs
```

## 📖 Usage

### Basic Usage

#### With Environment Variable (Recommended)
```bash
export GITHUB_TOKEN=ghp_your_token_here
node main.mjs https://github.com/owner/repo/pull/123
```

#### With Command Line Token
```bash
node main.mjs https://github.com/owner/repo/pull/123 <github_token>
```

### With Output Redirection
```bash
# Save trace to file (using environment variable)
export GITHUB_TOKEN=ghp_your_token_here
node main.mjs https://github.com/owner/repo/pull/123 > trace.json

# View analysis in terminal while saving trace
node main.mjs https://github.com/owner/repo/pull/123 2>&1 > trace.json

# Or with command line token
node main.mjs https://github.com/owner/repo/pull/123 <token> > trace.json
```

### GitHub Token Setup
```bash
# Create a GitHub personal access token at:
# https://github.com/settings/tokens

# Set as environment variable (recommended)
export GITHUB_TOKEN=ghp_your_token_here

# Now you can run without specifying token each time
node main.mjs https://github.com/owner/repo/pull/123
```

## 🔍 Analysis Features

### Timeline Visualization
- **Visual job execution**: See when jobs run and their duration
- **Concurrency detection**: Identify overlapping vs sequential jobs
- **Critical path**: Find the longest blocking sequence
- **Status indicators**: Success (█), Failed (▓), Cancelled (░)

### Performance Metrics
- **Success rates**: Workflow vs job-level success
- **Concurrency analysis**: Peak concurrent jobs
- **Duration statistics**: Average job and step times
- **Runner utilization**: Track runner types and usage

### Optimization Insights
- **Bottleneck identification**: Find the slowest jobs and steps
- **Parallelization opportunities**: Detect sequential jobs that could run concurrently
- **Resource optimization**: Identify long-running jobs for optimization

## 🎯 Perfetto Integration

### Opening Traces
1. Generate trace: `node main.mjs <pr_url> > trace.json`
2. Open [ui.Perfetto.dev](https://ui.perfetto.dev)
3. Click "Open trace file" and select `trace.json`

### Perfetto Features
- **Process view**: Each workflow run as a separate process
- **Thread hierarchy**: Jobs grouped with their steps
- **Event details**: Click any event to see metadata and URLs
- **Timeline navigation**: Zoom, pan, and search through execution
- **Custom tracks**: Concurrency counters and global metrics

### Clickable Elements
- **Process names**: Link to GitHub Actions
- **Job events**: Link to job logs
- **Step events**: Link to parent job
- **Metadata**: PR URLs and repository links

## 🧪 Testing

```bash
# Run all tests
npm test

# Run tests in watch mode
npm run test:watch

# Run specific test file
node --test test/helpers.test.mjs
```

### Test Coverage
- **Unit tests**: Individual helper functions
- **Integration tests**: Full workflow processing (with mocks)
- **Mock framework**: Realistic GitHub API responses
- **Edge cases**: Error handling and validation

## 🔧 Development

### Project Structure
```
├── main.mjs              # Main profiler script
├── test/
│   ├── main.test.mjs     # Integration tests
│   ├── helpers.test.mjs  # Unit tests
│   ├── github-mock.mjs   # API mocking utilities
│   └── debug.mjs         # Debug utilities
├── package.json          # Project configuration
└── README.md            # This file
```

### Adding Features
1. Add functionality to `main.mjs`
2. Add tests to `test/helpers.test.mjs`
3. Update documentation
4. Run tests: `npm test`

## 📝 Examples

### Analyzing a Failed Pipeline
```bash
# With environment variable
export GITHUB_TOKEN=ghp_your_token_here
node main.mjs https://github.com/your-org/your-repo/pull/123
```

### Comparing Multiple PRs
```bash
# Set token once
export GITHUB_TOKEN=ghp_your_token_here

# Generate traces for comparison
node main.mjs https://github.com/owner/repo/pull/123 > pr-123.json
node main.mjs https://github.com/owner/repo/pull/124 > pr-124.json

# Open both in Perfetto for side-by-side comparison
```


## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Add tests for your changes
4. Ensure all tests pass: `npm test`
5. Commit your changes: `git commit -m 'Add amazing feature'`
6. Push to the branch: `git push origin feature/amazing-feature`
7. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- [UI.Perfetto](https://ui.perfetto.dev) for the excellent tracing visualization
- [GitHub Actions API](https://docs.github.com/en/rest/actions) for workflow data
- Chrome DevTools team for the tracing format specification 
