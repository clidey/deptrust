#!/usr/bin/env node

import { createWriteStream, existsSync, mkdirSync, chmodSync, copyFileSync, rmSync, cpSync, renameSync, readFileSync, accessSync, constants } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { tmpdir, homedir } from "node:os";
import { randomBytes } from "node:crypto";
import { createInterface } from "node:readline/promises";
import https from "node:https";

const repo = "clidey/deptrust";
const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

main().catch((error) => {
  console.error(`error: ${error.message}`);
  process.exit(1);
});

async function main() {
  const args = process.argv.slice(2);
  if (args.length === 0 || args.includes("-h") || args.includes("--help")) {
    printUsage();
    return;
  }

  const command = args[0];
  if (command === "install") {
    const options = parseOptions(args.slice(1));
    await maybePromptInstallOptions(options);
    await confirmInstallPlan(options);
    const installPath = await installBinary(options);
    await maybeInstallSkill(options);
    maybeRegisterMCP(options, installPath);
    printNextSteps(installPath);
    return;
  }

  if (command === "uninstall") {
    const options = parseOptions(args.slice(1));
    applyDefaultUninstallSelection(options);
    await confirmUninstallPlan(options);
    uninstall(options);
    return;
  }

  if (command === "skills" && args[1] === "install") {
    const options = parseOptions(args.slice(2));
    options.codexSkill = true;
    await confirmInstallPlan(options);
    const installPath = await installBinary(options);
    await maybeInstallSkill(options);
    printNextSteps(installPath);
    return;
  }

  if (command === "mcp" && args[1] === "install") {
    const options = parseOptions(args.slice(2));
    if (!options.codexMCP && !options.claudeCodeMCP) {
      options.codexMCP = true;
      options.claudeCodeMCP = true;
    }
    await confirmInstallPlan(options);
    const installPath = await installBinary(options);
    maybeRegisterMCP(options, installPath);
    printNextSteps(installPath);
    return;
  }

  throw new Error(`unknown command: ${args.join(" ")}`);
}

function parseOptions(args) {
  const options = {
    version: "latest",
    binDir: process.env.DEPTRUST_BIN_DIR || join(homedir(), ".local", "bin"),
    codexMCP: false,
    codexSkill: false,
    claudeCodeMCP: false,
    check: false,
    yes: false,
    guided: false,
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    switch (arg) {
      case "--version":
        options.version = requiredValue(args, ++i, "--version");
        break;
      case "--bin-dir":
        options.binDir = requiredValue(args, ++i, "--bin-dir");
        break;
      case "--codex-mcp":
        options.codexMCP = true;
        break;
      case "--codex-skill":
        options.codexSkill = true;
        break;
      case "--claude-code-mcp":
        options.claudeCodeMCP = true;
        break;
      case "--all":
        options.codexMCP = true;
        options.codexSkill = true;
        options.claudeCodeMCP = true;
        break;
      case "--check":
        options.check = true;
        break;
      case "-y":
      case "--yes":
        options.yes = true;
        break;
      default:
        throw new Error(`unknown option: ${arg}`);
    }
  }

  options.guided = !options.yes && !options.codexMCP && !options.codexSkill && !options.claudeCodeMCP;
  return options;
}

function requiredValue(args, index, flag) {
  if (index >= args.length || args[index].startsWith("--")) {
    throw new Error(`${flag} requires a value`);
  }
  return args[index];
}

function applyDefaultUninstallSelection(options) {
  if (options.codexMCP || options.codexSkill || options.claudeCodeMCP) {
    return;
  }
  options.codexMCP = true;
  options.codexSkill = true;
  options.claudeCodeMCP = true;
}

async function installBinary(options) {
  const version = options.version === "latest" ? await withSpinner("Resolving latest deptrust release", latestVersion) : normalizeVersion(options.version);
  const target = platformTarget();
  const archiveName = `deptrust_${version}_${target.goos}_${target.goarch}.${target.extension}`;
  const url = `https://github.com/${repo}/releases/download/${version}/${archiveName}`;
  const workDir = join(tmpdir(), `deptrust-npm-${randomBytes(6).toString("hex")}`);
  const archivePath = join(workDir, archiveName);
  const extractDir = join(workDir, "extract");
  const binaryName = process.platform === "win32" ? "deptrust.exe" : "deptrust";

  mkdirSync(workDir, { recursive: true });
  mkdirSync(extractDir, { recursive: true });
  mkdirSync(options.binDir, { recursive: true });

  console.log(`Downloading ${url}`);
  await withSpinner(`Downloading ${archiveName}`, () => download(url, archivePath));
  console.log(`Extracting ${archiveName}`);
  run("tar", ["-xf", archivePath, "-C", extractDir]);

  const source = join(extractDir, archiveName.replace(`.${target.extension}`, ""), binaryName);
  const installPath = join(options.binDir, binaryName);
  if (!existsSync(source)) {
    throw new Error(`release archive did not contain ${binaryName}`);
  }
  copyFileSync(source, installPath);
  chmodSync(installPath, 0o755);

  if (options.check && !existsSync(installPath)) {
    throw new Error(`install check failed: ${installPath} does not exist`);
  }

  rmSync(workDir, { recursive: true, force: true });
  console.log(`Installed deptrust to ${installPath}`);
  return installPath;
}

async function confirmInstallPlan(options) {
  printInstallPlan(options);
  if (options.yes) {
    return;
  }
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log("Non-interactive shell detected; continuing without confirmation. Pass --yes to skip this message.");
    return;
  }

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    const answer = await rl.question("Continue? [y/N] ");
    if (!["y", "yes"].includes(answer.trim().toLowerCase())) {
      throw new Error("installation cancelled");
    }
  } finally {
    rl.close();
  }
}

async function maybePromptInstallOptions(options) {
  if (!options.guided) {
    return;
  }
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log("Non-interactive shell detected; installing binary only. Pass --all or specific integration flags to configure agents.");
    return;
  }

  console.log("deptrust guided install");
  console.log("  This installer does not modify files in the current project.");
  console.log("  It installs the deptrust binary first, then can configure optional user-level agent integrations.");
  console.log("  Choose optional agent integrations:");
  console.log("");

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    options.codexMCP = await confirm(rl, "Register Codex MCP server?", true);
    options.claudeCodeMCP = await confirm(rl, "Register Claude Code MCP server?", true);
    options.codexSkill = await confirm(rl, "Install Codex skill fallback?", true);
  } finally {
    rl.close();
  }
  console.log("");
}

async function confirm(rl, question, defaultValue) {
  const suffix = defaultValue ? "[Y/n]" : "[y/N]";
  const answer = (await rl.question(`${question} ${suffix} `)).trim().toLowerCase();
  if (answer === "") {
    return defaultValue;
  }
  return ["y", "yes"].includes(answer);
}

function printInstallPlan(options) {
  const binaryName = process.platform === "win32" ? "deptrust.exe" : "deptrust";
  const installPath = join(options.binDir, binaryName);
  console.log("deptrust will install/update user-level files:");
  console.log(`  Binary: ${installPath}`);
  if (options.codexSkill) {
    console.log(`  Codex skill: ${join(homedir(), ".agents", "skills", "deptrust-package-check")}`);
  }
  if (options.codexMCP) {
    console.log(`  Codex MCP: user/global Codex config -> ${installPath} mcp`);
  }
  if (options.claudeCodeMCP) {
    console.log(`  Claude Code MCP: user config -> ${installPath} mcp`);
  }
  console.log("");
}

async function confirmUninstallPlan(options) {
  printUninstallPlan(options);
  if (options.yes) {
    return;
  }
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log("Non-interactive shell detected; continuing without confirmation. Pass --yes to skip this message.");
    return;
  }

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    const answer = await rl.question("Remove these deptrust files/settings? [y/N] ");
    if (!["y", "yes"].includes(answer.trim().toLowerCase())) {
      throw new Error("uninstall cancelled");
    }
  } finally {
    rl.close();
  }
}

function printUninstallPlan(options) {
  const binaryName = process.platform === "win32" ? "deptrust.exe" : "deptrust";
  const installPath = join(options.binDir, binaryName);
  console.log("deptrust will remove user-level files/settings:");
  console.log(`  Binary: ${installPath}`);
  if (options.codexSkill) {
    console.log(`  Codex skill: ${join(homedir(), ".agents", "skills", "deptrust-package-check")}`);
  }
  if (options.codexMCP) {
    console.log("  Codex MCP: deptrust entry from user/global Codex config");
  }
  if (options.claudeCodeMCP) {
    console.log("  Claude Code MCP: deptrust entry from user config");
  }
  console.log("  Current project files: none");
  console.log("");
}

function uninstall(options) {
  const binaryName = process.platform === "win32" ? "deptrust.exe" : "deptrust";
  const installPath = join(options.binDir, binaryName);

  rmSync(installPath, { force: true });
  console.log(`Removed deptrust binary from ${installPath}`);

  if (options.codexSkill) {
    const skillPath = join(homedir(), ".agents", "skills", "deptrust-package-check");
    const source = join(packageRoot, ".agents", "skills", "deptrust-package-check");
    if (existsSync(skillPath) && !sameSkill(source, skillPath)) {
      const backup = `${skillPath}.bak-${Date.now()}`;
      renameSync(skillPath, backup);
      console.warn(`Skill at ${skillPath} differed from deptrust's; backed it up to ${backup} instead of deleting`);
    } else {
      rmSync(skillPath, { recursive: true, force: true });
      console.log(`Removed deptrust-package-check skill from ${skillPath}`);
    }
  }

  if (options.codexMCP) {
    if (commandExists("codex")) {
      runAllowFailure("codex", ["mcp", "remove", "deptrust"]);
      console.log("Removed deptrust MCP server from Codex if it existed");
    } else {
      console.warn("codex command not found; skipping Codex MCP removal");
    }
  }

  if (options.claudeCodeMCP) {
    if (commandExists("claude")) {
      runAllowFailure("claude", ["mcp", "remove", "deptrust", "--scope", "user"]);
      console.log("Removed deptrust MCP server from Claude Code if it existed");
    } else {
      console.warn("claude command not found; skipping Claude Code MCP removal");
    }
  }

  console.log("Uninstalled deptrust user-level setup.");
}

async function withSpinner(label, task) {
  if (!process.stderr.isTTY) {
    console.error(`${label}...`);
    return task();
  }

  const frames = ["-", "\\", "|", "/"];
  let index = 0;
  process.stderr.write(`${frames[index]} ${label}`);
  const timer = setInterval(() => {
    index = (index + 1) % frames.length;
    process.stderr.write(`\r${frames[index]} ${label}`);
  }, 100);

  try {
    const result = await task();
    clearInterval(timer);
    process.stderr.write(`\r[done] ${label}\n`);
    return result;
  } catch (error) {
    clearInterval(timer);
    process.stderr.write(`\r[failed] ${label}\n`);
    throw error;
  }
}

async function latestVersion() {
  const release = await getJSON(`https://api.github.com/repos/${repo}/releases/latest`);
  if (!release.tag_name) {
    throw new Error("GitHub latest release response did not include tag_name");
  }
  return release.tag_name;
}

function normalizeVersion(version) {
  return version.startsWith("v") ? version : `v${version}`;
}

function platformTarget() {
  const goos = {
    darwin: "darwin",
    linux: "linux",
    win32: "windows",
  }[process.platform];
  const goarch = {
    x64: "amd64",
    arm64: "arm64",
  }[process.arch];

  if (!goos || !goarch) {
    throw new Error(`unsupported platform: ${process.platform}/${process.arch}`);
  }

  return {
    goos,
    goarch,
    extension: goos === "windows" ? "zip" : "tar.gz",
  };
}

async function maybeInstallSkill(options) {
  if (!options.codexSkill) {
    return;
  }
  const source = join(packageRoot, ".agents", "skills", "deptrust-package-check");
  const target = join(homedir(), ".agents", "skills", "deptrust-package-check");
  if (!existsSync(join(source, "SKILL.md"))) {
    throw new Error("deptrust skill was not included in the npm package");
  }
  if (existsSync(target) && !sameSkill(source, target)) {
    const backup = `${target}.bak-${Date.now()}`;
    renameSync(target, backup);
    console.warn(`Existing skill at ${target} differed; backed it up to ${backup}`);
  } else {
    rmSync(target, { recursive: true, force: true });
  }
  mkdirSync(dirname(target), { recursive: true });
  cpSync(source, target, { recursive: true });
  console.log(`Installed deptrust-package-check skill to ${target}`);
}

function sameSkill(source, target) {
  try {
    return readFileSync(join(source, "SKILL.md"), "utf8") === readFileSync(join(target, "SKILL.md"), "utf8");
  } catch {
    return false;
  }
}

function maybeRegisterMCP(options, installPath) {
  if (options.codexMCP) {
    if (commandExists("codex")) {
      if (mcpServerRegistered("codex", ["mcp", "list"])) {
        console.warn("deptrust is already registered as a Codex MCP server; skipping. Re-run after `codex mcp remove deptrust` to replace it.");
      } else {
        run("codex", ["mcp", "add", "deptrust", "--", installPath, "mcp"]);
        console.log("Registered deptrust MCP server with Codex");
      }
    } else {
      console.warn("codex command not found; skipping Codex MCP registration");
    }
  }

  if (options.claudeCodeMCP) {
    if (commandExists("claude")) {
      if (mcpServerRegistered("claude", ["mcp", "list"])) {
        console.warn("deptrust is already registered as a Claude Code MCP server; skipping. Re-run after `claude mcp remove deptrust` to replace it.");
      } else {
        run("claude", ["mcp", "add", "--transport", "stdio", "--scope", "user", "deptrust", "--", installPath, "mcp"]);
        console.log("Registered deptrust MCP server with Claude Code");
      }
    } else {
      console.warn("claude command not found; skipping Claude Code MCP registration");
    }
  }
}

function mcpServerRegistered(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.status !== 0 || typeof result.stdout !== "string") {
    return false;
  }
  return result.stdout.split(/\r?\n/).some((line) => /(^|[^\w-])deptrust([^\w-]|$)/.test(line));
}

function commandExists(command) {
  const extensions = process.platform === "win32"
    ? (process.env.PATHEXT || ".EXE;.CMD;.BAT;.COM").split(";")
    : [""];
  for (const dir of (process.env.PATH || "").split(process.platform === "win32" ? ";" : ":")) {
    if (!dir) {
      continue;
    }
    for (const extension of extensions) {
      const candidate = join(dir, process.platform === "win32" && !command.toLowerCase().endsWith(extension.toLowerCase()) ? `${command}${extension}` : command);
      try {
        accessSync(candidate, constants.X_OK);
        return true;
      } catch {
        // Keep searching PATH.
      }
    }
  }
  return false;
}

function run(command, args) {
  const result = spawnSync(command, args, { stdio: "inherit" });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed`);
  }
}

function runAllowFailure(command, args) {
  spawnSync(command, args, { stdio: "inherit" });
}

function download(url, destination) {
  return new Promise((resolvePromise, rejectPromise) => {
    const file = createWriteStream(destination);
    https.get(url, requestHeaders(), (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        response.resume();
        file.close(() => {
          rmSync(destination, { force: true });
          download(response.headers.location, destination).then(resolvePromise, rejectPromise);
        });
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        file.close(() => {
          rmSync(destination, { force: true });
          rejectPromise(new Error(`download failed with HTTP ${response.statusCode}`));
        });
        return;
      }
      response.pipe(file);
      file.on("finish", () => {
        file.close(resolvePromise);
      });
    }).on("error", (error) => {
      file.close();
      rmSync(destination, { force: true });
      rejectPromise(error);
    });
  });
}

function getJSON(url) {
  return new Promise((resolvePromise, rejectPromise) => {
    https.get(url, requestHeaders(), (response) => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => {
        body += chunk;
      });
      response.on("end", () => {
        if (response.statusCode !== 200) {
          rejectPromise(new Error(`GitHub API returned HTTP ${response.statusCode}`));
          return;
        }
        try {
          resolvePromise(JSON.parse(body));
        } catch (error) {
          rejectPromise(error);
        }
      });
    }).on("error", rejectPromise);
  });
}

function requestHeaders() {
  return {
    headers: {
      "User-Agent": "deptrust-npm-installer",
      "Accept": "application/vnd.github+json",
    },
  };
}

function printNextSteps(installPath) {
  console.log(`
Next checks:
  ${installPath} check npm lodash latest
  ${installPath} mcp

Manual MCP config:
  command = "${installPath}"
  args = ["mcp"]
`);
}

function printUsage() {
  console.log(`
deptrust npm installer

Usage:
  npx @clidey/deptrust install [options]
  npx @clidey/deptrust uninstall [options]
  npx @clidey/deptrust skills install [options]
  npx @clidey/deptrust mcp install [options]

Options:
  --version VERSION       Release version to install. Default: latest
  --bin-dir DIR           Install directory. Default: ~/.local/bin
  --codex-mcp             Register Codex MCP
  --codex-skill           Install Codex skill fallback
  --claude-code-mcp       Register Claude Code MCP
  --all                   Install binary, Codex MCP, Codex skill, and Claude Code MCP
                          For uninstall, remove Codex MCP, Codex skill, and Claude Code MCP
  --check                 Verify the binary exists after install
  -y, --yes               Skip interactive confirmation

Examples:
  npx @clidey/deptrust install
  npx @clidey/deptrust install --yes
  npx @clidey/deptrust install --all
  npx @clidey/deptrust uninstall
  npx @clidey/deptrust uninstall --yes
  npx @clidey/deptrust skills install
  npx @clidey/deptrust mcp install --codex-mcp
`);
}
