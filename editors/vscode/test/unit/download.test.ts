import * as assert from 'node:assert';
import {
  resolveTarget,
  assetName,
  assetUrl,
  checksumsUrl,
  parseChecksums,
  verifyChecksum,
  sha256,
  normalizeVersion,
} from '../../src/download';

describe('download: target mapping', () => {
  it('maps darwin/arm64', () => {
    assert.deepStrictEqual(resolveTarget('darwin', 'arm64'), {
      os: 'darwin',
      arch: 'arm64',
      ext: 'tar.gz',
      binName: 'pine',
    });
  });

  it('maps linux/x64 to amd64/tar.gz', () => {
    const t = resolveTarget('linux', 'x64');
    assert.strictEqual(t.arch, 'amd64');
    assert.strictEqual(t.ext, 'tar.gz');
    assert.strictEqual(t.binName, 'pine');
  });

  it('maps win32/x64 to zip + pine.exe', () => {
    const t = resolveTarget('win32', 'x64');
    assert.strictEqual(t.os, 'windows');
    assert.strictEqual(t.ext, 'zip');
    assert.strictEqual(t.binName, 'pine.exe');
  });

  it('rejects unsupported platform/arch', () => {
    assert.throws(() => resolveTarget('freebsd' as NodeJS.Platform, 'x64'), /unsupported platform/);
    assert.throws(() => resolveTarget('linux', 'ia32'), /unsupported architecture/);
  });
});

describe('download: names and urls', () => {
  const t = resolveTarget('darwin', 'arm64');

  it('assetName strips a leading v', () => {
    assert.strictEqual(assetName('v0.1.0', t), 'pine_0.1.0_darwin_arm64.tar.gz');
    assert.strictEqual(assetName('0.1.0', t), 'pine_0.1.0_darwin_arm64.tar.gz');
  });

  it('assetUrl points at the GitHub release', () => {
    assert.strictEqual(
      assetUrl('0.1.0', t),
      'https://github.com/underworld14/pine/releases/download/v0.1.0/pine_0.1.0_darwin_arm64.tar.gz',
    );
  });

  it('checksumsUrl points at checksums.txt', () => {
    assert.strictEqual(
      checksumsUrl('v0.1.0'),
      'https://github.com/underworld14/pine/releases/download/v0.1.0/checksums.txt',
    );
  });

  it('normalizeVersion strips a leading v only', () => {
    assert.strictEqual(normalizeVersion('v1.2.3'), '1.2.3');
    assert.strictEqual(normalizeVersion('1.2.3'), '1.2.3');
  });
});

describe('download: checksums', () => {
  const buf = Buffer.from('hello pine');
  const sum = sha256(buf);
  const name = 'pine_0.1.0_darwin_arm64.tar.gz';
  const checksums = `${sum}  ${name}\n0000000000000000000000000000000000000000000000000000000000000000  other.zip\n`;

  it('parses "<sha>  <file>" lines', () => {
    const m = parseChecksums(checksums);
    assert.strictEqual(m.get(name), sum);
    assert.strictEqual(m.size, 2);
  });

  it('passes on a matching checksum', () => {
    assert.doesNotThrow(() => verifyChecksum(buf, checksums, name));
  });

  it('throws on a mismatched checksum', () => {
    assert.throws(() => verifyChecksum(Buffer.from('tampered'), checksums, name), /mismatch/);
  });

  it('throws when the file is not listed', () => {
    assert.throws(() => verifyChecksum(buf, checksums, 'missing.tar.gz'), /no checksum/);
  });
});
