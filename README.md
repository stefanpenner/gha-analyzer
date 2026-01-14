# GitHub Actions Performance Profiler

Analyze GitHub Actions workflow performance and generate Chrome Tracing format output for [ui.Perfetto.dev](https://ui.perfetto.dev) visualization.

## Features

- Timeline visualization with job duration bars
- Critical path analysis and bottleneck identification  
- Interactive terminal output with clickable links
- Chrome Tracing format for ui.Perfetto.dev analysis

## Requirements

- Go 1.25+

## 📊 Sample Output

### Perfetto UI
Example from one of Node.js’s GHA runs

<img width="1626" height="1042" alt="image" src="https://github.com/user-attachments/assets/7ebf33cb-5caf-4233-9e0d-c5f562a8e6ef" />

### Terminal Analysis (Text)
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


### Basic Usage

#### With Environment Variable (Recommended)
```bash
export GITHUB_TOKEN=ghp_your_token_here
go run ./cmd/gha-analyzer https://github.com/owner/repo/pull/123 <commit-or-pr-urls...>
```

#### With Command Line Token
```bash
go run ./cmd/gha-analyzer https://github.com/owner/repo/pull/123 <github_token>
```

### GitHub Token Setup
```bash
# 1. Create a GitHub personal access token at https://github.com/settings/tokens
# 2. should work with repo access
# 3. if in enterprise authorize the token for the appropriate org: 

# Set as environment variable (recommended)
export GITHUB_TOKEN=ghp_your_token_here

# Now you can run without specifying token each time
go run ./cmd/gha-analyzer https://github.com/owner/repo/pull/123
```

### Markdown Output

```bash
go run ./cmd/gha-analyzer https://github.com/owner/repo/pull/123 --format=markdown
```

### Perfetto Trace Output

```bash
go run ./cmd/gha-analyzer https://github.com/owner/repo/pull/123 --perfetto=trace.json
```

### Build

```bash
go build ./cmd/gha-analyzer
./gha-analyzer https://github.com/owner/repo/pull/123
```

## 🧪 Testing

```bash
# Run all tests
go test ./...
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Add tests for your changes
4. Ensure all tests pass: `go test ./...`
5. Commit your changes: `git commit -m 'Add amazing feature'`
6. Push to the branch: `git push origin feature/amazing-feature`
7. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- [UI.Perfetto](https://ui.perfetto.dev) for the excellent tracing visualization
- [GitHub Actions API](https://docs.github.com/en/rest/actions) for workflow data
- Chrome DevTools team for the tracing format specification 
