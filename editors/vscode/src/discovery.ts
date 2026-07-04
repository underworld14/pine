// Pure (no `vscode`) workspace discovery: find a `.pine/` directory by walking up
// from a starting directory. Mirrors the Go CLI's findPineDir (internal/cli/root.go).
import * as fs from 'node:fs';
import * as path from 'node:path';

/** Walk up from startDir looking for a `.pine/` directory. Returns its path or null. */
export function findPineDir(startDir: string): string | null {
  let dir = path.resolve(startDir);
  for (;;) {
    const p = path.join(dir, '.pine');
    try {
      if (fs.statSync(p).isDirectory()) {
        return p;
      }
    } catch {
      // not here; keep walking up
    }
    const parent = path.dirname(dir);
    if (parent === dir) {
      return null;
    }
    dir = parent;
  }
}

/** The workspace root that owns a `.pine` dir (the parent of `.pine`), or null. */
export function pineRootFrom(startDir: string): string | null {
  const p = findPineDir(startDir);
  return p ? path.dirname(p) : null;
}
