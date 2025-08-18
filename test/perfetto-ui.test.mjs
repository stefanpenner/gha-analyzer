import { test } from 'node:test';
import assert from 'node:assert/strict';
import { runPerfettoCheck } from '../scripts/perfetto-ui-check.mjs';

// Only run when explicitly enabled to avoid flakiness on CI/offline environments
const ENABLED = process.env.PERFETTO_E2E === '1';

const PERFETTO_URL = 'https://ui.perfetto.dev/';

async function hasNetwork() {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);
    const res = await fetch(PERFETTO_URL, { method: 'HEAD', signal: controller.signal });
    clearTimeout(timeout);
    return res.ok;
  } catch {
    return false;
  }
}

test('Perfetto UI opens without runtime errors', async (t) => {
  if (!ENABLED) return t.skip('Set PERFETTO_E2E=1 to run Puppeteer Perfetto UI test');
  if (!(await hasNetwork())) return t.skip('No network access - skipping Perfetto UI test');
  await runPerfettoCheck({ minimal: false, headful: false });
});


