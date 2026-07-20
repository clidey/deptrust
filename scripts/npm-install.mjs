#!/usr/bin/env node

import { createWriteStream, existsSync, mkdirSync, chmodSync, copyFileSync, rmSync, cpSync, renameSync, readFileSync, writeFileSync, accessSync, constants } from "node:fs";
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
    maybeInstallHook(options, installPath);
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
    maybeInstallHook(options, installPath);
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
    hook: false,
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
      case "--hook":
      case "--claude-code-hook":
        options.hook = true;
        break;
      case "--all":
        options.codexMCP = true;
        options.codexSkill = true;
        options.claudeCodeMCP = true;
        options.hook = true;
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

  options.guided = !options.yes && !options.codexMCP && !options.codexSkill && !options.claudeCodeMCP && !options.hook;
  return options;
}

function requiredValue(args, index, flag) {
  if (index >= args.length || args[index].startsWith("--")) {
    throw new Error(`${flag} requires a value`);
  }
  return args[index];
}

function applyDefaultUninstallSelection(options) {
  if (options.codexMCP || options.codexSkill || options.claudeCodeMCP || options.hook) {
    return;
  }
  options.codexMCP = true;
  options.codexSkill = true;
  options.claudeCodeMCP = true;
  options.hook = true;
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
  if (process.platform === "darwin") {
    // Re-sign downloaded Mach-O files locally. This protects users installing
    // an older release whose CI-built Darwin signature macOS may reject.
    run("codesign", ["--force", "--sign", "-", source]);
    run("codesign", ["--verify", "--strict", source]);
  }

  copyFileSync(source, installPath);
  chmodSync(installPath, 0o755);

  if (options.check && !existsSync(installPath)) {
    throw new Error(`install check failed: ${installPath} does not exist`);
  }

  rmSync(workDir, { recursive: true, force: true });
  console.log(`Installed deptrust to ${installPath}.`);
  return installPath;
}

async function confirmInstallPlan(options) {
  printInstallPlan(options);
  if (options.yes) {
    return;
  }
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log("No interactive terminal here, so we'll go ahead without asking. Pass --yes to silence this notice.");
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
    console.log("No interactive terminal here, so we'll just install the binary. Pass --all or specific flags to set up agent integrations too.");
    return;
  }

  console.log("Let's set up deptrust.");
  console.log("  Nothing in your current project will be touched.");
  console.log("  We'll install the binary first, then you can pick which agent integrations to add.");
  console.log("");

  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try {
    options.codexMCP = await confirm(rl, "Register Codex MCP server?", true);
    options.claudeCodeMCP = await confirm(rl, "Register Claude Code MCP server?", true);
    options.codexSkill = await confirm(rl, "Install Codex skill fallback?", true);
    options.hook = await confirm(rl, "Install dependency safety hooks for Codex and Claude Code?", true);
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
  console.log("Here's what will be installed or updated (all user-level, nothing in your project):");
  console.log(`  Binary: ${installPath}`);
  if (options.codexSkill) {
    console.log(`  Codex skill: ${join(homedir(), ".agents", "skills", "deptrust-package-check")}`);
  }
  if (options.codexMCP) {
    console.log(`  Codex MCP: added to your Codex config -> ${installPath} mcp`);
  }
  if (options.claudeCodeMCP) {
    console.log(`  Claude Code MCP: added to your Claude Code config -> ${installPath} mcp`);
  }
  if (options.hook) {
    console.log(`  Codex hook: package installs and GitHub Actions edits checked before tools run -> ${installPath} hook shell`);
    console.log(`  Claude Code hook: package installs and GitHub Actions edits checked before tools run -> ${installPath} hook shell`);
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
  console.log("Here's what will be removed (all user-level, nothing in your project):");
  console.log(`  Binary: ${installPath}`);
  if (options.codexSkill) {
    console.log(`  Codex skill: ${join(homedir(), ".agents", "skills", "deptrust-package-check")}`);
  }
  if (options.codexMCP) {
    console.log("  Codex MCP: the deptrust entry in your Codex config");
  }
  if (options.claudeCodeMCP) {
    console.log("  Claude Code MCP: the deptrust entry in your Claude Code config");
  }
  if (options.hook) {
    console.log("  Codex hook: the deptrust PreToolUse dependency hook in your Codex user hooks");
    console.log("  Claude Code hook: the deptrust PreToolUse dependency hook in your Claude Code user settings");
  }
  console.log("");
}

function uninstall(options) {
  const binaryName = process.platform === "win32" ? "deptrust.exe" : "deptrust";
  const installPath = join(options.binDir, binaryName);

  rmSync(installPath, { force: true });
  console.log(`Removed the deptrust binary from ${installPath}.`);

  if (options.codexSkill) {
    const skillPath = join(homedir(), ".agents", "skills", "deptrust-package-check");
    const source = join(packageRoot, ".agents", "skills", "deptrust-package-check");
    if (existsSync(skillPath) && !sameSkill(source, skillPath)) {
      const backup = `${skillPath}.bak-${Date.now()}`;
      renameSync(skillPath, backup);
      console.warn(`The skill at ${skillPath} looks customized, so we kept it safe at ${backup} rather than deleting it.`);
    } else {
      rmSync(skillPath, { recursive: true, force: true });
      console.log(`Removed the deptrust-package-check skill from ${skillPath}.`);
    }
  }

  if (options.codexMCP) {
    if (commandExists("codex")) {
      if (mcpServerRegistered("codex", ["mcp", "list"])) {
        runAllowFailure("codex", ["mcp", "remove", "deptrust"]);
        console.log("Removed the deptrust MCP server from Codex.");
      } else {
        console.log("Couldn't find a deptrust entry in Codex, so there was nothing to remove.");
      }
    } else {
      console.warn("Couldn't find the codex command, so we left Codex's MCP config alone.");
    }
  }

  if (options.claudeCodeMCP) {
    if (commandExists("claude")) {
      if (mcpServerRegistered("claude", ["mcp", "list"])) {
        runAllowFailure("claude", ["mcp", "remove", "deptrust", "--scope", "user"]);
        console.log("Removed the deptrust MCP server from Claude Code.");
      } else {
        console.log("Couldn't find a deptrust entry in Claude Code, so there was nothing to remove.");
      }
    } else {
      console.warn("Couldn't find the claude command, so we left Claude Code's MCP config alone.");
    }
  }

  if (options.hook) {
    removeCodexHook();
    removeClaudeHook();
  }

  console.log("All done. deptrust's user-level files are gone, and your project files were left untouched.");
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
  if (existsSync(target) && sameSkill(source, target)) {
    return;
  }
  if (existsSync(target)) {
    const backup = `${target}.bak-${Date.now()}`;
    renameSync(target, backup);
    console.warn(`Found a customized skill at ${target}, so we saved it to ${backup} before installing ours.`);
  } else {
    rmSync(target, { recursive: true, force: true });
  }
  mkdirSync(dirname(target), { recursive: true });
  cpSync(source, target, { recursive: true });
  console.log(`Updated the deptrust-package-check skill at ${target}.`);
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
      const status = mcpServerStatus("codex", installPath);
      if (status === "missing") {
        run("codex", ["mcp", "add", "deptrust", "--", installPath, "mcp"]);
        console.log("Connected deptrust to Codex.");
      } else if (status === "changed") {
        runAllowFailure("codex", ["mcp", "remove", "deptrust"]);
        run("codex", ["mcp", "add", "deptrust", "--", installPath, "mcp"]);
        console.log("Updated the deptrust Codex MCP configuration.");
      }
    } else {
      console.warn("Couldn't find the codex command, so we skipped the Codex setup. Install Codex first if you'd like it connected.");
    }
  }

  if (options.claudeCodeMCP) {
    if (commandExists("claude")) {
      const status = mcpServerStatus("claude", installPath);
      if (status === "missing") {
        run("claude", ["mcp", "add", "--transport", "stdio", "--scope", "user", "deptrust", "--", installPath, "mcp"]);
        console.log("Connected deptrust to Claude Code.");
      } else if (status === "changed") {
        runAllowFailure("claude", ["mcp", "remove", "deptrust", "--scope", "user"]);
        run("claude", ["mcp", "add", "--transport", "stdio", "--scope", "user", "deptrust", "--", installPath, "mcp"]);
        console.log("Updated the deptrust Claude Code MCP configuration.");
      }
    } else {
      console.warn("Couldn't find the claude command, so we skipped the Claude Code setup. Install Claude Code first if you'd like it connected.");
    }
  }
}

function maybeInstallHook(options, installPath) {
  if (!options.hook) {
    return;
  }
  installCodexHook(installPath);
  installClaudeHook(installPath);
}

function installCodexHook(installPath) {
  const hooksPath = codexHooksPath();
  const config = readJSONFile(hooksPath);
  config.hooks ||= {};
  const current = config.hooks.PreToolUse || [];
  const next = removeDeptrustHookEntries(current);
  next.push({
    matcher: "Bash|apply_patch|Edit|Write|MultiEdit",
    hooks: [
      {
        type: "command",
        command: `${shellQuote(installPath)} hook shell`,
        statusMessage: "Checking dependency safety with deptrust",
      },
    ],
  });
  if (JSON.stringify(current) === JSON.stringify(next)) {
    return;
  }
  config.hooks.PreToolUse = next;
  writeJSONFile(hooksPath, config);
  console.log(`Updated the deptrust Codex shell hook in ${hooksPath}.`);
}

function installClaudeHook(installPath) {
  const settingsPath = claudeSettingsPath();
  const settings = readJSONFile(settingsPath);
  settings.hooks ||= {};
  const current = settings.hooks.PreToolUse || [];
  const next = removeDeptrustHookEntries(current);
  next.push({
    matcher: "Bash|Edit|Write|MultiEdit",
    hooks: [
      {
        type: "command",
        command: installPath,
        args: ["hook", "shell"],
      },
    ],
  });
  if (JSON.stringify(current) === JSON.stringify(next)) {
    return;
  }
  settings.hooks.PreToolUse = next;
  writeJSONFile(settingsPath, settings);
  console.log(`Updated the deptrust Claude Code shell hook in ${settingsPath}.`);
}

function removeCodexHook() {
  const hooksPath = codexHooksPath();
  if (!existsSync(hooksPath)) {
    console.log("Codex hooks were not found, so there was no deptrust hook to remove.");
    return;
  }
  const config = readJSONFile(hooksPath);
  if (!config.hooks || !Array.isArray(config.hooks.PreToolUse)) {
    console.log("Couldn't find a deptrust Codex hook, so there was nothing to remove.");
    return;
  }
  const next = removeDeptrustHookEntries(config.hooks.PreToolUse);
  if (next.length === config.hooks.PreToolUse.length) {
    console.log("Couldn't find a deptrust Codex hook, so there was nothing to remove.");
    return;
  }
  config.hooks.PreToolUse = next;
  if (config.hooks.PreToolUse.length === 0) {
    delete config.hooks.PreToolUse;
  }
  if (Object.keys(config.hooks).length === 0) {
    delete config.hooks;
  }
  writeJSONFile(hooksPath, config);
  console.log(`Removed the deptrust Codex shell hook from ${hooksPath}.`);
}

function removeClaudeHook() {
  const settingsPath = claudeSettingsPath();
  if (!existsSync(settingsPath)) {
    console.log("Claude Code settings were not found, so there was no deptrust hook to remove.");
    return;
  }
  const settings = readJSONFile(settingsPath);
  if (!settings.hooks || !Array.isArray(settings.hooks.PreToolUse)) {
    console.log("Couldn't find a deptrust Claude Code hook, so there was nothing to remove.");
    return;
  }
  const next = removeDeptrustHookEntries(settings.hooks.PreToolUse);
  if (next.length === settings.hooks.PreToolUse.length) {
    console.log("Couldn't find a deptrust Claude Code hook, so there was nothing to remove.");
    return;
  }
  settings.hooks.PreToolUse = next;
  if (settings.hooks.PreToolUse.length === 0) {
    delete settings.hooks.PreToolUse;
  }
  if (Object.keys(settings.hooks).length === 0) {
    delete settings.hooks;
  }
  writeJSONFile(settingsPath, settings);
  console.log(`Removed the deptrust Claude Code shell hook from ${settingsPath}.`);
}

function removeDeptrustHookEntries(groups) {
  return groups
    .map((group) => {
      if (!Array.isArray(group.hooks)) {
        return group;
      }
      return {
        ...group,
        hooks: group.hooks.filter((item) => !isDeptrustHook(item)),
      };
    })
    .filter((group) => Array.isArray(group.hooks) ? group.hooks.length > 0 : true);
}

function isDeptrustHook(item) {
  if (!item || item.type !== "command") {
    return false;
  }
  if (Array.isArray(item.args) && item.args[0] === "hook" && (item.args[1] === "shell" || item.args[1] === "tool")) {
    return true;
  }
  return typeof item.command === "string" && /\bdeptrust(?:\.exe)?['"]?\s+hook\s+(shell|tool)\b/.test(item.command);
}

function codexHooksPath() {
  return join(homedir(), ".codex", "hooks.json");
}

function claudeSettingsPath() {
  return join(homedir(), ".claude", "settings.json");
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\"'\"'")}'`;
}

function readJSONFile(path) {
  if (!existsSync(path)) {
    return {};
  }
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    throw new Error(`could not parse ${path}: ${error.message}`);
  }
}

function writeJSONFile(path, value) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`);
}

function mcpServerRegistered(command, args) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (result.status !== 0 || typeof result.stdout !== "string") {
    return false;
  }
  return result.stdout.split(/\r?\n/).some((line) => /(^|[^\w-])deptrust([^\w-]|$)/.test(line));
}

function mcpServerStatus(command, installPath) {
  if (!mcpServerRegistered(command, ["mcp", "list"])) {
    return "missing";
  }

  // `get` is supported by the current Codex and Claude CLIs. If an older CLI
  // cannot describe an existing server, leave it alone rather than requiring
  // a destructive remove/re-add that the installer cannot verify.
  const result = spawnSync(command, ["mcp", "get", "deptrust"], { encoding: "utf8" });
  if (result.status !== 0 || typeof result.stdout !== "string") {
    return "unchanged";
  }
  const commandMatch = result.stdout.match(/^\s*command:\s*(.+)$/im);
  const argsMatch = result.stdout.match(/^\s*args:\s*(.+)$/im);
  if (!commandMatch || !argsMatch) {
    return "unchanged";
  }
  const configuredCommand = commandMatch[1].trim();
  const configuredArgs = argsMatch[1].trim();
  return configuredCommand === installPath && configuredArgs === "mcp" ? "unchanged" : "changed";
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
You're all set. A couple of things to try:
  ${installPath} check npm lodash latest
  ${installPath} mcp

If you'd rather wire up MCP by hand, point your client at:
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
  --hook                  Install Codex and Claude Code dependency safety hooks
  --all                   Install binary, Codex MCP, Codex skill, Claude Code MCP, and hook
                          For uninstall, remove Codex MCP, Codex skill, Claude Code MCP, and hook
  --check                 Verify the binary exists after install
  -y, --yes               Skip interactive confirmation

Examples:
  npx @clidey/deptrust install
  npx @clidey/deptrust install --yes
  npx @clidey/deptrust install --all
  npx @clidey/deptrust install --hook
  npx @clidey/deptrust uninstall
  npx @clidey/deptrust uninstall --yes
  npx @clidey/deptrust skills install
  npx @clidey/deptrust mcp install --codex-mcp
`);
}
