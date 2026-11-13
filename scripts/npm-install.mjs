#!/usr/bin/env node

import { createWriteStream, existsSync, mkdirSync, chmodSync, copyFileSync, rmSync, renameSync, readFileSync, writeFileSync, accessSync, realpathSync, constants } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { tmpdir, homedir } from "node:os";
import { randomBytes } from "node:crypto";
import { createInterface } from "node:readline/promises";
import https from "node:https";

const repo = "clidey/deptrust";
const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

if (process.argv[1] && realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(`error: ${error.message}`);
    process.exit(1);
  });
}

async function main(args = process.argv.slice(2), install = installBinary) {
  if (args.length === 0 || args.includes("-h") || args.includes("--help")) {
    printUsage();
    return;
  }

  const command = args[0];
  if (command === "install" || command === "upgrade") {
    const options = parseOptions(args.slice(1));
    if (command === "upgrade") {
      options.guided = false;
      options.yes = true;
      options.upgrade = true;
    }
    await maybePromptInstallOptions(options);
    await confirmInstallPlan(options);
    await runInstallation(options, install);
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
    await runInstallation(options);
    return;
  }

  if (command === "mcp" && args[1] === "install") {
    const options = parseOptions(args.slice(2));
    if (!options.codexMCP && !options.claudeCodeMCP) {
      options.codexMCP = true;
      options.claudeCodeMCP = true;
    }
    await confirmInstallPlan(options);
    await runInstallation(options);
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
    githubAuth: false,
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

async function runInstallation(options, install = installBinary) {
  const installPath = await install(options);
  await maybeInstallSkill(options, installPath);
  maybeRegisterMCP(options, installPath);
  maybeInstallHook(options, installPath);
  let refreshed = true;
  try {
    run(installPath, ["setup", "--refresh"]);
  } catch {
    refreshed = false;
    console.warn(`deptrust was installed successfully, but existing agent settings could not be refreshed. Fix the configuration error above, then run:\n  ${shellQuote(installPath)} setup --refresh`);
  }
  if (options.upgrade) {
    console.log(refreshed
      ? "Upgrade complete. Your existing agent setup is ready; restart active agent sessions to reload running MCP servers."
      : "Binary upgrade complete. Agent settings still need refreshing.");
  } else {
    printNextSteps(installPath);
  }
  return installPath;
}

async function installBinary(options, fetchRelease = download) {
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
  await withSpinner(`Downloading ${archiveName}`, () => fetchRelease(url, archivePath));
  console.log(`Extracting ${archiveName}`);
  run("tar", ["-xf", archivePath, "-C", extractDir]);

  const source = join(extractDir, archiveName.replace(`.${target.extension}`, ""), binaryName);
  const installPath = resolve(options.binDir, binaryName);
  if (!existsSync(source)) {
    throw new Error(`release archive did not contain ${binaryName}`);
  }
  if (process.platform === "darwin") {
    // Re-sign downloaded Mach-O files locally. This protects users installing
    // an older release whose CI-built Darwin signature macOS may reject.
    run("codesign", ["--force", "--sign", "-", source]);
    run("codesign", ["--verify", "--strict", source]);
  }

  const checked = spawnSync(source, ["version"], { encoding: "utf8", timeout: 10_000 });
  if (checked.status !== 0 || checked.stdout?.split(/\r?\n/, 1)[0] !== `deptrust ${version}`) {
    throw new Error(`downloaded binary did not report deptrust ${version}; existing installation was left unchanged`);
  }
  // Replace the directory entry instead of truncating a running executable.
  const staged = join(options.binDir, `.deptrust-update-${randomBytes(6).toString("hex")}`);
  try {
    copyFileSync(source, staged);
    chmodSync(staged, 0o755);
    renameSync(staged, installPath);
  } finally {
    rmSync(staged, { force: true });
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
    if (options.hook && commandExists("gh")) {
      options.githubAuth = await confirm(rl, "Use your existing gh login for deptrust hook checks?", true);
    }
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
  console.log("  Existing deptrust integrations and installed skills: refreshed; disabled integrations stay disabled");
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

async function maybeInstallSkill(options, installPath) {
  if (options.codexSkill) {
    run(installPath, ["setup", "--skill"]);
  }
}

function sameSkill(source, target) {
  try {
    return readFileSync(join(source, "SKILL.md"), "utf8") === readFileSync(join(target, "SKILL.md"), "utf8");
  } catch {
    return false;
  }
}

function maybeRegisterMCP(options, installPath) {
  if (options.codexMCP || options.claudeCodeMCP) {
    const args = ["setup"];
    if (options.codexMCP) {
      args.push("--codex-mcp");
    }
    if (options.claudeCodeMCP) {
      args.push("--claude-code-mcp");
    }
    run(installPath, args);
  }
}

function maybeInstallHook(options, installPath) {
  if (!options.hook) {
    return;
  }
  installCodexHook(installPath, options.githubAuth);
  installClaudeHook(installPath, options.githubAuth);
}

function installCodexHook(installPath, useGitHubAuth) {
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
        command: `${useGitHubAuth ? "DEPTRUST_GITHUB_AUTH=gh " : ""}${shellQuote(installPath)} hook shell`,
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

function installClaudeHook(installPath, useGitHubAuth) {
  const settingsPath = claudeSettingsPath();
  const settings = readJSONFile(settingsPath);
  settings.hooks ||= {};
  let envChanged = false;
  if (useGitHubAuth) {
    settings.env ||= {};
    if (settings.env.DEPTRUST_GITHUB_AUTH !== "gh") {
      settings.env.DEPTRUST_GITHUB_AUTH = "gh";
      envChanged = true;
    }
  }
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
  if (JSON.stringify(current) === JSON.stringify(next) && !envChanged) {
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
  npx @clidey/deptrust@latest upgrade [options]
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
  --check                 Accepted for compatibility; every install verifies its binary
  -y, --yes               Skip interactive confirmation

Examples:
  npx @clidey/deptrust install
  npx @clidey/deptrust@latest upgrade
  npx @clidey/deptrust install --yes
  npx @clidey/deptrust install --all
  npx @clidey/deptrust install --hook
  npx @clidey/deptrust uninstall
  npx @clidey/deptrust uninstall --yes
  npx @clidey/deptrust skills install
  npx @clidey/deptrust mcp install --codex-mcp
`);
}

export { installBinary, main, parseOptions, runInstallation };
