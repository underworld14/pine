// End-to-end tests that drive the REAL `pine` binary (no VS Code host needed).
// Set PINE_BIN to the binary path, or place a built `pine` at the repo root.
import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { spawnSync } from 'node:child_process';
import { ServerManager } from '../../src/server';
import { checkHealth } from '../../src/health';

const PINE = resolvePineBin();

function resolvePineBin(): string {
  if (process.env.PINE_BIN && fs.existsSync(process.env.PINE_BIN)) {
    return process.env.PINE_BIN;
  }
  // compiled test lives at editors/vscode/out/test/e2e/ -> repo root is 5 up
  return path.resolve(__dirname, '../../../../../pine');
}

function initRepo(): string {
  const tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'pine-e2e-')));
  const r = spawnSync(PINE, ['init'], { cwd: tmp });
  if (r.status !== 0) {
    throw new Error(`pine init failed: ${r.stderr?.toString()}`);
  }
  return tmp;
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

describe('e2e: ServerManager against the real pine binary', function () {
  this.timeout(30000);
  let root: string;
  let mgr: ServerManager;

  before(function () {
    if (!fs.existsSync(PINE)) {
      // eslint-disable-next-line no-console
      console.warn(`skipping e2e: pine binary not found at ${PINE} (set PINE_BIN)`);
      this.skip();
    }
  });
  beforeEach(() => {
    root = initRepo();
    mgr = new ServerManager(PINE);
  });
  afterEach(() => {
    mgr.dispose();
    fs.rmSync(root, { recursive: true, force: true });
  });

  it('spawns a healthy server we own', async () => {
    const s = await mgr.resolveOrSpawn(root);
    assert.ok(s.ownedByUs, 'server should be owned by us');
    assert.match(s.baseUrl, /^http:\/\/127\.0\.0\.1:\d+$/);
    const h = await checkHealth(s.baseUrl, 1000);
    assert.ok(h?.ok, 'health endpoint should report ok');
    assert.ok(h?.repo, 'health should report a repo');
  });

  it('reuses the same server on a second call', async () => {
    const a = await mgr.resolveOrSpawn(root);
    const b = await mgr.resolveOrSpawn(root);
    assert.strictEqual(a.baseUrl, b.baseUrl);
    assert.strictEqual(a.child, b.child);
  });

  it('serializes concurrent calls into a single spawn (no orphans)', async () => {
    const [a, b, c] = await Promise.all([
      mgr.resolveOrSpawn(root),
      mgr.resolveOrSpawn(root),
      mgr.resolveOrSpawn(root),
    ]);
    assert.strictEqual(a.child, b.child, 'all concurrent callers share one child');
    assert.strictEqual(b.child, c.child);
    assert.ok(a.ownedByUs);
  });

  it('stops the owned server on stopServer', async () => {
    const s = await mgr.resolveOrSpawn(root);
    mgr.stopServer(root);
    await delay(1500);
    const h = await checkHealth(s.baseUrl, 500);
    assert.strictEqual(h, null, 'server should no longer answer after being stopped');
  });
});
