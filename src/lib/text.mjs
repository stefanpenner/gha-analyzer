// Text and formatting utilities for terminal output

export function makeClickableLink(url, text = null) {
  const displayText = text || url;
  return `\u001b]8;;${url}\u0007${displayText}\u001b]8;;\u0007`;
}

export function grayText(text) {
  return `\u001b[90m${text}\u001b[0m`;
}

export function greenText(text) {
  return `\u001b[32m${text}\u001b[0m`;
}

export function redText(text) {
  return `\u001b[31m${text}\u001b[0m`;
}

export function yellowText(text) {
  return `\u001b[33m${text}\u001b[0m`;
}

export function blueText(text) {
  return `\u001b[34m${text}\u001b[0m`;
}

export function humanizeTime(seconds) {
  if (seconds === 0) {
    return '0s';
  }
  if (seconds < 1) {
    return `${Math.round(seconds * 1000)}ms`;
  }

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  const parts = [];
  if (hours > 0) {
    parts.push(`${hours}h`);
  }
  if (minutes > 0) {
    parts.push(`${minutes}m`);
  }
  if (secs > 0 || parts.length === 0) {
    parts.push(`${secs}s`);
  }

  return parts.join(' ');
}

// Utility: visible length ignoring ANSI color codes and OSC 8 hyperlinks
export function visibleLength(text) {
  if (!text) return 0;
  // Strip SGR (color) sequences
  let stripped = text.replace(/\u001b\[[0-9;]*m/g, '');
  // Strip OSC 8 hyperlinks open and close sequences, but keep display text
  stripped = stripped
    .replace(/\u001b\]8;;[^\u0007]*\u0007/g, '') // open with URL
    .replace(/\u001b\]8;;\u0007/g, ''); // close
  return stripped.length;
}

export function padToWidth(content, width) {
  const visible = visibleLength(content);
  const padding = Math.max(0, width - visible);
  return content + ' '.repeat(padding);
}


