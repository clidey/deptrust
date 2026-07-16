#!/usr/bin/env node

import fs from "node:fs";

const [tag, checksumsPath, flakePath = "flake.nix"] = process.argv.slice(2);

if (!tag || !checksumsPath) {
  console.error("usage: update-nix-flake.mjs <vX.Y.Z> <checksums.txt> [flake.nix]");
  process.exit(2);
}
if (!/^v\d+\.\d+\.\d+(?:[.-][0-9A-Za-z.-]+)?$/.test(tag)) {
  throw new Error(`invalid release tag: ${tag}`);
}

const version = tag.slice(1);
const assetSuffixes = {
  "x86_64-linux": "linux_amd64.tar.gz",
  "aarch64-linux": "linux_arm64.tar.gz",
  "x86_64-darwin": "darwin_amd64.tar.gz",
  "aarch64-darwin": "darwin_arm64.tar.gz",
};

const checksums = new Map();
for (const line of fs.readFileSync(checksumsPath, "utf8").split(/\r?\n/)) {
  const match = line.trim().match(/^([0-9a-fA-F]{64})\s+(.+)$/);
  if (match) {
    checksums.set(match[2], match[1].toLowerCase());
  }
}

let source = fs.readFileSync(flakePath, "utf8");
let replacements = 0;
source = source.replace(/version = "[^"]+";/, () => {
  replacements += 1;
  return `version = "${version}";`;
});
if (replacements !== 1) {
  throw new Error(`expected one version field in ${flakePath}, replaced ${replacements}`);
}

for (const [system, suffix] of Object.entries(assetSuffixes)) {
  const checksumFile = `deptrust_${tag}_${suffix}`;
  const flakeFile = `deptrust_v\${version}_${suffix}`;
  const hex = checksums.get(checksumFile);
  if (!hex) {
    throw new Error(`missing checksum for ${checksumFile}`);
  }
  const sri = `sha256-${Buffer.from(hex, "hex").toString("base64")}`;
  const blockPattern = new RegExp(`("${system}" = \\{[\\s\\S]*?\\n\\s*file = ")[^"]*(";[\\s\\S]*?\\n\\s*sha256 = ")[^"]*(";)`);
  let count = 0;
  source = source.replace(blockPattern, (_match, beforeFile, afterFile, afterHash) => {
    count += 1;
    return `${beforeFile}${flakeFile}${afterFile}${sri}${afterHash}`;
  });
  if (count !== 1) {
    throw new Error(`expected one asset block for ${system}, replaced ${count}`);
  }
}

fs.writeFileSync(flakePath, source);
console.log(`updated ${flakePath} to ${tag}`);
