// Integration tests that run inside a real VS Code host (via @vscode/test-electron).
import * as assert from 'node:assert';
import * as vscode from 'vscode';

describe('Pine extension (integration)', function () {
  this.timeout(60000);

  before(async () => {
    const cfg = vscode.workspace.getConfiguration('pine');
    // Never hit the network during tests: use the provided binary, no download.
    if (process.env.PINE_BIN) {
      await cfg.update('path', process.env.PINE_BIN, vscode.ConfigurationTarget.Global);
    }
    await cfg.update('autoDownload', false, vscode.ConfigurationTarget.Global);
  });

  it('activates and registers its commands', async () => {
    const ext = vscode.extensions.getExtension('underworld14.pine-vscode');
    assert.ok(ext, 'extension should be found');
    await ext.activate();
    const cmds = await vscode.commands.getCommands(true);
    for (const c of ['pine.openBoard', 'pine.createBug', 'pine.createFeature']) {
      assert.ok(cmds.includes(c), `command ${c} should be registered`);
    }
  });

  it('opens the board as a "Pine Board" webview tab', async function () {
    if (!process.env.PINE_BIN) {
      this.skip();
      return;
    }
    await vscode.commands.executeCommand('pine.openBoard');
    const appeared = await waitFor(() => tabTitles().includes('Pine Board'), 20000);
    assert.ok(appeared, `expected a 'Pine Board' tab; got: ${tabTitles().join(', ') || '(none)'}`);
  });
});

function tabTitles(): string[] {
  const titles: string[] = [];
  for (const group of vscode.window.tabGroups.all) {
    for (const tab of group.tabs) {
      titles.push(tab.label);
    }
  }
  return titles;
}

async function waitFor(pred: () => boolean, timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (pred()) {
      return true;
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  return pred();
}
