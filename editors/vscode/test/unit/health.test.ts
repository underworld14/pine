import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { findFreePort, samePath } from '../../src/health';

describe('health: findFreePort', () => {
  it('returns a usable port number', async () => {
    const p = await findFreePort();
    assert.ok(Number.isInteger(p) && p > 0 && p < 65536, `port ${p} out of range`);
  });
});

describe('health: samePath', () => {
  let tmp: string;

  beforeEach(() => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'pine-sp-')));
  });
  afterEach(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  it('matches equal paths', () => {
    assert.ok(samePath(tmp, tmp));
  });

  it('ignores a trailing separator', () => {
    assert.ok(samePath(tmp, tmp + path.sep));
  });

  it('resolves symlinks before comparing', () => {
    assert.ok(samePath(tmp, fs.realpathSync(tmp)));
  });

  it('does not match different paths', () => {
    assert.ok(!samePath(tmp, path.join(tmp, 'child')));
  });

  it('does not match when either side is undefined', () => {
    assert.ok(!samePath(undefined, tmp));
    assert.ok(!samePath(tmp, undefined));
  });
});
