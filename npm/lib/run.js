'use strict';

const { spawnSync } = require('child_process');
const fs = require('fs');

const { binaryPath } = require('./binary.js');

function run(name) {
  const bin = binaryPath(name);
  if (!fs.existsSync(bin)) {
    console.error(`airstrings: ${name} binary is missing — reinstall the package to fetch it.`);
    process.exit(1);
  }

  const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });

  if (result.error) {
    console.error(`airstrings: failed to run ${name} — ${result.error.message}`);
    process.exit(1);
  }
  if (result.signal) {
    process.kill(process.pid, result.signal);
    return;
  }
  process.exit(result.status ?? 1);
}

module.exports = { run };
