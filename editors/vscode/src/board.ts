// The Pine board webview: one panel per workspace root, wrapping a full-viewport
// iframe that points at the local Pine server. All UI reuse happens here — the
// Svelte app is served, unchanged, by `pine serve` inside the iframe.
import * as vscode from 'vscode';
import { getBoardHtml, makeNonce } from './webview-html';

export class BoardPanel {
  private static panels = new Map<string, BoardPanel>();
  private readonly panel: vscode.WebviewPanel;
  private readonly disposables: vscode.Disposable[] = [];
  private baseUrl: string;
  private disposed = false;

  static async show(
    context: vscode.ExtensionContext,
    root: string,
    baseUrl: string,
    onDispose: () => void,
  ): Promise<BoardPanel> {
    const existing = BoardPanel.panels.get(root);
    if (existing) {
      // Server may have restarted on a different port — re-point the iframe.
      if (existing.baseUrl !== baseUrl) {
        existing.baseUrl = baseUrl;
        await existing.render();
      }
      existing.panel.reveal();
      return existing;
    }
    const bp = new BoardPanel(context, root, baseUrl, onDispose);
    BoardPanel.panels.set(root, bp);
    await bp.render();
    return bp;
  }

  /** The open board for `root`, if any. */
  static get(root: string): BoardPanel | undefined {
    return BoardPanel.panels.get(root);
  }

  private constructor(
    context: vscode.ExtensionContext,
    private readonly root: string,
    baseUrl: string,
    private readonly onDisposeCb: () => void,
  ) {
    this.baseUrl = baseUrl;
    this.panel = vscode.window.createWebviewPanel(
      'pineBoard',
      'Pine Board',
      vscode.ViewColumn.Active,
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [vscode.Uri.joinPath(context.extensionUri, 'media')],
      },
    );
    this.panel.iconPath = vscode.Uri.joinPath(context.extensionUri, 'media', 'pine.svg');
    this.panel.onDidDispose(() => this.dispose(), null, this.disposables);
  }

  private async render(): Promise<void> {
    const external = await vscode.env.asExternalUri(vscode.Uri.parse(this.baseUrl));
    const iframeSrc = external.toString();
    this.panel.webview.html = getBoardHtml({
      iframeSrc,
      frameOrigin: originOf(iframeSrc),
      nonce: makeNonce(),
    });
  }

  postError(message: string): void {
    void this.panel.webview.postMessage({ type: 'pine:error', message });
  }

  private dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    BoardPanel.panels.delete(this.root);
    this.onDisposeCb();
    for (const d of this.disposables) {
      d.dispose();
    }
    this.panel.dispose();
  }
}

function originOf(url: string): string {
  try {
    return new URL(url).origin;
  } catch {
    return url;
  }
}
