// Pure (no `vscode`) wrapper around `pine create --json`. Shelling out to the CLI
// (rather than POSTing to the server) means ticket creation works whether or not
// the board/server is currently open; a running server's watcher picks up the new
// file and pushes it to the board live.
import { spawn } from 'node:child_process';

export interface CreateResult {
  id: string;
  title: string;
}

/** Run `pine create -C <root> --type <type> --title <title> --json`. */
export function pineCreate(
  binaryPath: string,
  root: string,
  type: string,
  title: string,
): Promise<CreateResult> {
  return new Promise((resolve, reject) => {
    const child = spawn(
      binaryPath,
      ['create', '-C', root, '--type', type, '--title', title, '--json'],
      { cwd: root },
    );
    let out = '';
    let err = '';
    child.stdout.on('data', (d) => (out += d.toString()));
    child.stderr.on('data', (d) => (err += d.toString()));
    child.on('error', reject);
    child.on('exit', (code) => {
      if (code !== 0) {
        reject(new Error(err.trim() || `pine create exited with code ${code}`));
        return;
      }
      try {
        const obj = JSON.parse(out) as { id?: string; title?: string };
        if (!obj.id) {
          throw new Error('response had no id');
        }
        resolve({ id: obj.id, title: obj.title ?? title });
      } catch {
        reject(new Error(`could not parse pine create output: ${out.trim()}`));
      }
    });
  });
}
