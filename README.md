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

## License

MIT
