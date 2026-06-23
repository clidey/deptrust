#!/usr/bin/env node

import { createWriteStream, existsSync, mkdirSync, chmodSync, copyFileSync, rmSync, cpSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { tmpdir, homedir } from "node:os";
import { randomBytes } from "node:crypto";
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
    const installPath = await installBinary(options);
    await maybeInstallSkill(options);
    maybeRegisterMCP(options, installPath);
    printNextSteps(installPath);
    return;
  }

  if (command === "skills" && args[1] === "install") {
    const options = parseOptions(args.slice(2));
    options.codexSkill = true;
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
      default:
        throw new Error(`unknown option: ${arg}`);
    }
  }

  return options;
}

function requiredValue(args, index, flag) {
  if (index >= args.length || args[index].startsWith("--")) {
    throw new Error(`${flag} requires a value`);
  }
  return args[index];
}

async function installBinary(options) {
  const version = options.version === "latest" ? await latestVersion() : normalizeVersion(options.version);
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
  await download(url, archivePath);
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
  rmSync(target, { recursive: true, force: true });
  mkdirSync(dirname(target), { recursive: true });
  cpSync(source, target, { recursive: true });
  console.log(`Installed deptrust-package-check skill to ${target}`);
}

function maybeRegisterMCP(options, installPath) {
  if (options.codexMCP) {
    if (commandExists("codex")) {
      run("codex", ["mcp", "add", "deptrust", "--", installPath, "mcp"]);
      console.log("Registered deptrust MCP server with Codex");
    } else {
      console.warn("codex command not found; skipping Codex MCP registration");
    }
  }

  if (options.claudeCodeMCP) {
    if (commandExists("claude")) {
      run("claude", ["mcp", "add", "--transport", "stdio", "--scope", "user", "deptrust", "--", installPath, "mcp"]);
      console.log("Registered deptrust MCP server with Claude Code");
    } else {
      console.warn("claude command not found; skipping Claude Code MCP registration");
    }
  }
}

function commandExists(command) {
  const check = process.platform === "win32" ? "where" : "command";
  const args = process.platform === "win32" ? [command] : ["-v", command];
  const result = spawnSync(check, args, { stdio: "ignore", shell: process.platform !== "win32" });
  return result.status === 0;
}

function run(command, args) {
  const result = spawnSync(command, args, { stdio: "inherit" });
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed`);
  }
}

function download(url, destination) {
  return new Promise((resolvePromise, rejectPromise) => {
    const file = createWriteStream(destination);
    https.get(url, requestHeaders(), (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        file.close();
        rmSync(destination, { force: true });
        download(response.headers.location, destination).then(resolvePromise, rejectPromise);
        return;
      }
      if (response.statusCode !== 200) {
        file.close();
        rmSync(destination, { force: true });
        rejectPromise(new Error(`download failed with HTTP ${response.statusCode}`));
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
  npx @clidey/deptrust skills install [options]
  npx @clidey/deptrust mcp install [options]

Options:
  --version VERSION       Release version to install. Default: latest
  --bin-dir DIR           Install directory. Default: ~/.local/bin
  --codex-mcp             Register Codex MCP
  --codex-skill           Install Codex skill fallback
  --claude-code-mcp       Register Claude Code MCP
  --all                   Install binary, Codex MCP, Codex skill, and Claude Code MCP
  --check                 Verify the binary exists after install

Examples:
  npx @clidey/deptrust install
  npx @clidey/deptrust install --all
  npx @clidey/deptrust skills install
  npx @clidey/deptrust mcp install --codex-mcp
`);
}
