package analyzer

import "sort"

func AddThreadMetadata(events *[]TraceEvent, processID, threadID int, name string, sortIndex *int) {
	*events = append(*events, TraceEvent{
		Name: "thread_name",
		Ph:   "M",
		Pid:  processID,
		Tid:  threadID,
		Args: map[string]interface{}{"name": name},
	})

	if sortIndex != nil {
		*events = append(*events, TraceEvent{
			Name: "thread_sort_index",
			Ph:   "M",
			Pid:  processID,
			Tid:  threadID,
			Args: map[string]interface{}{"sort_index": *sortIndex},
		})
	}
}

func GenerateConcurrencyCounters(jobStartTimes, jobEndTimes []JobEvent, events *[]TraceEvent, earliestTime int64) {
	if len(jobStartTimes) == 0 {
		return
	}
	all := append([]JobEvent{}, jobStartTimes...)
	all = append(all, jobEndTimes...)
	SortJobEvents(all)

	current := 0
	metricsProcessID := 999
	counterThreadID := 1

	*events = append(*events, TraceEvent{
		Name: "process_name",
		Ph:   "M",
		Pid:  metricsProcessID,
		Args: map[string]interface{}{"name": "GitHub Events"},
	})
	AddThreadMetadata(events, metricsProcessID, counterThreadID, "Job Concurrency", intPtr(0))

	for _, event := range all {
		if event.Type == "start" {
			current++
		} else {
			current--
		}
		normalizedTs := (event.Ts - earliestTime) * 1000
		*events = append(*events, TraceEvent{
			Name: "Concurrent Jobs",
			Ph:   "C",
			Ts:   normalizedTs,
			Pid:  metricsProcessID,
			Tid:  counterThreadID,
			Args: map[string]interface{}{"Concurrent Jobs": current},
		})
	}
}

func SortTraceEvents(events []TraceEvent) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Ts < events[j].Ts
	})
}

func intPtr(value int) *int {
	return &value
}
