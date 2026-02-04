# GitHub Actions Analyzer

Analyze GitHub Actions performance and visualize workflow timelines directly in your terminal or [Perfetto UI](https://ui.perfetto.dev).

![TUI Screenshot](docs/tui-screenshot.jpeg)

## Install

```bash
go install github.com/stefanpenner/gha-analyzer/cmd/gha-analyzer@latest
```

## Usage

```bash
export GITHUB_TOKEN="$(gh auth token)"

# Analyze a PR or Commit
gha-analyzer nodejs/node/pull/60369

# Save and open in Perfetto UI
gha-analyzer <url> --perfetto=trace.json --open-in-perfetto

# Filter to recent activity
gha-analyzer <url> --window=1h
```

## License

MIT
