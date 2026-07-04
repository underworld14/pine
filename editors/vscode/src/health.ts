// Pure (no `vscode`) helpers for probing a running Pine server and picking ports.
import * as net from 'node:net';
import * as fs from 'node:fs';

export interface Health {
  ok: boolean;
  version?: string;
  project?: string;
  repo?: string;
}

/** GET {baseUrl}/api/health with a timeout. Returns parsed health, or null on any failure. */
export async function checkHealth(baseUrl: string, timeoutMs = 500): Promise<Health | null> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${baseUrl}/api/health`, { signal: controller.signal });
    if (!res.ok) {
      return null;
    }
    return (await res.json()) as Health;
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}

export interface WaitOptions {
  repo?: string;
  timeoutMs?: number;
  intervalMs?: number;
}

/** Poll checkHealth until healthy (and repo matches, if given) or the deadline passes. */
export async function waitForHealth(baseUrl: string, opts: WaitOptions = {}): Promise<Health | null> {
  const timeoutMs = opts.timeoutMs ?? 10000;
  const intervalMs = opts.intervalMs ?? 200;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const h = await checkHealth(baseUrl, Math.min(1000, Math.max(250, intervalMs * 2)));
    if (h?.ok && (!opts.repo || samePath(h.repo, opts.repo))) {
      return h;
    }
    await delay(intervalMs);
  }
  return null;
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

/** Find a free TCP port on 127.0.0.1. */
export function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.on('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address();
      if (addr && typeof addr === 'object') {
        const { port } = addr;
        srv.close(() => resolve(port));
      } else {
        srv.close(() => reject(new Error('could not determine a free port')));
      }
    });
  });
}

/** True if `port` can currently be bound on 127.0.0.1 (i.e. nothing is listening). */
export function portAvailable(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const srv = net.createServer();
    srv.once('error', () => resolve(false));
    srv.once('listening', () => srv.close(() => resolve(true)));
    srv.listen(port, '127.0.0.1');
  });
}

/**
 * Compare two filesystem paths for equality, resolving symlinks where possible
 * (e.g. macOS /var -> /private/var) so a server's reported `repo` matches the
 * workspace root. Falls back to normalized string comparison.
 */
export function samePath(a?: string, b?: string): boolean {
  if (!a || !b) {
    return false;
  }
  const real = (p: string): string => {
    try {
      return fs.realpathSync(p);
    } catch {
      return p;
    }
  };
  const norm = (p: string): string => real(p).replace(/[/\\]+$/, '');
  const na = norm(a);
  const nb = norm(b);
  if (na === nb) {
    return true;
  }
  return process.platform === 'win32' && na.toLowerCase() === nb.toLowerCase();
}
