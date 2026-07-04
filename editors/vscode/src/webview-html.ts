// Pure (no `vscode`) builder for the webview shell HTML. The shell is a strict
// nonce-CSP page whose body is a full-viewport iframe pointing at the local Pine
// server; inside that iframe the origin is http://127.0.0.1:<port>, so the Svelte
// app's same-origin API/SSE/attachment requests all work unchanged.
import { randomBytes } from 'node:crypto';

export interface BoardHtmlParams {
  /** The iframe src (result of vscode.env.asExternalUri, stringified). */
  iframeSrc: string;
  /** The origin (scheme://host:port) of iframeSrc, for the CSP frame-src. */
  frameOrigin: string;
  /** A per-render nonce for the shell's inline <style>/<script>. */
  nonce: string;
}

/** Escape a value for safe inclusion in an HTML attribute / CSP directive. */
function attr(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

export function getBoardHtml(params: BoardHtmlParams): string {
  const iframeSrc = attr(params.iframeSrc);
  const frameOrigin = attr(params.frameOrigin);
  const nonce = attr(params.nonce);
  const csp = [
    `default-src 'none'`,
    `frame-src ${frameOrigin}`,
    `img-src ${frameOrigin} data:`,
    `style-src 'nonce-${nonce}'`,
    `script-src 'nonce-${nonce}'`,
  ].join('; ');

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Pine Board</title>
<style nonce="${nonce}">
  html, body { margin: 0; padding: 0; height: 100%; width: 100%; overflow: hidden;
    background: var(--vscode-editor-background, #1e1e1e); }
  iframe { border: 0; position: absolute; inset: 0; width: 100%; height: 100%; }
  #overlay { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center;
    font-family: var(--vscode-font-family, sans-serif); font-size: 13px;
    color: var(--vscode-foreground, #ccc); background: var(--vscode-editor-background, #1e1e1e); }
  #overlay.hidden { display: none; }
  .spinner { width: 16px; height: 16px; margin-right: 10px; border: 2px solid currentColor;
    border-top-color: transparent; border-radius: 50%; animation: spin 0.8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
</head>
<body>
<div id="overlay"><span class="spinner"></span> Starting Pine…</div>
<iframe id="frame" src="${iframeSrc}" allow="clipboard-read; clipboard-write"></iframe>
<script nonce="${nonce}">
  const frame = document.getElementById('frame');
  const overlay = document.getElementById('overlay');
  frame.addEventListener('load', () => overlay.classList.add('hidden'));
  window.addEventListener('message', (e) => {
    const msg = e.data;
    if (msg && msg.type === 'pine:error') {
      overlay.classList.remove('hidden');
      overlay.textContent = msg.message || 'Pine failed to start.';
    }
  });
</script>
</body>
</html>`;
}

/**
 * A random 32-char alphanumeric nonce for the shell's inline script/style.
 * Uses a CSPRNG (node:crypto) — a CSP nonce's whole value is its unpredictability.
 */
export function makeNonce(): string {
  let s = '';
  while (s.length < 32) {
    s += randomBytes(24).toString('base64').replace(/[^A-Za-z0-9]/g, '');
  }
  return s.slice(0, 32);
}
