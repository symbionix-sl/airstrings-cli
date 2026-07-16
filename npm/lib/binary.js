'use strict';

const fs = require('fs');
const path = require('path');

const TARGETS = {
  'darwin-arm64': { asset: 'darwin-arm64', ext: 'tar.gz' },
  'darwin-x64': { asset: 'darwin-amd64', ext: 'tar.gz' },
  'linux-arm64': { asset: 'linux-arm64', ext: 'tar.gz' },
  'linux-x64': { asset: 'linux-amd64', ext: 'tar.gz' },
  'win32-x64': { asset: 'windows-amd64', ext: 'zip' },
};

const vendorDir = path.join(__dirname, '..', 'vendor');

function target() {
  const key = `${process.platform}-${process.arch}`;
  const found = TARGETS[key];
  if (!found) {
    throw new Error(
      `airstrings: no prebuilt binary for ${key}. Supported: ${Object.keys(TARGETS).join(', ')}. ` +
        `Install with Homebrew instead: brew install symbionix-sl/airstrings/airstrings`
    );
  }
  return found;
}

function binaryPath(name) {
  const ext = process.platform === 'win32' ? '.exe' : '';
  return path.join(vendorDir, `${name}${ext}`);
}

// Git Bash's GNU tar shadows Windows' bsdtar on PATH: it parses C:\ as a
// remote host and cannot read .zip. Always prefer the System32 one.
function tarBinary() {
  if (process.platform === 'win32') {
    const sysTar = path.join(process.env.SystemRoot || 'C:\\Windows', 'System32', 'tar.exe');
    if (fs.existsSync(sysTar)) {
      return sysTar;
    }
  }
  return 'tar';
}

module.exports = { target, vendorDir, binaryPath, tarBinary };
