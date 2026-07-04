// Launches a headless VS Code with the extension loaded and runs the integration
// suite (test/suite). Prepares a temporary workspace with an initialized `.pine`
// so the extension activates and can open the board. Requires `pine` on PATH or
// PINE_BIN pointing at a built binary.
import * as path from 'node:path';
import * as fs from 'node:fs';
import * as os from 'node:os';
import { spawnSync } from 'node:child_process';
import { runTests } from '@vscode/test-electron';

async function main(): Promise<void> {
  const extensionDevelopmentPath = path.resolve(__dirname, '../../');
  const extensionTestsPath = path.resolve(__dirname, './suite/index');

  const pine = process.env.PINE_BIN || 'pine';
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), 'pine-it-'));
  const init = spawnSync(pine, ['init'], { cwd: workspace });
  if (init.status !== 0) {
    // eslint-disable-next-line no-console
    console.error('pine init failed; is `pine` on PATH or PINE_BIN set?', init.stderr?.toString());
    process.exit(1);
  }

  try {
    await runTests({
      extensionDevelopmentPath,
      extensionTestsPath,
      launchArgs: [workspace, '--disable-extensions'],
      extensionTestsEnv: { PINE_BIN: process.env.PINE_BIN ?? '' },
    });
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error('Integration tests failed', err);
    process.exitCode = 1;
  } finally {
    fs.rmSync(workspace, { recursive: true, force: true });
  }
}

void main();
