'use strict';

const assert = require('assert');
const { execFileSync } = require('child_process');
const path = require('path');

const { checksumFor } = require('./lib/checksums.js');
const { target, binaryPath, tarBinary } = require('./lib/binary.js');

const SUMS = [
  'aaa1  airstrings-v1.2.3-darwin-amd64.tar.gz',
  'bbb2  airstrings-v1.2.3-darwin-arm64.tar.gz',
  'ccc3  airstrings-v1.2.3-windows-amd64.zip',
  '',
].join('\n');

assert.strictEqual(checksumFor(SUMS, 'airstrings-v1.2.3-darwin-arm64.tar.gz'), 'bbb2');
assert.strictEqual(checksumFor(SUMS, 'airstrings-v1.2.3-windows-amd64.zip'), 'ccc3');
assert.throws(() => checksumFor(SUMS, 'airstrings-v1.2.3-linux-amd64.tar.gz'), /no checksum published/);

// A prefix of another asset name must not match.
assert.throws(() => checksumFor(SUMS, 'airstrings-v1.2.3-darwin'), /no checksum published/);

const t = target();
assert.ok(t.asset && ['tar.gz', 'zip'].includes(t.ext));
assert.strictEqual(t.ext === 'zip', process.platform === 'win32');
assert.strictEqual(path.basename(binaryPath('airstrings')), process.platform === 'win32' ? 'airstrings.exe' : 'airstrings');

// tar must exist: install.js shells out to it to unpack both .tar.gz and .zip.
const tar = tarBinary();
if (process.platform === 'win32') {
  assert.ok(path.isAbsolute(tar), `expected System32 tar.exe on Windows, got ${tar}`);
} else {
  assert.strictEqual(tar, 'tar');
}
execFileSync(tar, ['--version'], { stdio: 'ignore' });

console.log('ok');
