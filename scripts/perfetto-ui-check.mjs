#!/usr/bin/env node

// Standalone Perfetto UI sanity check using Puppeteer.
// Usage examples:
//   node scripts/perfetto-ui-check.mjs
//   node scripts/perfetto-ui-check.mjs --minimal
//   node scripts/perfetto-ui-check.mjs --upload ./trace.json --headful
// Exit code 0 when OK, 1 when the page surfaces runtime errors.

import os from 'node:os';
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';

const PERFETTO_URL = 'https://ui.perfetto.dev/';

function parseArgs(argv) {
  const args = { upload: null, minimal: false, headful: false, timeout: 45000 };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--minimal') args.minimal = true;
    else if (a === '--headful') args.headful = true;
    else if (a === '--timeout' && argv[i + 1]) { args.timeout = Number(argv[++i]) || args.timeout; }
    else if ((a === '--upload' || a === '-u') && argv[i + 1]) { args.upload = argv[++i]; }
  }
  return args;
}

async function hasNetwork() {
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 5000);
    const res = await fetch(PERFETTO_URL, { method: 'HEAD', signal: controller.signal });
    clearTimeout(timer);
    return res.ok;
  } catch {
    return false;
  }
}

function createMinimalTraceFile() {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'gha-analyzer-'));
  const file = path.join(tmp, 'trace-minimal.json');
  const content = {
    displayTimeUnit: 'ms',
    traceEvents: [
      { name: 'process_name', ph: 'M', pid: 1, args: { name: 'Test Process' } },
      { name: 'thread_name', ph: 'M', pid: 1, tid: 1, args: { name: 'Main Thread' } },
      { name: 'thread_sort_index', ph: 'M', pid: 1, tid: 1, args: { sort_index: 0 } },
      { name: 'Warmup', ph: 'X', pid: 1, tid: 1, ts: 1000, dur: 500000 }
    ]
  };
  fs.writeFileSync(file, JSON.stringify(content));
  return file;
}

export async function runPerfettoCheck(options = {}) {
  const opts = { upload: null, minimal: false, headful: false, timeout: 45000, ...options };


  const puppeteer = await import('puppeteer');
  let fileServer = null;
  let fileUrl = null;
  if (opts.minimal || opts.upload) {
    // Serve the trace over HTTP to avoid file picker flakiness; we'll allow mixed content via flags
    const file = opts.upload || createMinimalTraceFile();
    const buf = fs.readFileSync(file);
    fileServer = http.createServer((req, res) => {
      res.setHeader('Access-Control-Allow-Origin', '*');
      res.setHeader('Access-Control-Allow-Headers', '*');
      res.setHeader('Access-Control-Allow-Methods', 'GET, OPTIONS');
      if (req.method === 'OPTIONS') { res.writeHead(204); return res.end(); }
      if (req.url && req.url.startsWith('/trace')) {
        res.writeHead(200, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
        res.end(buf);
      } else { res.writeHead(404); res.end(); }
    });
    await new Promise((resolve) => fileServer.listen(0, '127.0.0.1', resolve));
    const port = fileServer.address().port;
    fileUrl = `http://127.0.0.1:${port}/trace`;
  }

  const launchArgs = [];
  if (process.env.CI) launchArgs.push('--no-sandbox', '--disable-setuid-sandbox');
  if (fileUrl) launchArgs.push('--allow-running-insecure-content', `--unsafely-treat-insecure-origin-as-secure=${new URL(fileUrl).origin}`);

  const browser = await puppeteer.default.launch({
    headless: opts.headful ? false : 'new',
    args: launchArgs,
    protocolTimeout: opts.timeout + 20000
  });

  try {
    const page = await browser.newPage();
    const errors = [];
    const ignorableHosts = ['googletagmanager.com', 'accounts.google.com'];
    const ignorableUrls = ['http://127.0.0.1:9001/status'];
    page.on('pageerror', (err) => errors.push(new Error('pageerror: ' + err.message)));
    page.on('requestfailed', (req) => {
      const url = req.url();
      if (ignorableUrls.some(u => url.startsWith(u))) return;
      if (ignorableHosts.some(h => url.includes(h))) return;
      errors.push(new Error(`requestfailed: ${url} ${req.failure()?.errorText || ''}`));
    });
    page.on('console', (msg) => {
      if (msg.type() !== 'error') return;
      const text = msg.text();
      if (ignorableUrls.some(u => text.includes(u))) return;
      if (ignorableHosts.some(h => text.includes(h))) return;
      if (/trace|perfetto|upload|json/i.test(text)) {
        errors.push(new Error('console.error: ' + text));
      }
    });

    // Always land on base UI first
    await page.goto(PERFETTO_URL, { waitUntil: 'domcontentloaded', timeout: opts.timeout });
    await page.title();

    // If we have a served file URL, try URL-param route first with mixed-content allowances
    if (fileUrl) {
      const target = `${PERFETTO_URL}#!/?url=${encodeURIComponent(fileUrl)}`;
      try {
        await page.goto(target, { waitUntil: 'load', timeout: opts.timeout });
        await page.waitForNetworkIdle({ idleTime: 1500, timeout: 15000 }).catch(() => {});
      } catch {}
    }

    if (opts.minimal || opts.upload) {
      const file = opts.upload || createMinimalTraceFile();
      console.log('[perfetto-ui-check] uploading trace:', file);

      // Try piercing selector to set files directly inside shadow DOM
      let uploaded = false;
      try {
        await page.setInputFiles('pierce/input[type="file"]', file);
        try { await page.$eval('pierce/input[type="file"]', (el) => el.dispatchEvent(new Event('change', { bubbles: true }))); } catch {}
        uploaded = true;
        console.log('[perfetto-ui-check] upload via pierce selector input[type=file]');
      } catch {}

      // Native file chooser next
      try {
        if (!uploaded) {
          await page.keyboard.press('KeyO');
          const chooser = await page.waitForFileChooser({ timeout: 2000 }).catch(() => null);
          if (chooser) { await chooser.accept([file]); uploaded = true; console.log('[perfetto-ui-check] upload via native file chooser'); }
        }
      } catch {}

      // Deep input fallback
      if (!uploaded) {
        for (let i = 0; i < 10 && !uploaded; i++) {
          const inputDeep = await page.evaluateHandle(() => {
            const seen = new Set();
            function search(root) {
              if (!root || seen.has(root)) return null;
              seen.add(root);
              const q = root.querySelectorAll ? root.querySelectorAll('input[type="file"]') : [];
              if (q && q.length > 0) return q[0];
              const nodes = root.querySelectorAll ? root.querySelectorAll('*') : [];
              for (const el of nodes) { if (el.shadowRoot) { const f = search(el.shadowRoot); if (f) return f; } }
              return null;
            }
            return search(document);
          });
          const inputEl = inputDeep.asElement();
          if (inputEl) {
            try {
              if (typeof inputEl.setInputFiles === 'function') {
                await inputEl.setInputFiles([file]);
              } else if (typeof page.setInputFiles === 'function') {
                await page.setInputFiles('input[type="file"]', file);
              }
              try { await inputEl.evaluate((el) => el.dispatchEvent(new Event('change', { bubbles: true }))); } catch {}
              uploaded = true;
              console.log('[perfetto-ui-check] upload via deep input[type=file]');
            } catch {}
          }
          if (!uploaded) await new Promise(r => setTimeout(r, 500));
        }
      }

      // Drag & drop fallback
      if (!uploaded) {
        try {
          const b64 = fs.readFileSync(file, { encoding: 'base64' });
          const didDrop = await page.evaluate((b64, filename) => {
            function b64ToUint8(b) { const raw = atob(b); const arr = new Uint8Array(raw.length); for (let i=0;i<raw.length;i++) arr[i]=raw.charCodeAt(i); return arr; }
            const blob = new Blob([b64ToUint8(b64)], { type: 'application/json' });
            const fileObj = new File([blob], filename, { type: 'application/json' });
            const dt = new DataTransfer(); dt.items.add(fileObj);
            const target = (() => {
              const all = [];
              const seen = new Set();
              function collect(root) { if (!root || seen.has(root)) return; seen.add(root); const nodes = root.querySelectorAll ? root.querySelectorAll('*') : []; for (const el of nodes) { all.push(el); if (el.shadowRoot) collect(el.shadowRoot); } }
              collect(document);
              const drop = all.find(el => el.tagName && el.tagName.toLowerCase() === 'perfetto-file-drop');
              return drop || document.body || document.documentElement;
            })();
            let ok = false;
            for (const evName of ['dragenter','dragover','drop']) { const ev = new DragEvent(evName, { bubbles: true, cancelable: true, dataTransfer: dt }); ok = target.dispatchEvent(ev) || ok; }
            return ok;
          }, b64, path.basename(file));
          if (didDrop) { uploaded = true; console.log('[perfetto-ui-check] upload via drag-and-drop'); }
        } catch {}
      }

      if (!uploaded) {
        throw new Error('Unable to upload trace to Perfetto UI');
      }

      // If Perfetto shows a modal dialog with errors (e.g., Failed to fetch), try closing it and retrying
      const modalText = await page.evaluate(() => {
        const seen = new Set();
        function collect(root, acc=[]) {
          if (!root || seen.has(root)) return acc;
          seen.add(root);
          const nodes = [root, ...(root.querySelectorAll ? Array.from(root.querySelectorAll('*')) : [])];
          for (const el of nodes) {
            if (el.shadowRoot) collect(el.shadowRoot, acc);
            const txt = el.innerText || '';
            if (/Could not load local trace/i.test(txt) || /Failed to fetch/i.test(txt)) acc.push({ elTxt: txt });
          }
          return acc;
        }
        const found = collect(document);
        return found.map(o => o.elTxt).join('\n');
      });
      if (modalText && /Failed to fetch|Could not load local trace/i.test(modalText)) {
        // Try to close modal
        await page.evaluate(() => {
          const seen = new Set();
          function collect(root) {
            if (!root || seen.has(root)) return [];
            seen.add(root);
            const all = [];
            const nodes = root.querySelectorAll ? root.querySelectorAll('*') : [];
            for (const el of nodes) {
              if (el.shadowRoot) all.push(...collect(el.shadowRoot));
              const txt = el.innerText || '';
              if (/close/i.test(txt) && (el.tagName === 'BUTTON' || el.getAttribute?.('role') === 'button')) all.push(el);
            }
            return all;
          }
          const buttons = collect(document);
          buttons.forEach(b => b.click());
        });
        // Retry deep input after closing
        try {
          const inputDeep = await page.evaluateHandle(() => {
            const seen = new Set();
            function search(root) {
              if (!root || seen.has(root)) return null;
              seen.add(root);
              const q = root.querySelectorAll ? root.querySelectorAll('input[type="file"]') : [];
              if (q && q.length > 0) return q[0];
              const nodes = root.querySelectorAll ? root.querySelectorAll('*') : [];
              for (const el of nodes) { if (el.shadowRoot) { const f = search(el.shadowRoot); if (f) return f; } }
              return null;
            }
            return search(document);
          });
          const inputEl = inputDeep.asElement();
          if (inputEl) {
            if (typeof inputEl.setInputFiles === 'function') {
              await inputEl.setInputFiles([file]);
            } else if (typeof page.setInputFiles === 'function') {
              await page.setInputFiles('input[type="file"]', file);
            }
            try { await inputEl.evaluate((el) => el.dispatchEvent(new Event('change', { bubbles: true }))); } catch {}
            console.log('[perfetto-ui-check] retried upload after closing modal');
          }
        } catch {}
      }
    }

    // Wait for success signals: the main canvas and the omnibox should appear
    async function queryDeep(selector) {
      return await page.evaluate((sel) => {
        const seen = new Set();
        function search(root) {
          if (!root || seen.has(root)) return null;
          seen.add(root);
          const el = root.querySelector ? root.querySelector(sel) : null;
          if (el) return el;
          const nodes = root.querySelectorAll ? root.querySelectorAll('*') : [];
          for (const n of nodes) {
            if (n.shadowRoot) {
              const found = search(n.shadowRoot);
              if (found) return found;
            }
          }
          return null;
        }
        return !!search(document);
      }, selector);
    }

    let foundSuccess = false;
    for (let i = 0; i < 30; i++) { // ~15s total
      const hasCanvas = await queryDeep('canvas');
      const hasOmnibox = await queryDeep('input[placeholder*="Search"], input[aria-label*="Search"], .omnibox, perfetto-search, perfetto-omnibox');
      if (hasCanvas && hasOmnibox) { foundSuccess = true; break; }
      await new Promise(r => setTimeout(r, 500));
    }

    // Detect Perfetto crash dialog and extract error details
    const crashDetails = await page.evaluate(() => {
      function collect(root, seen = new Set(), acc = []) {
        if (!root || seen.has(root)) return acc;
        seen.add(root);
        const nodes = [root, ...(root.querySelectorAll ? Array.from(root.querySelectorAll('*')) : [])];
        for (const el of nodes) {
          if (el.shadowRoot) collect(el.shadowRoot, seen, acc);
          const text = el.innerText || '';
          if (/Oops, something went wrong\./i.test(text) || /JSON trace file is incomplete/i.test(text)) {
            const msg = text.trim();
            const code = el.querySelector ? (el.querySelector('code, pre')?.innerText || '') : '';
            acc.push({ msg, code: code.trim() });
          }
        }
        return acc;
      }
      const found = collect(document);
      if (found.length === 0) return null;
      // Return the most detailed one
      return found.sort((a, b) => b.msg.length - a.msg.length)[0];
    });
    if (crashDetails) {
      throw new Error(`Perfetto UI reported a crash: ${crashDetails.msg}${crashDetails.code ? `\nDetails: ${crashDetails.code}` : ''}`);
    }

    if (!foundSuccess) {
      throw new Error('Perfetto UI did not show canvas + omnibox within timeout (trace likely failed to load).');
    }

    if (errors.length > 0) {
      throw new Error(errors.map(e => e.message).join('\n'));
    }
  } finally {
    await browser.close();
  }
}

async function main() {
  const opts = parseArgs(process.argv);
  console.log('[perfetto-ui-check] options:', opts);
  try {
    await runPerfettoCheck(opts);
    console.log('[perfetto-ui-check] OK (no errors detected)');
  } catch (e) {
    console.error('[perfetto-ui-check] Failed:', e);
    process.exit(1);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}


