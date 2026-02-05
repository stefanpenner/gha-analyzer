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

## License

MIT
