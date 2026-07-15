'use strict';

const crypto = require('crypto');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const { target, vendorDir, binaryPath } = require('./lib/binary.js');
const { checksumFor } = require('./lib/checksums.js');
const { version } = require('./package.json');

const RELEASE_BASE = `https://github.com/symbionix-sl/homebrew-airstrings/releases/download/v${version}`;

async function download(url) {
  const res = await fetch(url, { redirect: 'follow' });
  if (!res.ok) {
    throw new Error(`airstrings: download failed with HTTP ${res.status} — ${url}`);
  }
  return Buffer.from(await res.arrayBuffer());
}

async function main() {
  const { asset: platform, ext } = target();
  const asset = `airstrings-v${version}-${platform}.${ext}`;

  const [archive, sums] = await Promise.all([
    download(`${RELEASE_BASE}/${asset}`),
    download(`${RELEASE_BASE}/SHA256SUMS`),
  ]);

  const expected = checksumFor(sums, asset);
  const actual = crypto.createHash('sha256').update(archive).digest('hex');
  if (actual !== expected) {
    throw new Error(
      `airstrings: checksum mismatch for ${asset}\n  expected ${expected}\n  actual   ${actual}\n` +
        `Refusing to install a binary that does not match the published checksum.`
    );
  }

  const tmp = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'airstrings-')), asset);
  fs.writeFileSync(tmp, archive);

  fs.rmSync(vendorDir, { recursive: true, force: true });
  fs.mkdirSync(vendorDir, { recursive: true });
  execFileSync('tar', ['-xf', tmp, '-C', vendorDir]);
  fs.rmSync(path.dirname(tmp), { recursive: true, force: true });

  if (process.platform !== 'win32') {
    for (const name of ['airstrings', 'airstrings-mcp']) {
      fs.chmodSync(binaryPath(name), 0o755);
    }
  }
}

main().catch((err) => {
  console.error(err.message);
  process.exit(1);
});
