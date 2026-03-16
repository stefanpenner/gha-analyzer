<p align="center">
  <strong>otel-analyzer</strong><br>
  See where your CI/CD time actually goes.
</p>

<p align="center">
  <a href="#install">Install</a> &middot;
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#trends">Trends</a> &middot;
  <a href="#opentelemetry">OpenTelemetry</a>
</p>

---

An interactive terminal tool that turns OpenTelemetry traces and CI/CD runs into navigable timelines — so you can find the slow jobs, the flaky tests, and the queue-time bottlenecks. Works with GitHub Actions, Jenkins, GitLab CI, Buildkite, Dagger, and any system that emits OTel traces.

![Interactive TUI with timeline visualization](docs/tui-screenshot.jpeg)

## Install

```bash
brew install stefanpenner/tap/otel-analyzer
```

Or with Go:

```bash
go install github.com/stefanpenner/otel-analyzer/cmd/otel-analyzer@latest
```

## Quick Start

Point it at any PR or commit:

```bash
otel-analyzer nodejs/node/pull/60369
```

That's it. If you have [GitHub CLI](https://cli.github.com/) installed and authenticated, the token is picked up automatically. Otherwise:

```bash
export GITHUB_TOKEN="your_token_here"
```

## Features

### Interactive TUI

The default view is a full-screen terminal UI with a tree of workflows, jobs, and steps on the left and a Gantt-style timeline on the right. Navigate with arrow keys or vim bindings, expand/collapse nodes, multi-select ranges, search, and drill into details.

### Perfetto Export

Export any analysis as a [Perfetto](https://ui.perfetto.dev) trace for deep-dive visualization with full zoom, search, and flame-chart views:

```bash
otel-analyzer <url> --perfetto=trace.json --open-in-perfetto
```

### Trace Backend Integration

Pull traces directly from Grafana Tempo or Jaeger:

```bash
otel-analyzer --tempo=http://localhost:3200 --trace-id=abc123
otel-analyzer --jaeger=http://localhost:16686 --trace-id=abc123
```

### Webhook Input

Pipe a GitHub Actions webhook payload to analyze the associated commit — useful for event-driven analysis:

```bash
echo '{"workflow_run":{"head_sha":"abc123"},"repository":{"full_name":"owner/repo"}}' \
  | otel-analyzer --otel
```

### Enrichment

Beyond raw timings, the analyzer enriches spans with:

- **Queue time** — how long jobs waited for a runner
- **Runner distribution** — which runners ran which jobs
- **Billable minutes** — computed cost breakdown
- **Retry detection** — identifies re-run jobs and counts attempts
- **PR annotations** — review approvals, comments, merge events shown as markers on the timeline
- **CI/CD pipeline recognition** — auto-classifies spans using [OTel CI/CD semantic conventions](https://opentelemetry.io/docs/specs/semconv/cicd/) (`cicd.pipeline.*` attributes)

## Trends

Analyze workflow performance over time to spot regressions, flaky jobs, and slow-downs:

```bash
otel-analyzer trends owner/repo                          # last 30 days
otel-analyzer trends owner/repo --days=7 --branch=main   # scoped
otel-analyzer trends owner/repo --format=json             # machine-readable
```

```
================================================================================
  Historical Trend Analysis: stefanpenner/otel-analyzer
================================================================================

Summary Statistics
------------------
Average Duration                        1m 46s
Median Duration                         1m 41s
95th Percentile                         3m 13s
Average Success Rate                     61.7%
Trend Direction           Improving (-20.7%)
Flaky Jobs Detected                          1
```

Trend analysis covers success rates, duration percentiles, per-job breakdowns, flaky detection (>10% failure rate), and trend direction. For large repos, it uses stratified temporal sampling to keep API usage reasonable — run-level metrics are always exact, job-level analysis is sampled at 95% confidence / ±10% margin by default.

```bash
otel-analyzer trends owner/repo --no-sample               # exact, more API calls
otel-analyzer trends owner/repo --confidence=0.99 --margin=0.05  # tune sampling
```

## OpenTelemetry

Export analysis data as OpenTelemetry spans — feed them into any observability stack:

```bash
# JSON spans to stdout
otel-analyzer <url> --otel

# OTLP/HTTP
otel-analyzer <url> --otel=localhost:4318

# OTLP/gRPC
otel-analyzer <url> --otel-grpc=localhost:4317
```

You can also **ingest** OTel trace files from any CI/CD system — Jenkins, GitLab CI, Buildkite, Dagger, and anything else that emits traces following the [OTel CI/CD semantic conventions](https://opentelemetry.io/docs/specs/semconv/cicd/):

```bash
otel-analyzer --trace=spans.json
```

## Development

Built with [Bazel](https://bazel.build/) for hermetic, reproducible builds.

```bash
bazel run //:otel-analyzer -- <url>   # run
bazel build //...                     # build all
bazel test //...                      # test all
bazel run //:gazelle                  # regenerate BUILD files
```

## License

MIT
