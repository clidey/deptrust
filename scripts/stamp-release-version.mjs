#!/usr/bin/env node

import { readFileSync, writeFileSync } from "node:fs";

const version = process.argv[2];
if (!version || !/^\d+\.\d+\.\d+$/.test(version)) {
  console.error("usage: scripts/stamp-release-version.mjs <major.minor.patch>");
  process.exit(1);
}

const packageJSONPath = "package.json";
const packageJSON = JSON.parse(readFileSync(packageJSONPath, "utf8"));
packageJSON.version = version;

const skillPath = ".agents/skills/deptrust-package-check/SKILL.md";
let skill = readFileSync(skillPath, "utf8");
if (/^version: .+$/m.test(skill)) {
  skill = skill.replace(/^version: .+$/m, `version: "${version}"`);
} else {
  skill = skill.replace(/^description: (.+)$/m, `description: $1\nversion: "${version}"`);
}
writeFileSync(skillPath, skill);
writeFileSync(packageJSONPath, `${JSON.stringify(packageJSON, null, 2)}\n`);
