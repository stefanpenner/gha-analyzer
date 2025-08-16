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

    console.error(`🔗 Opening ${traceFile} in Perfetto UI...`);
    await new Promise((resolve, reject) => {
      const openScript = spawn(scriptPath, [traceFile], { stdio: 'inherit', env: { ...process.env, PYTHONIOENCODING: 'utf-8' } });
      openScript.on('close', (code) => (code === 0 ? resolve() : reject(new Error(`Failed to open trace in Perfetto (exit code: ${code})`))));
      openScript.on('error', (error) => reject(new Error(`Failed to execute script: ${error.message}`)));
    });

    console.error(`✅ Trace opened successfully in Perfetto UI!`);
  } catch (error) {
    console.error(`❌ Failed to open trace in Perfetto: ${error.message}`);
    console.error(`💡 You can manually open the trace at: https://ui.perfetto.dev`);
    console.error(`   Then click "Open trace file" and select: ${traceFile}`);
  }
}


