// Pure (no `vscode`) logic for locating and verifying the `pine` release binary.
// Kept dependency-free so it is unit-testable in plain Node.
import { createHash } from 'node:crypto';
import { spawn } from 'node:child_process';

export type Goos = 'linux' | 'darwin' | 'windows';
export type Goarch = 'amd64' | 'arm64';

export interface Target {
  os: Goos;
  arch: Goarch;
  ext: 'tar.gz' | 'zip';
  /** Name of the binary inside the extracted archive. */
  binName: string;
}

const REPO = 'underworld14/pine';

/** Map Node's process.platform/process.arch to a goreleaser build target. */
export function resolveTarget(platform: NodeJS.Platform, arch: string): Target {
  let os: Goos;
  switch (platform) {
    case 'darwin':
      os = 'darwin';
      break;
    case 'linux':
      os = 'linux';
      break;
    case 'win32':
      os = 'windows';
      break;
    default:
      throw new Error(`unsupported platform: ${platform}`);
  }
  let a: Goarch;
  switch (arch) {
    case 'x64':
      a = 'amd64';
      break;
    case 'arm64':
      a = 'arm64';
      break;
    default:
      throw new Error(`unsupported architecture: ${arch}`);
  }
  return {
    os,
    arch: a,
    ext: os === 'windows' ? 'zip' : 'tar.gz',
    binName: os === 'windows' ? 'pine.exe' : 'pine',
  };
}

/** Strip an optional leading `v` from a release version. */
export function normalizeVersion(version: string): string {
  return version.replace(/^v/, '');
}

/** goreleaser archive name, e.g. `pine_0.1.0_darwin_arm64.tar.gz`. */
export function assetName(version: string, t: Target): string {
  return `pine_${normalizeVersion(version)}_${t.os}_${t.arch}.${t.ext}`;
}

/** Full GitHub release download URL for the archive. */
export function assetUrl(version: string, t: Target): string {
  const v = normalizeVersion(version);
  return `https://github.com/${REPO}/releases/download/v${v}/${assetName(version, t)}`;
}

/** Full GitHub release download URL for checksums.txt. */
export function checksumsUrl(version: string): string {
  const v = normalizeVersion(version);
  return `https://github.com/${REPO}/releases/download/v${v}/checksums.txt`;
}

export function sha256(buf: Buffer): string {
  return createHash('sha256').update(buf).digest('hex');
}

/** Parse a goreleaser checksums.txt ("<sha256>  <filename>" per line) into a map. */
export function parseChecksums(text: string): Map<string, string> {
  const m = new Map<string, string>();
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }
    const parts = trimmed.split(/\s+/);
    if (parts.length >= 2) {
      const sum = parts[0].toLowerCase();
      // The filename may be prefixed with `*` (binary mode marker).
      const name = parts[parts.length - 1].replace(/^\*/, '');
      m.set(name, sum);
    }
  }
  return m;
}

/** Throw unless `archive`'s sha256 matches the entry for `name` in checksums.txt. */
export function verifyChecksum(archive: Buffer, checksumsTxt: string, name: string): void {
  const want = parseChecksums(checksumsTxt).get(name);
  if (!want) {
    throw new Error(`no checksum listed for ${name}`);
  }
  const got = sha256(archive);
  if (got !== want) {
    throw new Error(`checksum mismatch for ${name}: got ${got}, want ${want}`);
  }
}

/**
 * Extract a `.tar.gz` or `.zip` into destDir using the system `tar`.
 * bsdtar (macOS, Windows 10+) handles both; GNU tar (Linux) handles the
 * `.tar.gz` we always ship on Linux. Avoids a native archive dependency.
 */
export function extractArchive(archivePath: string, destDir: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const child = spawn('tar', ['-xf', archivePath, '-C', destDir], { stdio: 'ignore' });
    child.on('error', reject);
    child.on('exit', (code) =>
      code === 0 ? resolve() : reject(new Error(`tar exited with code ${code}`)),
    );
  });
}
