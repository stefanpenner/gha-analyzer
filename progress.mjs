

export default class ProgressBar {
  constructor(totalUrls, totalRuns) {
    this.totalUrls = totalUrls;
    this.totalRuns = totalRuns;
    this.currentUrl = 0;
    this.currentRun = 0;
    this.currentUrlRuns = 0;
    this.isProcessing = false;
  }

  startUrl(urlIndex, url) {
    this.currentUrl = urlIndex + 1;
    this.currentRun = 0;
    this.isProcessing = true;
    this.updateDisplay();
  }

  setUrlRuns(runCount) {
    this.currentUrlRuns = runCount;
    this.updateDisplay();
  }

  processRun() {
    this.currentRun++;
    this.updateDisplay();
  }

  updateDisplay() {
    if (!this.isProcessing) return;
    
    const urlProgress = `${this.currentUrl}/${this.totalUrls}`;
    const runProgress = this.currentUrlRuns > 0 ? ` (${this.currentRun}/${this.currentUrlRuns} runs)` : '';
    const progressText = `Processing URL ${urlProgress}${runProgress}...`;
    
    process.stderr.write(`\r${progressText}`);
  }

  finish() {
    this.isProcessing = false;
    process.stderr.write('\r' + ' '.repeat(50) + '\r');
  }
}
