# Before/After: GHA → OTel Trace Mapping

## Before (current)

A workflow run produces spans like this (simplified JSON):

```json
{
  "Name": "CI",
  "SpanContext": {"TraceID": "a1b2c3...", "SpanID": "1111..."},
  "Attributes": [
    {"Key": "type", "Value": {"Type": "STRING", "Value": "workflow"}},
    {"Key": "github.run_id", "Value": {"Type": "INT64", "Value": 12345}},
    {"Key": "github.status", "Value": {"Type": "STRING", "Value": "completed"}},
    {"Key": "github.conclusion", "Value": {"Type": "STRING", "Value": "success"}},
    {"Key": "github.repo", "Value": {"Type": "STRING", "Value": "owner/repo"}},
    {"Key": "github.url", "Value": {"Type": "STRING", "Value": "https://github.com/owner/repo/actions/runs/12345"}}
  ],
  "Status": {"Code": "Unset"},
  "Events": null,
  "Links": null
}
```

Reviews/merges were **separate zero-duration "marker" spans**:

```json
{
  "Name": "Review: APPROVED",
  "SpanContext": {"TraceID": "0000...", "SpanID": "aaaa..."},
  "Attributes": [
    {"Key": "type", "Value": {"Type": "STRING", "Value": "marker"}},
    {"Key": "github.event_type", "Value": {"Type": "STRING", "Value": "approved"}},
    {"Key": "github.user", "Value": {"Type": "STRING", "Value": "alice"}}
  ],
  "Status": {"Code": "Unset"},
  "Events": null,
  "Links": null
}
```

**Problems**:
- No OTel CI/CD semantic conventions
- Reviews as fake spans (no trace_id, orphaned)
- No OTel status codes (always Unset)
- No InstrumentationScope
- No span links for retries
- All attributes are GitHub-specific, opaque to OTel backends

---

## After (canonical mapping)

### Workflow root span

```json
{
  "Name": "CI",
  "SpanContext": {"TraceID": "a1b2c3...", "SpanID": "1111..."},
  "InstrumentationLibrary": {
    "Name": "github.com/stefanpenner/otel-explorer/pkg/analyzer"
  },
  "Attributes": [
    {"Key": "cicd.pipeline.name", "Value": {"Type": "STRING", "Value": "CI"}},
    {"Key": "cicd.pipeline.run.id", "Value": {"Type": "STRING", "Value": "12345"}},
    {"Key": "cicd.pipeline.run.url.full", "Value": {"Type": "STRING", "Value": "https://github.com/owner/repo/actions/runs/12345"}},
    {"Key": "cicd.pipeline.run.result", "Value": {"Type": "STRING", "Value": "success"}},
    {"Key": "vcs.repository.url.full", "Value": {"Type": "STRING", "Value": "https://github.com/owner/repo"}},
    {"Key": "vcs.revision", "Value": {"Type": "STRING", "Value": "abc123def"}},
    {"Key": "vcs.ref.head.name", "Value": {"Type": "STRING", "Value": "main"}},
    {"Key": "type", "Value": {"Type": "STRING", "Value": "workflow"}},
    {"Key": "github.run_id", "Value": {"Type": "INT64", "Value": 12345}},
    {"Key": "github.status", "Value": {"Type": "STRING", "Value": "completed"}},
    {"Key": "github.conclusion", "Value": {"Type": "STRING", "Value": "success"}}
  ],
  "Status": {"Code": "Ok"},
  "Events": [
    {
      "Name": "github.review",
      "Time": "2024-06-15T10:30:00Z",
      "Attributes": [
        {"Key": "github.review.state", "Value": {"Type": "STRING", "Value": "APPROVED"}},
        {"Key": "github.user", "Value": {"Type": "STRING", "Value": "alice"}}
      ]
    },
    {
      "Name": "github.merge",
      "Time": "2024-06-15T11:00:00Z",
      "Attributes": [
        {"Key": "github.user", "Value": {"Type": "STRING", "Value": "bob"}},
        {"Key": "github.pr_number", "Value": {"Type": "INT64", "Value": 42}}
      ]
    },
    {
      "Name": "github.commit",
      "Time": "2024-06-15T09:00:00Z",
      "Attributes": [
        {"Key": "vcs.revision", "Value": {"Type": "STRING", "Value": "abc123def"}}
      ]
    }
  ],
  "Links": null
}
```

### Job span (child of workflow)

```json
{
  "Name": "build",
  "SpanContext": {"TraceID": "a1b2c3...", "SpanID": "2222..."},
  "Parent": {"TraceID": "a1b2c3...", "SpanID": "1111..."},
  "Attributes": [
    {"Key": "cicd.pipeline.task.name", "Value": {"Type": "STRING", "Value": "build"}},
    {"Key": "cicd.pipeline.task.type", "Value": {"Type": "STRING", "Value": "build"}},
    {"Key": "cicd.pipeline.task.run.id", "Value": {"Type": "STRING", "Value": "67890"}},
    {"Key": "cicd.pipeline.task.run.result", "Value": {"Type": "STRING", "Value": "failure"}},
    {"Key": "cicd.pipeline.task.run.url.full", "Value": {"Type": "STRING", "Value": "https://github.com/owner/repo/actions/runs/12345/job/67890"}},
    {"Key": "type", "Value": {"Type": "STRING", "Value": "job"}}
  ],
  "Status": {"Code": "Error", "Description": "failure"},
  "Events": [
    {
      "Name": "Test assertion failed",
      "Attributes": [
        {"Key": "annotation.path", "Value": {"Type": "STRING", "Value": "test/foo_test.go"}},
        {"Key": "annotation.line", "Value": {"Type": "INT64", "Value": 42}},
        {"Key": "annotation.level", "Value": {"Type": "STRING", "Value": "failure"}}
      ]
    }
  ]
}
```

### Retry attempt (span link to previous)

```json
{
  "Name": "CI",
  "SpanContext": {"TraceID": "d4e5f6...", "SpanID": "1111..."},
  "Attributes": [
    {"Key": "cicd.pipeline.name", "Value": {"Type": "STRING", "Value": "CI"}},
    {"Key": "github.run_attempt", "Value": {"Type": "INT64", "Value": 2}}
  ],
  "Links": [
    {
      "SpanContext": {"TraceID": "a1b2c3...", "SpanID": "1111..."},
      "Attributes": [
        {"Key": "link.type", "Value": {"Type": "STRING", "Value": "retry"}},
        {"Key": "github.previous_attempt", "Value": {"Type": "INT64", "Value": 1}}
      ]
    }
  ]
}
```

---

## Summary of changes

| Aspect | Before | After |
|---|---|---|
| Attributes | GitHub-specific only | CI/CD semconv + GitHub-specific |
| Reviews/merges | Fake marker spans (orphaned) | Span events on workflow root |
| Status | Always `Unset` | Mapped: success→Ok, failure→Error |
| InstrumentationScope | Empty | Set to analyzer package |
| Retry links | None | Span link to previous attempt |
| Backward compat | — | Legacy marker spans still emitted |

## Breaking changes

1. **Marker spans are now also span events**: The enricher chain still recognizes legacy marker spans, but new OTel-native consumers should use span events on the workflow root span instead.

2. **`is_required` → `github.is_required`**: Attribute key renamed for namespace consistency.
