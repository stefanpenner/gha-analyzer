import { spawn } from 'child_process';
import os from 'os';
import path from 'path';
import fs from 'fs';

export async function openTraceInPerfetto(traceFile) {
  const scriptName = 'open_trace_in_ui';
  const scriptUrl = 'https://raw.githubusercontent.com/google/perfetto/main/tools/open_trace_in_ui';
  const tmpDir = os.tmpdir();
  const scriptPath = path.join(tmpDir, scriptName);

  try {
    console.error(`\n🚀 Opening trace in Perfetto UI...`);

    if (!fs.existsSync(scriptPath)) {
      console.error(`📥 Downloading ${scriptName} from Perfetto...`);

      await new Promise((resolve, reject) => {
        const curl = spawn('curl', ['-L', '-o', scriptPath, scriptUrl], { stdio: 'inherit' });
        curl.on('close', (code) => (code === 0 ? resolve() : reject(new Error(`Failed to download ${scriptName} (exit code: ${code})`))));
        curl.on('error', reject);
      });

      await new Promise((resolve, reject) => {
        const chmod = spawn('chmod', ['+x', scriptPath], { stdio: 'inherit' });
        chmod.on('close', (code) => (code === 0 ? resolve() : reject(new Error(`Failed to make ${scriptName} executable (exit code: ${code})`))));
        chmod.on('error', reject);
      });
    } else {
      console.error(`📁 Using existing script: ${scriptPath}`);
    }

    console.error(`🔗 Launching Perfetto UI for ${traceFile}...`);
    try {
      const child = spawn(scriptPath, [traceFile], { stdio: 'ignore', env: { ...process.env, PYTHONIOENCODING: 'utf-8' } });
      console.error(`✅ Perfetto UI launch initiated.`);
      await new Promise((r) => setTimeout(r, 8000));
    } catch (e) {
      console.error(`⚠️  Launch script failed (${e.message}). Falling back to opening Perfetto UI site...`);
      try {
        const openCmd = process.platform === 'darwin' ? 'open' : process.platform === 'win32' ? 'start' : 'xdg-open';
        spawn(openCmd, ['https://ui.perfetto.dev'], { stdio: 'ignore', shell: true, detached: true }).unref();
      } catch {
        // ignore
      }
    }

    // Note: Do not run headless validation here; keep validation limited to tests and the standalone script.
  } catch (error) {
    console.error(`❌ Failed to open trace in Perfetto: ${error.message}`);
    console.error(`💡 You can manually open the trace at: https://ui.perfetto.dev`);
    console.error(`   Then click "Open trace file" and select: ${traceFile}`);
  }
}


