'use strict';

function checksumFor(sums, asset) {
  const line = String(sums)
    .split('\n')
    .map((l) => l.trim().split(/\s+/))
    .find(([, name]) => name === asset);
  if (!line) {
    throw new Error(`airstrings: no checksum published for ${asset}`);
  }
  return line[0];
}

module.exports = { checksumFor };
