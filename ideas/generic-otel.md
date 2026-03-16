# Generic OTel Trace Analyzer — Strategic Assessment

## 1. Is This a Good Idea?

**Yes — but only if you stay in the "opinionated CLI trace analyzer" lane.**

The unique value is **analysis, not display**:
- No other OTel viewer has `SpanHints` — transforming raw attributes into *interpreted* presentation (outcome, icon, grouping, dedup, required status)
- Changepoint detection, bottleneck identification, concurrency analysis — analytical features no general viewer offers
- Zero-infrastructure TUI: `otel-analyzer --trace=spans.json` gives you an interactive waterfall in your terminal. No Docker, no browser, no server. Useful in SSH sessions, CI, quick local iteration

**Where it would go wrong:**
- Building a web UI (puts you against Jaeger/Grafana where you lose)
- Building an OTLP collector/storage system (that's OTel Collector + Tempo)
- Trying to comprehensively support every OTel semantic convention (focus on the *framework* being extensible)

**Positioning:** "An opinionated trace analyzer for your terminal. Bring traces from anywhere — GHA, files, Tempo, Jaeger. We don't just show spans, we analyze them."

---

## 2. OSS OTel Sources Worth Targeting

### Tier 1: High-value, already OTLP, natural fit

| Source | How it emits OTel | Why it's interesting |
|--------|------------------|---------------------|
| **Dagger** | Native. Set `OTEL_EXPORTER_OTLP_ENDPOINT`. Every function call = span. | Richest CI/CD trace source. Deep nesting → bottleneck/concurrency analysis shines |
| **Jenkins** | `opentelemetry-plugin`. OTLP gRPC/HTTP. Stages/steps as spans. | Huge install base. Gradle/Maven sub-builds nest inside. |
| **GitLab CI** | Native (16+). `GITLAB_OBSERVABILITY_EXPORT=traces`. Pipeline→job→script. | Second-largest CI system. |
| **Buildkite** | Native agent. `BUILDKITE_AGENT_TRACING_BACKEND=opentelemetry`. | Clean OTLP output. |
| **Gradle** | `opentelemetry-gradle-plugin`. Per-task + per-test spans. | Built for "find bottlenecks" — exact same pitch. |
| **Maven** | Official OTel extension (otel-java-contrib). | Lifecycle phases as spans. |
| **Docker BuildKit** | Native. Set `OTEL_EXPORTER_OTLP_ENDPOINT`. | Layer cache analysis, multi-stage parallelism. |
| **pytest-opentelemetry** | `pytest --export-traces`. Session→module→class→function. | Test duration regression → changepoint detection. Direct mapping. |

### Tier 2: Unique differentiator

**Bazel** — no OTLP yet, but emits Chrome trace-event JSON profiles (`--profile=`). You dogfood Bazel. A Bazel-profile → OTLP importer would be genuinely unique. The profile shows action parallelism, cache hits, critical path — maps directly to concurrency/bottleneck analysis.

### Tier 3: Infrastructure (broader, less unique)

- **Terraform/Terragrunt** — plan/apply as spans, per-resource children
- **Kubernetes control plane** — apiserver/kubelet emit OTLP natively
- **ArgoCD** — sync operations as spans
- **Tekton** — formal TEP for distributed tracing

### Key enabler: OTel CI/CD Semantic Conventions (v1.27+)

Standard attributes: `cicd.pipeline.name`, `cicd.pipeline.run.id`, `cicd.pipeline.task.run.result`, `vcs.repo`, `vcs.ref`. Jenkins, GitLab, Buildkite, Dagger, and Tekton are converging on these. A single `CICDEnricher` recognizing these attributes makes the tool work across *all* CI/CD systems without per-system code.

### Demo/testing data
- `brew install krzko/tap/otelgen` — synthetic OTLP trace generator
- `telemetrygen` — official OTel collector-contrib tool
- OTel Demo App — 14+ microservices with Jaeger/Tempo
- `opentelemetry-proto/examples/trace.json` — reference OTLP JSON

---

## 3. What Remains (Gaps)

### Already generic (no work needed)
- Pipeline + Exporter architecture
- Tree building from parent-child spans
- TUI rendering (works with any enriched spans)
- OTLP file/Tempo/Jaeger ingestion
- OTLP/gRPC/HTTP/Perfetto export

### High-leverage gaps (ordered by impact)

**A. CICDEnricher** — new enricher recognizing OTel CI/CD semantic conventions (`cicd.pipeline.*`, `cicd.pipeline.task.*`, `vcs.*`). Maps to the same workflow→job→step hierarchy as GHA but works for Jenkins, GitLab, Buildkite, Dagger, Tekton out of the box. This is the single highest-leverage addition.

**B. Richer GenericEnricher** — `pkg/enrichment/generic.go` currently only reads `otel.status_code` and `otel.span_kind`. Add:
- `http.method` / `http.route` → category "http"
- `db.system` / `db.statement` → category "database"
- `rpc.system` / `rpc.service` → category "rpc"
- `messaging.system` → category "messaging"
- `service.name` on root spans → display name

**C. Thread enricher through the call stack** — `DefaultEnricher()` is called in ~5 hardcoded locations. Should accept an `Enricher` parameter wired from `main.go`.

**D. Rule-based enrichment config** — `--enrichment=rules.yaml` for user-defined attribute-pattern → SpanHints mappings.

### Nice-to-haves (later)
- Bazel profile JSON → OTLP importer (unique differentiator, dogfooding)
- OTLP receiver mode (`--listen :4318`) for live span ingestion
- Span search/filter in TUI
- Generic trend analysis (currently GHA-specific)

### Explicitly not needed
- Web UI (TUI is the differentiator)
- Persistent storage (CLI tool viewing one trace at a time is fine)
- Service map / dependency graph (Jaeger does this well)

---

## 4. How to Pluginize Enrichments

### Recommended: Domain Enrichers + Rule Config + Library Composition

The enricher chain becomes domain-oriented:
```go
ChainEnricher(
  &GHAEnricher{},      // GitHub Actions (existing)
  &CICDEnricher{},     // OTel CI/CD semconv (Jenkins, GitLab, Buildkite, Dagger, Tekton)
  &BuildEnricher{},    // Gradle/Maven/Bazel build tasks (if needed beyond CICD)
  ruleEnricher,        // User-defined YAML rules
  &GenericEnricher{},  // Fallback: http/db/rpc/messaging semconv
)
```

**Why not alternatives:**
- Go plugins: fragile, no cross-compilation, dead with Bazel
- Starlark/Wasm: over-engineered for current stage
- OTel Collector processor pattern: massive dependency bloat, wrong abstraction

### Phase 1: Thread enricher (pure refactoring)

Change all `DefaultEnricher()` call sites to accept an `enrichment.Enricher` parameter. Wire from `main.go`. No behavior change.

**Files:**
- `pkg/tui/results/model.go` — model construction
- `pkg/tui/results/items.go` — item building
- `pkg/export/terminal/terminal.go` — terminal exporter
- `pkg/output/timeline.go` — timeline rendering
- `cmd/otel-analyzer/main.go` — construct and pass enricher

### Phase 2: CICDEnricher

New `pkg/enrichment/cicd.go`. Recognizes OTel CI/CD semantic conventions:
- `cicd.pipeline.name` present → category "workflow", IsRoot
- `cicd.pipeline.task.run.id` present → category "job"
- `cicd.pipeline.task.run.result` → outcome mapping
- `vcs.ref.head.name` → branch context
- Falls through to GenericEnricher if no CI/CD attributes found

### Phase 3: Rule-based enricher

New `pkg/enrichment/rules.go`. Loads YAML rules mapping attribute glob patterns to SpanHints:

```yaml
enrichers:
  - name: kubernetes
    match:
      attributes:
        k8s.deployment.name: "*"
    hints:
      category: "deployment"
      icon: "🚀"
      color: "blue"
      is_root: true

  - name: http-errors
    match:
      attributes:
        http.status_code: "5*"
    hints:
      outcome: "failure"
      color: "red"
```

Add `--enrichment=<file>` flag in `main.go`.

### Phase 4: Expand GenericEnricher

Add OTel semantic convention recognition (http, db, rpc, messaging). Improves baseline for all non-CI/CD traces.

### Phase 5 (if demand): Library approach

Users write a Go package implementing `Enricher` and build their own binary. Phase 1's refactoring makes this trivially composable.

---

## 5. Verification

After each phase:
- `bazel build //cmd/otel-analyzer` compiles
- Existing GHA trace analysis unchanged (GHAEnricher still first in chain)
- `otel-analyzer --trace=<otel-file>` shows enriched spans with appropriate categories
- Test with: `otelgen` synthetic traces, OTel demo app output, Jenkins/GitLab traces if available
- Rule-based enricher tested with example YAML + sample trace file
