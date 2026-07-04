// Pure (no `vscode`) helpers for finding an executable `pine` on PATH.
import * as fs from 'node:fs';
import * as path from 'node:path';
import { spawnSync } from 'node:child_process';

/** True if `p` is a file that is executable (X_OK on POSIX; existence on Windows). */
export function isExecutable(p: string): boolean {
  try {
    const st = fs.statSync(p);
    if (!st.isFile()) {
      return false;
    }
    if (process.platform !== 'win32') {
      fs.accessSync(p, fs.constants.X_OK);
    }
    return true;
  } catch {
    return false;
  }
}

/**
 * Search PATH for `bin` (appending `.exe` on Windows). Returns the first entry
 * that is executable and responds to `--version`, or null. A custom `env` and
 * `runVersionCheck` are injectable for testing.
 */
export function findOnPath(
  bin: string,
  env: NodeJS.ProcessEnv = process.env,
  runVersionCheck: (p: string) => boolean = defaultVersionCheck,
): string | null {
  const exe = process.platform === 'win32' ? `${bin}.exe` : bin;
  const dirs = (env.PATH || '').split(path.delimiter);
  for (const d of dirs) {
    if (!d) {
      continue;
    }
    const full = path.join(d, exe);
    if (isExecutable(full) && runVersionCheck(full)) {
      return full;
    }
  }
  return null;
}

function defaultVersionCheck(p: string): boolean {
  const r = spawnSync(p, ['--version'], { timeout: 3000 });
  return r.status === 0;
}
