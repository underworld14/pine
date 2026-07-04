import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { isExecutable, findOnPath } from '../../src/locate';

describe('locate', () => {
  let tmp: string;

  beforeEach(() => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'pine-loc-')));
  });
  afterEach(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  it('isExecutable is false for a missing file', () => {
    assert.ok(!isExecutable(path.join(tmp, 'nope')));
  });

  it('isExecutable is false for a non-executable file (POSIX)', function () {
    if (process.platform === 'win32') {
      this.skip();
      return;
    }
    const f = path.join(tmp, 'plain');
    fs.writeFileSync(f, 'x', { mode: 0o644 });
    assert.ok(!isExecutable(f));
  });

  it('findOnPath locates an executable on PATH', function () {
    if (process.platform === 'win32') {
      this.skip();
      return;
    }
    const bin = path.join(tmp, 'pine');
    fs.writeFileSync(bin, '#!/bin/sh\necho ok\n', { mode: 0o755 });
    const found = findOnPath('pine', { PATH: tmp } as NodeJS.ProcessEnv, () => true);
    assert.strictEqual(found, bin);
  });

  it('findOnPath returns null when absent', () => {
    const found = findOnPath('pine', { PATH: tmp } as NodeJS.ProcessEnv, () => true);
    assert.strictEqual(found, null);
  });

  it('findOnPath skips entries that fail the version check', function () {
    if (process.platform === 'win32') {
      this.skip();
      return;
    }
    const bin = path.join(tmp, 'pine');
    fs.writeFileSync(bin, '#!/bin/sh\n', { mode: 0o755 });
    const found = findOnPath('pine', { PATH: tmp } as NodeJS.ProcessEnv, () => false);
    assert.strictEqual(found, null);
  });
});
