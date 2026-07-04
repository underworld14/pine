// Pure (no `vscode`) lifecycle manager for `pine serve` processes. One server per
// workspace root: reuse an already-running instance (matched by /api/health repo)
// or spawn our own on a free port. A minimal Logger interface keeps this testable
// in plain Node while structurally accepting a vscode.OutputChannel.
import { spawn, ChildProcess } from 'node:child_process';
import { checkHealth, waitForHealth, findFreePort, portAvailable, samePath } from './health';

const DEFAULT_PORT = 3412;

export interface Logger {
  append(value: string): void;
  appendLine(value: string): void;
}

export interface PineServer {
  baseUrl: string;
  port: number;
  /** True only when this manager spawned the process (so we may stop it). */
  ownedByUs: boolean;
  child?: ChildProcess;
  /** Internal: set when we are intentionally stopping this server. */
  stopping?: boolean;
}

export class ServerManager {
  private servers = new Map<string, PineServer>();
  private inflight = new Map<string, Promise<PineServer>>();
  private onExit?: (root: string) => void;

  constructor(
    private binaryPath: string,
    private readonly log?: Logger,
  ) {}

  setBinaryPath(p: string): void {
    this.binaryPath = p;
  }

  /** Register a callback fired when a server WE own exits without us stopping it. */
  setOnUnexpectedExit(cb: (root: string) => void): void {
    this.onExit = cb;
  }

  /**
   * Return a healthy server for `root`, reusing a running one or spawning our own.
   * Concurrent calls for the same root join a single in-flight attempt so a
   * double-click can never spawn (and orphan) two servers.
   */
  resolveOrSpawn(root: string, preferredPort?: number): Promise<PineServer> {
    const pending = this.inflight.get(root);
    if (pending) {
      return pending;
    }
    const p = this.doResolveOrSpawn(root, preferredPort).finally(() => {
      this.inflight.delete(root);
    });
    this.inflight.set(root, p);
    return p;
  }

  private async doResolveOrSpawn(root: string, preferredPort?: number): Promise<PineServer> {
    const cached = this.servers.get(root);
    if (cached && (await this.isAlive(cached, root))) {
      return cached;
    }
    if (cached) {
      // Stale entry — stop it (kills the child if we own it and it is still alive).
      this.stopServer(root);
    }

    // 1) Reuse an already-running server (preferred port, then the default).
    const candidates = new Set<number>();
    if (preferredPort) {
      candidates.add(preferredPort);
    }
    candidates.add(DEFAULT_PORT);
    for (const port of candidates) {
      const baseUrl = `http://127.0.0.1:${port}`;
      const h = await checkHealth(baseUrl, 500);
      if (h?.ok && samePath(h.repo, root)) {
        const reused: PineServer = { baseUrl, port, ownedByUs: false };
        this.servers.set(root, reused);
        this.log?.appendLine(`[pine] reusing server at ${baseUrl} for ${root}`);
        return reused;
      }
    }

    // 2) Spawn our own. Honor preferredPort only if it is actually free.
    const port =
      preferredPort && (await portAvailable(preferredPort)) ? preferredPort : await findFreePort();
    const baseUrl = `http://127.0.0.1:${port}`;
    this.log?.appendLine(`[pine] starting: ${this.binaryPath} serve -C ${root} --port ${port}`);
    const child = spawn(
      this.binaryPath,
      ['serve', '-C', root, '--port', String(port), '--host', '127.0.0.1'],
      { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] },
    );
    child.stdout?.on('data', (d) => this.log?.append(d.toString()));
    child.stderr?.on('data', (d) => this.log?.append(d.toString()));

    const server: PineServer = { baseUrl, port, ownedByUs: true, child };
    this.servers.set(root, server);
    child.on('exit', (code) => {
      this.log?.appendLine(`[pine] server for ${root} exited (code ${code ?? 'signal'})`);
      const cur = this.servers.get(root);
      if (cur?.child === child) {
        this.servers.delete(root);
        if (!server.stopping) {
          this.onExit?.(root);
        }
      }
    });

    const healthy = await waitForHealth(baseUrl, { repo: root, timeoutMs: 10000, intervalMs: 200 });
    if (!healthy) {
      this.stopServer(root);
      throw new Error(`Pine server did not become healthy on ${baseUrl}`);
    }
    return server;
  }

  private async isAlive(s: PineServer, root: string): Promise<boolean> {
    if (s.child && s.child.exitCode !== null) {
      return false;
    }
    const h = await checkHealth(s.baseUrl, 500);
    return !!(h?.ok && samePath(h.repo, root));
  }

  /** Stop only a server we spawned; leave reused/user-run servers running. */
  stopServer(root: string): void {
    const s = this.servers.get(root);
    if (!s) {
      return;
    }
    this.servers.delete(root);
    if (s.ownedByUs && s.child && s.child.exitCode === null) {
      s.stopping = true;
      s.child.kill('SIGTERM');
    }
  }

  dispose(): void {
    for (const root of Array.from(this.servers.keys())) {
      this.stopServer(root);
    }
  }
}
