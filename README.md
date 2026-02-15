# GitHub Actions Analyzer

Analyze GitHub Actions performance and visualize workflow timelines directly in your terminal or [Perfetto UI](https://ui.perfetto.dev).

![TUI Screenshot](docs/tui-screenshot.jpeg)

## Install

```bash
go install github.com/stefanpenner/gha-analyzer/cmd/gha-analyzer@latest
```

## Authentication

If you have the [GitHub CLI](https://cli.github.com/) installed and authenticated (`gh auth login`), the token is picked up automatically — no extra setup needed.

Otherwise, set `GITHUB_TOKEN`:

```bash
export GITHUB_TOKEN="your_token_here"
```

## Usage

```bash
# Analyze a PR or Commit
gha-analyzer nodejs/node/pull/60369

# Save and open in Perfetto UI
gha-analyzer <url> --perfetto=trace.json --open-in-perfetto

# Filter to recent activity
gha-analyzer <url> --window=1h
```

## Historical Trend Analysis

Analyze workflow performance trends over time to identify patterns, flaky jobs, and performance regressions:

```bash
# Analyze trends for the last 30 days (default)
gha-analyzer trends owner/repo

# Analyze trends for a specific time period
gha-analyzer trends owner/repo --days=7

# Filter by branch (e.g., only main branch workflows)
gha-analyzer trends owner/repo --branch=main

# Filter by workflow file (e.g., only post-merge workflows)
gha-analyzer trends owner/repo --workflow=post-merge.yaml

# Combine filters for focused analysis
gha-analyzer trends owner/repo --days=14 --branch=main --workflow=ci.yaml

# Export as JSON for programmatic analysis
gha-analyzer trends owner/repo --days=14 --format=json
```

Trend analysis provides:
- **Summary Statistics**: Average, median, and 95th percentile durations
- **Success Rate Trends**: Track workflow reliability over time
- **Duration Trends**: Visualize performance changes with ASCII charts
- **Job Performance**: Identify slowest jobs and their trends
- **Flaky Job Detection**: Automatically detect jobs with inconsistent outcomes (>10% failure rate)
- **Trend Direction**: See if performance is improving, stable, or degrading

### Sampling

For large repositories, fetching job details for every workflow run is expensive (1 API call per run). Trend analysis uses **job-level statistical sampling** to reduce API usage while maintaining accuracy.

**How it works**: All workflow runs are always fetched (cheap — 1 call per 100 runs), so run-level metrics (duration trends, success rates) are exact. Job detail fetching is then sampled using a finite-population formula at 95% confidence with ±10% margin of error. For example, a repo with 1,000 runs in 30 days will fetch job details for ~464 runs instead of all 1,000.

**What's affected**: Job-level analysis (per-job trends, regressions, improvements, flaky detection, queue times) uses the sampled subset. Rare jobs with only a few runs may not appear in sampled results. Trend directions for borderline jobs may vary between runs.

**What's not affected**: Total run count, average/median/p95 duration, success rate, and overall trend direction are always computed from the full dataset.

```bash
# Default: sampling enabled (95% confidence, ±10% margin)
gha-analyzer trends owner/repo --days=30

# Disable sampling for exact results (more API calls)
gha-analyzer trends owner/repo --days=30 --no-sample

# Tune confidence and margin
gha-analyzer trends owner/repo --confidence=0.99 --margin=0.05
```

### Sample Output

```
================================================================================
📈 Historical Trend Analysis: stefanpenner/gha-analyzer
================================================================================

Summary Statistics
------------------
Average Duration                        1m 46s
Median Duration                         1m 41s
95th Percentile                         3m 13s
Average Success Rate                     61.7%
Trend Direction           ✓ Improving (-20.7%)
Flaky Jobs Detected                          1
```

## OTel Output

```bash
# JSON spans to stdout (agent-readable JSONL)
gha-analyzer <url> --otel

# Export via OTLP/HTTP
gha-analyzer <url> --otel=localhost:4318

# Export via OTLP/gRPC
gha-analyzer <url> --otel-grpc
gha-analyzer <url> --otel-grpc=localhost:4317
```

## Webhook Input

Pipe a GitHub Actions webhook payload on stdin to analyze the associated commit:

```bash
echo '{"workflow_run":{"head_sha":"abc123"},"repository":{"full_name":"owner/repo"}}' | gha-analyzer --otel
```

Supports `workflow_run` and `workflow_job` events. When no URL arguments are given and stdin is piped, the webhook is read automatically.

## Development

The project uses [Bazel](https://bazel.build/) for hermetic builds.

```bash
# Run the analyzer
bazel run //:gha-analyzer -- <url>

# Build everything
bazel build //...

# Run all tests
bazel test //...

# Regenerate BUILD.bazel files after adding Go packages
bazel run //:gazelle
```

## License

MIT
