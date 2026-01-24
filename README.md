# GitHub Actions Analyzer

Analyze GitHub Actions performance and visualize workflow timelines in your terminal or [Perfetto UI](https://ui.perfetto.dev).

## Features

- **Terminal Waterfall**: Interactive timeline with clickable links.
- **Perfetto Integration**: Generate Chrome Tracing files for deep analysis.
- **PR Context**: Includes reviews, comments, and merge events in the timeline.
- **Smart Filtering**: Use `--window` to focus on recent activity.

## Usage

```bash
export GITHUB_TOKEN=your_token_here

# Analyze a PR or Commit (supports full URLs or org/repo/pull/123)
go run ./cmd/gha-analyzer nodejs/node/pull/60369

# Save trace for Perfetto visualization
go run ./cmd/gha-analyzer <url> --perfetto=trace.json

# Focus on the last 24 hours of activity
go run ./cmd/gha-analyzer <url> --window=24h
```

## Flags

- `--perfetto=file.json`: Save trace for Perfetto.dev analysis.
- `--window=duration`: Only show events within duration of merge/latest activity (e.g., `24h`, `2h`).
- `--open-in-perfetto`: Automatically open the trace in Perfetto UI.

## License

MIT
