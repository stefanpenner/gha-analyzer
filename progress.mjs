/**
 * Enhanced Progress Indicator with Visual Progress Bars and Real-time Updates
 * Provides a much more engaging and informative progress display
 */

export default class ProgressBar {
  constructor(totalUrls, totalRuns) {
    this.totalUrls = totalUrls;
    this.totalRuns = totalRuns;
    this.currentUrl = 0;
    this.currentRun = 0;
    this.currentUrlRuns = 0;
    this.isProcessing = false;
    this.startTime = Date.now();
    this.spinnerFrames = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];
    this.spinnerIndex = 0;
    this.lastUpdate = 0;
    this.updateInterval = 50; // Update every 50ms for smoother animation
    this.timer = null;
    this.statusMessage = '';
  }

  startUrl(urlIndex, url) {
    this.currentUrl = urlIndex + 1;
    this.currentRun = 0;
    this.isProcessing = true;
    this.urlStartTime = Date.now();
    this.statusMessage = 'Contacting GitHub…';
    this.startTicker();
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

  setStatus(message) {
    this.statusMessage = message || '';
    this.updateDisplay();
  }

  startTicker() {
    if (this.timer) clearInterval(this.timer);
    this.timer = setInterval(() => {
      if (!this.isProcessing) return;
      this.updateDisplay();
    }, this.updateInterval);
  }

  updateDisplay() {
    if (!this.isProcessing) return;
    
    // Update spinner
    this.spinnerIndex = (this.spinnerIndex + 1) % this.spinnerFrames.length;
    
    const spinner = this.spinnerFrames[this.spinnerIndex];
    const urlProgress = this.renderUrlProgress();
    const runProgress = this.renderRunProgress();
    const timing = this.renderTiming();
    
    // Single-line progress that updates in-place
    const label = this.statusMessage ? this.statusMessage : 'Processing';
    const progressLine = `${spinner} ${label}: ${urlProgress}${runProgress} | ${timing}`;
    
    // Clear line and write new progress
    process.stderr.write(`\r\x1b[K${progressLine}`);
  }

  renderUrlProgress() {
    const width = 20;
    if (this.currentUrlRuns > 0) {
      const urlPercent = ((this.currentRun / this.currentUrlRuns) * 100).toFixed(1);
      const urlBar = this.createProgressBar(this.currentRun, this.currentUrlRuns, width);
      return `URL ${this.currentUrl}/${this.totalUrls} ${urlBar} ${urlPercent}%`;
    } else {
      const urlBar = this.createProgressBar(0, 1, width);
      return `URL ${this.currentUrl}/${this.totalUrls} ${urlBar} 0.0%`;
    }
  }

  renderRunProgress() {
    if (this.currentUrlRuns === 0) return ' | Discovering runs…';
    
    // Simplified run progress - just show count without percentage bars
    return ` | Runs ${this.currentRun}/${this.currentUrlRuns}`;
  }

  renderTiming() {
    const elapsed = Date.now() - this.startTime;
    const elapsedStr = this.formatDuration(elapsed);
    return `⏱️ ${elapsedStr}`;
  }

  createProgressBar(current, total, width) {
    if (total === 0) return '█'.repeat(width);
    
    const filled = Math.round((current / total) * width);
    const empty = width - filled;
    
    const filledBar = '█'.repeat(filled);
    const emptyBar = '░'.repeat(empty);
    
    return filledBar + emptyBar;
  }

  formatDuration(ms) {
    if (ms < 1000) return `${ms}ms`;
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
    if (ms < 3600000) return `${(ms / 60000).toFixed(1)}m`;
    return `${(ms / 3600000).toFixed(1)}h`;
  }

  finish() {
    this.isProcessing = false;
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
    const totalTime = Date.now() - this.startTime;
    const totalTimeStr = this.formatDuration(totalTime);
    
    // Show final progress state briefly before clearing
    setTimeout(() => {
      // Clear the progress line and write completion
      process.stderr.write('\r\x1b[K');
      
      const completion = [
        `✅ Analysis Complete!`,
        `╭${'─'.repeat(78)}╮`,
        `│ 🎯 Processed ${this.totalUrls} URLs in ${totalTimeStr}`,
        `│ 📈 Total workflow runs analyzed: ${this.totalRuns}`,
        `│ 🚀 Average time per URL: ${this.formatDuration(totalTime / this.totalUrls)}`,
        `╰${'─'.repeat(78)}╯`
      ].join('\n');
      
      process.stderr.write(completion + '\n');
    }, 100); // Small delay to show final progress state
  }
}
