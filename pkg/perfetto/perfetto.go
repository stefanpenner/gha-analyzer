package perfetto

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
	"github.com/stefanpenner/gha-analyzer/pkg/utils"
	"go.opentelemetry.io/otel/sdk/trace"
)

func WriteTrace(w io.Writer, urlResults []analyzer.URLResult, combined analyzer.CombinedMetrics, traceEvents []analyzer.TraceEvent, globalEarliestTime int64, perfettoFile string, openInPerfetto bool, spans []trace.ReadOnlySpan) error {
	traceTitle := fmt.Sprintf("GitHub Actions: Multi-URL Analysis (%d URLs)", len(urlResults))
	traceMetadata := []analyzer.TraceEvent{
		{
			Name: "process_name",
			Ph:   "M",
			Pid:  0,
			Args: map[string]interface{}{
				"name":       traceTitle,
				"url":        "https://perfetto.dev",
				"github_url": "https://github.com",
			},
		},
	}

	// Find the true earliest timestamp across all events
	trueEarliest := globalEarliestTime
	for _, s := range spans {
		ms := s.StartTime().UnixMilli()
		if ms < trueEarliest {
			trueEarliest = ms
		}
	}
	for _, res := range urlResults {
		if res.EarliestTime != 0 && res.EarliestTime < trueEarliest {
			trueEarliest = res.EarliestTime
		}
	}

	// Convert OTel spans to TraceEvents
	otelEvents := []analyzer.TraceEvent{}
	pidsSeen := make(map[int]bool)
	tidsSeen := make(map[int]bool)

	for _, s := range spans {
		// Only include relevant spans
		isGHA := false
		attrs := make(map[string]interface{})
		for _, attr := range s.Attributes() {
			val := attr.Value.AsInterface()
			// Basic sanitization for Perfetto JSON
			if str, ok := val.(string); ok {
				val = utils.StripANSI(str)
				if len(str) > 1000 {
					val = str[:1000] + "..."
				}
			}
			attrs[string(attr.Key)] = val
			if attr.Key == "github.url" {
				attrs["url"] = val
			}
			if attr.Key == "type" && (attr.Value.AsString() == "workflow" || attr.Value.AsString() == "job" || attr.Value.AsString() == "step" || attr.Value.AsString() == "marker") {
				isGHA = true
			}
		}
		if !isGHA {
			continue
		}

		ts := (s.StartTime().UnixMilli() - trueEarliest) * 1000
		dur := s.EndTime().Sub(s.StartTime()).Microseconds()
		if dur <= 0 && attrs["type"] != "marker" {
			dur = 1000 // Ensure at least 1ms for visibility
		}

		ph := "X"
		sScope := ""
		if attrs["type"] == "marker" {
			ph = "i"
			sScope = "p"
		}

		pid := 1
		tid := 1
		trackName := ""

		if attrs["type"] == "workflow" {
			if runID, ok := attrs["github.run_id"].(int64); ok {
				pid = int(runID % 2147483647)
				tid = 0
				trackName = utils.StripANSI(s.Name())
			}
		} else if attrs["type"] == "job" {
			if runID, ok := attrs["github.run_id"].(int64); ok {
				pid = int(runID % 2147483647)
			}
			if jobID, ok := attrs["github.job_id"].(int64); ok {
				tid = int(jobID % 2147483647)
				trackName = "Job: " + utils.StripANSI(s.Name())
			}
		} else if attrs["type"] == "step" {
			if runID, ok := attrs["github.run_id"].(int64); ok {
				pid = int(runID % 2147483647)
			}
			// Steps on the same track as their job
			// Since we don't have the job ID easily, use the parent SpanID
			parentID := s.Parent().SpanID()
			var tidVal uint32
			for i := 0; i < 8; i++ {
				tidVal = (tidVal << 8) | uint32(parentID[i])
			}
			tid = int(tidVal % 2147483647)
		} else if attrs["type"] == "marker" {
			pid = 999
			tid = 2 // Unified "GitHub PR Events" track
			if !pidsSeen[pid] {
				otelEvents = append(otelEvents, analyzer.TraceEvent{
					Name: "process_name", Ph: "M", Pid: pid,
					Args: map[string]interface{}{"name": "GitHub Events"},
				})
				pidsSeen[pid] = true
			}
			if !tidsSeen[tid] {
				otelEvents = append(otelEvents, analyzer.TraceEvent{
					Name: "thread_name", Ph: "M", Pid: pid, Tid: tid,
					Args: map[string]interface{}{"name": "GitHub PR Events"},
				})
				tidsSeen[tid] = true
			}
		}

		// Add metadata events for names if we haven't seen them
		if trackName != "" {
			if tid != 0 && !tidsSeen[tid] {
				otelEvents = append(otelEvents, analyzer.TraceEvent{
					Name: "thread_name", Ph: "M", Pid: pid, Tid: tid,
					Args: map[string]interface{}{"name": trackName},
				})
				tidsSeen[tid] = true
			} else if tid == 0 && !pidsSeen[pid] {
				otelEvents = append(otelEvents, analyzer.TraceEvent{
					Name: "process_name", Ph: "M", Pid: pid,
					Args: map[string]interface{}{"name": trackName},
				})
				pidsSeen[pid] = true
			}
		}

		otelEvents = append(otelEvents, analyzer.TraceEvent{
			Name: utils.StripANSI(s.Name()),
			Ph:   ph,
			Ts:   ts,
			Dur:  dur,
			Pid:  pid,
			Tid:  tid,
			Cat:  fmt.Sprintf("%v", attrs["type"]),
			Args: attrs,
			S:    sScope,
		})
	}

	allEvents := append(traceEvents, otelEvents...)

	renormalized := make([]analyzer.TraceEvent, 0, len(allEvents))
	for _, event := range allEvents {
		// Clean up legacy events too
		event.Name = utils.StripANSI(event.Name)
		if event.Ph == "i" && event.S == "" {
			event.S = "p"
		}
		if event.Args != nil {
			for k, v := range event.Args {
				if str, ok := v.(string); ok {
					event.Args[k] = utils.StripANSI(str)
				}
			}
		}

		// Only renormalize legacy events that have url_index
		isLegacy := false
		if event.Args != nil {
			if _, ok := event.Args["url_index"]; ok {
				isLegacy = true
			}
		}

		if isLegacy && event.Ts != 0 {
			eventURLIndex := 1
			if val, ok := event.Args["url_index"].(int); ok {
				eventURLIndex = val
			} else if valFloat, ok := event.Args["url_index"].(float64); ok {
				eventURLIndex = int(valFloat)
			}
			eventSource := ""
			if val, ok := event.Args["source_url"].(string); ok {
				eventSource = val
			}

			var urlResult *analyzer.URLResult
			for i := range urlResults {
				if urlResults[i].URLIndex == eventURLIndex-1 || urlResults[i].DisplayURL == eventSource {
					urlResult = &urlResults[i]
					break
				}
			}

			if urlResult != nil {
				absoluteTime := event.Ts/1000 + urlResult.EarliestTime
				event.Ts = (absoluteTime - trueEarliest) * 1000
			}
		}

		// Final check to prevent negative timestamps which Perfetto hates
		if event.Ts < 0 {
			event.Ts = 0
		}

		renormalized = append(renormalized, event)
	}
	analyzer.SortTraceEvents(renormalized)

	output := map[string]interface{}{
		"displayTimeUnit": "ms",
		"traceEvents":     append(traceMetadata, renormalized...),
		"otherData": map[string]interface{}{
			"trace_title":  traceTitle,
			"url_count":    len(urlResults),
			"total_runs":   combined.TotalRuns,
			"total_jobs":   combined.TotalJobs,
			"success_rate": fmt.Sprintf("%s%%", combined.SuccessRate),
			"total_events": len(renormalized),
			"urls":         buildTraceURLData(urlResults),
			"performance_analysis": map[string]interface{}{
				"slowest_jobs": buildSlowJobsForTrace(combined.JobTimeline),
			},
		},
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(perfettoFile, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n💾 Perfetto trace saved to: %s\n", perfettoFile)

	if openInPerfetto {
		return openTraceInPerfetto(w, perfettoFile)
	}
	return nil
}

func buildTraceURLData(results []analyzer.URLResult) []map[string]interface{} {
	data := make([]map[string]interface{}, 0, len(results))
	for i, result := range results {
		data = append(data, map[string]interface{}{
			"index":        i + 1,
			"owner":        result.Owner,
			"repo":         result.Repo,
			"type":         result.Type,
			"identifier":   result.Identifier,
			"display_name": result.DisplayName,
			"display_url":  result.DisplayURL,
			"total_runs":   result.Metrics.TotalRuns,
			"total_jobs":   result.Metrics.TotalJobs,
			"success_rate": result.Metrics.SuccessRate,
		})
	}
	return data
}

func buildSlowJobsForTrace(jobs []analyzer.CombinedTimelineJob) []map[string]interface{} {
	if len(jobs) == 0 {
		return nil
	}
	sorted := append([]analyzer.CombinedTimelineJob{}, jobs...)
	analyzer.SortCombinedJobsByDuration(sorted)
	if len(sorted) > 10 {
		sorted = sorted[:10]
	}
	output := []map[string]interface{}{}
	for _, job := range sorted {
		output = append(output, map[string]interface{}{
			"name":             job.Name,
			"duration_seconds": fmt.Sprintf("%.1f", float64(job.EndTime-job.StartTime)/1000),
			"url":              job.URL,
			"source_url":       job.SourceURL,
			"source_name":      job.SourceName,
		})
	}
	return output
}

func openTraceInPerfetto(w io.Writer, traceFile string) error {
	scriptName := "open_trace_in_ui"
	scriptURL := "https://raw.githubusercontent.com/google/perfetto/main/tools/open_trace_in_ui"
	scriptPath := filepath.Join(os.TempDir(), scriptName)

	if _, err := os.Stat(scriptPath); err != nil {
		fmt.Fprintln(w, "\n🚀 Opening trace in Perfetto UI...")
		fmt.Fprintln(w, "📥 Downloading open_trace_in_ui from Perfetto...")
		if err := exec.Command("curl", "-L", "-o", scriptPath, scriptURL).Run(); err != nil {
			return fmt.Errorf("failed to download %s: %w", scriptName, err)
		}
		if err := exec.Command("chmod", "+x", scriptPath).Run(); err != nil {
			return fmt.Errorf("failed to make %s executable: %w", scriptName, err)
		}
	} else {
		fmt.Fprintf(w, "\n📁 Using existing script: %s\n", scriptPath)
	}

	fmt.Fprintf(w, "🔗 Opening %s in Perfetto UI...\n", traceFile)
	cmd := exec.Command(scriptPath, traceFile)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(w, "❌ Failed to open trace in Perfetto: %v\n", err)
		fmt.Fprintln(w, "💡 You can manually open the trace at: https://ui.perfetto.dev")
		fmt.Fprintf(w, "   Then click \"Open trace file\" and select: %s\n", traceFile)
		return nil
	}
	fmt.Fprintln(w, "✅ Trace opened successfully in Perfetto UI!")
	return nil
}
