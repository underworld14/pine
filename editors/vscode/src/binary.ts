// Resolves a usable `pine` binary. Order: `pine.path` setting -> PATH -> download
// the matching release from GitHub into extension global storage.
import * as vscode from 'vscode';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { isExecutable, findOnPath } from './locate';
import {
  resolveTarget,
  assetName,
  assetUrl,
  checksumsUrl,
  verifyChecksum,
  extractArchive,
} from './download';

// Pinned known-good Pine release the extension downloads when none is found.
export const PINNED_VERSION = '0.1.0';

export async function resolvePineBinary(context: vscode.ExtensionContext): Promise<string> {
  const cfg = vscode.workspace.getConfiguration('pine');
  const override = (cfg.get<string>('path') || '').trim();
  if (override) {
    if (isExecutable(override)) {
      return override;
    }
    throw new Error(`pine.path is set to "${override}" but it is not an executable file.`);
  }

  const onPath = findOnPath('pine');
  if (onPath) {
    return onPath;
  }

  if (!cfg.get<boolean>('autoDownload', true)) {
    throw new Error(
      'Pine binary not found on PATH and pine.autoDownload is disabled. Install Pine or set pine.path.',
    );
  }
  return downloadPine(context, PINNED_VERSION);
}

async function downloadPine(context: vscode.ExtensionContext, version: string): Promise<string> {
  const target = resolveTarget(process.platform, process.arch);
  const cacheDir = path.join(context.globalStorageUri.fsPath, 'pine', version);
  const binPath = path.join(cacheDir, target.binName);
  // cacheDir is only ever created via an atomic rename below, so its presence
  // means a fully-verified, fully-extracted binary — the exec-bit check is sound.
  if (isExecutable(binPath)) {
    return binPath;
  }

  await vscode.window.withProgress(
    {
      location: vscode.ProgressLocation.Notification,
      title: `Downloading Pine ${version}…`,
      cancellable: false,
    },
    async () => {
      const parent = path.dirname(cacheDir);
      fs.mkdirSync(parent, { recursive: true });
      const tmpDir = path.join(parent, `.tmp-${version}-${process.pid}-${Date.now()}`);
      fs.rmSync(tmpDir, { recursive: true, force: true });
      fs.mkdirSync(tmpDir, { recursive: true });
      try {
        const name = assetName(version, target);
        const [archive, checksums] = await Promise.all([
          fetchBuffer(assetUrl(version, target)),
          fetchText(checksumsUrl(version)),
        ]);
        // Integrity is verified on the in-memory buffer, before any disk write.
        verifyChecksum(archive, checksums, name);
        const archivePath = path.join(tmpDir, name);
        fs.writeFileSync(archivePath, archive);
        await extractArchive(archivePath, tmpDir);
        fs.rmSync(archivePath, { force: true });

        const tmpBin = path.join(tmpDir, target.binName);
        if (!fs.existsSync(tmpBin)) {
          throw new Error('downloaded archive did not contain the pine binary');
        }
        if (process.platform !== 'win32') {
          fs.chmodSync(tmpBin, 0o755);
        }
        // Publish atomically: only now does cacheDir come into existence.
        fs.rmSync(cacheDir, { recursive: true, force: true });
        fs.renameSync(tmpDir, cacheDir);
      } finally {
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    },
  );

  if (!isExecutable(binPath)) {
    throw new Error('Pine download completed but the binary is missing or not executable.');
  }
  return binPath;
}

async function fetchBuffer(url: string): Promise<Buffer> {
  const res = await fetch(url, { redirect: 'follow' });
  if (!res.ok) {
    throw new Error(`download failed (${res.status}) for ${url}`);
  }
  return Buffer.from(await res.arrayBuffer());
}

async function fetchText(url: string): Promise<string> {
  const res = await fetch(url, { redirect: 'follow' });
  if (!res.ok) {
    throw new Error(`download failed (${res.status}) for ${url}`);
  }
  return res.text();
}
