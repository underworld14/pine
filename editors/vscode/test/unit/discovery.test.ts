import * as assert from 'node:assert';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { findPineDir, pineRootFrom } from '../../src/discovery';

describe('discovery', () => {
  let tmp: string;

  beforeEach(() => {
    tmp = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'pine-disc-')));
  });
  afterEach(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  it('finds .pine at the root', () => {
    fs.mkdirSync(path.join(tmp, '.pine'));
    assert.strictEqual(findPineDir(tmp), path.join(tmp, '.pine'));
    assert.strictEqual(pineRootFrom(tmp), tmp);
  });

  it('finds .pine from a nested subdirectory', () => {
    fs.mkdirSync(path.join(tmp, '.pine'));
    const nested = path.join(tmp, 'a', 'b', 'c');
    fs.mkdirSync(nested, { recursive: true });
    assert.strictEqual(findPineDir(nested), path.join(tmp, '.pine'));
    assert.strictEqual(pineRootFrom(nested), tmp);
  });

  it('returns null when no .pine exists', () => {
    assert.strictEqual(findPineDir(tmp), null);
    assert.strictEqual(pineRootFrom(tmp), null);
  });

  it('ignores a .pine that is a file, not a directory', () => {
    fs.writeFileSync(path.join(tmp, '.pine'), 'not a dir');
    assert.strictEqual(findPineDir(tmp), null);
  });
});
