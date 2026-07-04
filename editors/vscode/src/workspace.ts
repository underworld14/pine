// VS Code workspace glue over the pure discovery helpers.
import * as vscode from 'vscode';
import { pineRootFrom } from './discovery';

/** All workspace-folder roots that contain a `.pine` directory. */
export function pineRoots(): string[] {
  const folders = vscode.workspace.workspaceFolders ?? [];
  const roots: string[] = [];
  for (const f of folders) {
    const root = pineRootFrom(f.uri.fsPath);
    if (root && !roots.includes(root)) {
      roots.push(root);
    }
  }
  return roots;
}

/** Pick a single Pine root: the only one, or ask when there are several. */
export async function pickPineRoot(): Promise<string | null> {
  const roots = pineRoots();
  if (roots.length === 0) {
    return null;
  }
  if (roots.length === 1) {
    return roots[0];
  }
  const pick = await vscode.window.showQuickPick(roots, {
    placeHolder: 'Select a Pine workspace',
  });
  return pick ?? null;
}
