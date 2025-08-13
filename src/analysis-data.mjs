export class AnalysisData {
  constructor() {
    this.allTraceEvents = [];
    this.allJobStartTimes = [];
    this.allJobEndTimes = [];
    this.allMetrics = AnalysisData.initializeMetrics();
    this.urlResults = [];
    this.globalEarliestTime = Infinity;
    this.globalLatestTime = 0;
    this.totalRuns = 0;
  }

  static initializeMetrics() {
    return {
      totalRuns: 0,
      successfulRuns: 0,
      failedRuns: 0,
      totalJobs: 0,
      failedJobs: 0,
      totalSteps: 0,
      failedSteps: 0,
      jobDurations: [],
      jobNames: [],
      jobUrls: [],
      stepDurations: [],
      runnerTypes: new Set(),
      totalDuration: 0,
      longestJob: { name: '', duration: 0 },
      shortestJob: { name: '', duration: Infinity },
      jobTimeline: []
    };
  }

  // Methods to update the data
  addTraceEvents(events) {
    this.allTraceEvents.push(...events);
  }

  addJobStartTimes(times) {
    this.allJobStartTimes.push(...times);
  }

  addJobEndTimes(times) {
    this.allJobEndTimes.push(...times);
  }

  addUrlResult(result) {
    this.urlResults.push(result);
  }

  updateGlobalTimeRange(earliestTime, latestTime) {
    if (earliestTime < this.globalEarliestTime) {
      this.globalEarliestTime = earliestTime;
    }
    if (latestTime > this.globalLatestTime) {
      this.globalLatestTime = latestTime;
    }
  }

  incrementTotalRuns(count) {
    this.totalRuns += count;
  }

  // Getters for accessing the data
  get traceEvents() {
    return this.allTraceEvents;
  }

  get jobStartTimes() {
    return this.allJobStartTimes;
  }

  get jobEndTimes() {
    return this.allJobEndTimes;
  }

  get metrics() {
    return this.allMetrics;
  }

  get results() {
    return this.urlResults;
  }

  get earliestTime() {
    return this.globalEarliestTime;
  }

  get latestTime() {
    return this.globalLatestTime;
  }

  get runsCount() {
    return this.totalRuns;
  }

  get urlCount() {
    return this.urlResults.length;
  }

  // Method to check if we have any data
  hasData() {
    return this.urlResults.length > 0;
  }

  // Method to reset all data (useful for testing or reusing the instance)
  reset() {
    this.allTraceEvents = [];
    this.allJobStartTimes = [];
    this.allJobEndTimes = [];
    this.allMetrics = AnalysisData.initializeMetrics();
    this.urlResults = [];
    this.globalEarliestTime = Infinity;
    this.globalLatestTime = 0;
    this.totalRuns = 0;
  }
}
