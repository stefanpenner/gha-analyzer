package perfetto

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stefanpenner/gha-analyzer/pkg/analyzer"
)

func WriteTrace(w io.Writer, urlResults []analyzer.URLResult, combined analyzer.CombinedMetrics, traceEvents []analyzer.TraceEvent, globalEarliestTime int64, perfettoFile string, openInPerfetto bool) error {
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

	renormalized := make([]analyzer.TraceEvent, 0, len(traceEvents))
	for _, event := range traceEvents {
		if event.Ts != 0 {
			eventURLIndex := 1
			if event.Args != nil {
				if val, ok := event.Args["url_index"].(int); ok {
					eventURLIndex = val
				} else if valFloat, ok := event.Args["url_index"].(float64); ok {
					eventURLIndex = int(valFloat)
				}
			}
			eventSource := ""
			if event.Args != nil {
				if val, ok := event.Args["source_url"].(string); ok {
					eventSource = val
				}
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
				event.Ts = (absoluteTime - globalEarliestTime) * 1000
			}
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
			"total_events": len(traceEvents),
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
