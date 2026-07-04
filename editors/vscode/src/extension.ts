// Extension entry point: wires commands, the status-bar item, the Activity Bar
// view, the `pine.hasWorkspace` context key, and live `.pine` detection. All the
// real work lives in the focused modules this file composes.
import * as vscode from 'vscode';
import { pineRoots, pickPineRoot } from './workspace';
import { resolvePineBinary } from './binary';
import { ServerManager } from './server';
import { BoardPanel } from './board';
import { pineCreate } from './create';

let serverManager: ServerManager | undefined;
let output: vscode.OutputChannel;
let statusItem: vscode.StatusBarItem;
let extContext: vscode.ExtensionContext;
let remoteWarned = false;

export function activate(context: vscode.ExtensionContext): void {
  extContext = context;
  output = vscode.window.createOutputChannel('Pine');
  context.subscriptions.push(output);

  statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  statusItem.text = '$(list-tree) Pine';
  statusItem.tooltip = 'Open the Pine board';
  statusItem.command = 'pine.openBoard';
  context.subscriptions.push(statusItem);

  const refresh = (): void => {
    const has = pineRoots().length > 0;
    void vscode.commands.executeCommand('setContext', 'pine.hasWorkspace', has);
    if (has) {
      statusItem.show();
    } else {
      statusItem.hide();
    }
  };
  refresh();
  context.subscriptions.push(vscode.workspace.onDidChangeWorkspaceFolders(refresh));

  // Activity Bar view — an empty tree so its welcome content (the Open Board
  // button) renders. It can grow into a real ticket list later.
  context.subscriptions.push(
    vscode.window.registerTreeDataProvider('pine.board', {
      getChildren: () => [],
      getTreeItem: (e: vscode.TreeItem) => e,
    }),
  );

  // Live-detect `.pine` appearing or disappearing so the view + status bar update
  // without a window reload.
  const watcher = vscode.workspace.createFileSystemWatcher('**/.pine/config.json');
  watcher.onDidCreate(refresh);
  watcher.onDidDelete(refresh);
  context.subscriptions.push(watcher);

  context.subscriptions.push(
    vscode.commands.registerCommand('pine.openBoard', () => openBoard(context)),
    vscode.commands.registerCommand('pine.createBug', () => createTicket(context, 'bug')),
    vscode.commands.registerCommand('pine.createFeature', () => createTicket(context, 'feature')),
  );
}

export function deactivate(): void {
  serverManager?.dispose();
}

async function ensureServerManager(context: vscode.ExtensionContext): Promise<ServerManager> {
  const binary = await resolvePineBinary(context);
  if (!serverManager) {
    serverManager = new ServerManager(binary, output);
    serverManager.setOnUnexpectedExit((root) => onServerExit(root));
  } else {
    serverManager.setBinaryPath(binary);
  }
  return serverManager;
}

function onServerExit(root: string): void {
  BoardPanel.get(root)?.postError('The Pine server stopped unexpectedly.');
  void vscode.window
    .showErrorMessage('The Pine server stopped unexpectedly.', 'Restart')
    .then((choice) => {
      if (choice === 'Restart') {
        void openBoard(extContext);
      }
    });
}

function maybeWarnRemote(): void {
  if (remoteWarned || !vscode.env.remoteName) {
    return;
  }
  remoteWarned = true;
  void vscode.window.showWarningMessage(
    `Pine is running in a remote workspace (${vscode.env.remoteName}). Viewing the board works, ` +
      'but editing cards directly in the board may be blocked by the server’s localhost-origin ' +
      'policy. Use the "Pine: Create Bug/Feature" commands, which run on the remote host.',
  );
}

async function openBoard(context: vscode.ExtensionContext): Promise<void> {
  try {
    const root = await pickPineRoot();
    if (!root) {
      void vscode.window.showWarningMessage('No Pine workspace found. Run `pine init` first.');
      return;
    }
    const mgr = await ensureServerManager(context);
    const preferred = getPreferredPort();
    const server = await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Window, title: 'Starting Pine…' },
      () => mgr.resolveOrSpawn(root, preferred),
    );
    maybeWarnRemote();
    await BoardPanel.show(context, root, server.baseUrl, () => mgr.stopServer(root));
  } catch (e) {
    void vscode.window.showErrorMessage(`Pine: ${errMessage(e)}`);
  }
}

async function createTicket(
  context: vscode.ExtensionContext,
  type: 'bug' | 'feature',
): Promise<void> {
  try {
    const root = await pickPineRoot();
    if (!root) {
      void vscode.window.showWarningMessage('No Pine workspace found. Run `pine init` first.');
      return;
    }
    const title = await vscode.window.showInputBox({
      prompt: `New ${type} title`,
      placeHolder: `Describe the ${type}`,
      validateInput: (v) => (v.trim() ? null : 'Title cannot be empty'),
    });
    if (!title) {
      return;
    }
    const binary = await resolvePineBinary(context);
    const res = await pineCreate(binary, root, type, title.trim());
    const open = 'Open Board';
    const choice = await vscode.window.showInformationMessage(`Created ${res.id}: ${res.title}`, open);
    if (choice === open) {
      await openBoard(context);
    }
  } catch (e) {
    void vscode.window.showErrorMessage(`Pine: ${errMessage(e)}`);
  }
}

function getPreferredPort(): number | undefined {
  const p = vscode.workspace.getConfiguration('pine').get<number | null>('server.port');
  return typeof p === 'number' && p > 0 ? p : undefined;
}

function errMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
