import test, { after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, copyFileSync, readdirSync, existsSync, symlinkSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import { installBinary, main, parseOptions, runInstallation } from "./npm-install.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const goCache = JSON.parse(execFileSync("go", ["env", "-json", "GOCACHE", "GOMODCACHE"], { cwd: root, encoding: "utf8" }));
const fixture = mkdtempSync(join(tmpdir(), "deptrust-upgrade-fixture-"));
after(() => rmSync(fixture, { recursive: true, force: true }));
const binary = join(fixture, process.platform === "win32" ? "deptrust.exe" : "deptrust");
execFileSync("go", ["build", "-ldflags", "-X github.com/clidey/deptrust/internal/buildinfo.Version=v1.2.3", "-o", binary, "./cmd/deptrust"], { cwd: root });

function isolatedHome(t) {
  const home = mkdtempSync(join(tmpdir(), "deptrust-upgrade-home-"));
  const previous = { HOME: process.env.HOME, USERPROFILE: process.env.USERPROFILE };
  process.env.HOME = home;
  process.env.USERPROFILE = home;
  t.after(() => {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete process.env[key]; else process.env[key] = value;
    }
    rmSync(home, { recursive: true, force: true });
  });
  return home;
}

function write(path, text) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, text);
}

// Exercise the actual extraction, version verification, replacement and setup
// subprocesses. Only the release download is substituted; no network is used.
function fixtureDownload(version) {
  return async (url, destination) => {
    const archive = new URL(url).pathname.split("/").at(-1);
    const folder = archive.replace(/\.(tar\.gz|zip)$/, "");
    const staging = mkdtempSync(join(fixture, "archive-"));
    mkdirSync(join(staging, folder));
    copyFileSync(binary, join(staging, folder, process.platform === "win32" ? "deptrust.exe" : "deptrust"));
    assert.ok(url.includes(`/download/${version}/`));
    execFileSync("tar", ["-czf", destination, "-C", staging, folder]);
  };
}

test("upgrade replaces the binary and refreshes only existing integrations", async (t) => {
  if (process.platform === "win32") return t.skip("fixture archive currently exercises tar.gz release platforms");
  const home = isolatedHome(t);
  const options = parseOptions(["--version", "1.2.3", "--bin-dir", join(home, "bin"), "--yes"]);
  const installed = join(options.binDir, "deptrust");
  write(installed, "old binary fixture");
  const skill = join(home, ".agents", "skills", "deptrust-package-check", "SKILL.md");
  write(skill, "previous skill with a local customization\n");
  const codexPath = join(home, ".codex", "config.toml");
  write(codexPath, "# preserve\n[mcp_servers.deptrust]\ncommand = '/stale/deptrust'\nargs = ['mcp']\n[mcp_servers.deptrust.env]\nEXAMPLE = 'keep'\n");
  const hooksPath = join(home, ".codex", "hooks.json");
  write(hooksPath, JSON.stringify({ hooks: { PreToolUse: [{ matcher: "Bash", hooks: [
    { type: "command", command: "DEPTRUST_GITHUB_AUTH=gh '/stale/deptrust' hook shell", timeout: 45 },
    { type: "command", command: "other-hook" },
  ] }] } }));
  const install = (o) => installBinary(o, fixtureDownload("v1.2.3"));
  const upgrade = () => main(["upgrade", "--version", "1.2.3", "--bin-dir", options.binDir], install);
  await upgrade();
  assert.match(execFileSync(installed, ["version"], { encoding: "utf8" }), /^deptrust v1\.2\.3\n/);
  assert.ok(readFileSync(codexPath, "utf8").includes(installed));
  assert.ok(readFileSync(codexPath, "utf8").includes("# preserve"));
  assert.ok(readFileSync(codexPath, "utf8").includes("EXAMPLE = 'keep'"));
  const hooks = JSON.parse(readFileSync(hooksPath)).hooks.PreToolUse;
  assert.equal(hooks.length, 1);
  assert.equal(hooks[0].hooks.length, 2);
  assert.ok(hooks[0].hooks[0].command.includes(installed));
  assert.match(hooks[0].hooks[0].command, /^DEPTRUST_GITHUB_AUTH=gh /);
  assert.equal(hooks[0].hooks[0].timeout, 45);
  assert.equal(hooks[0].hooks[1].command, "other-hook");
  assert.equal(existsSync(join(home, ".claude.json")), false);
  assert.equal(existsSync(join(home, ".claude", "settings.json")), false);
  assert.equal(readFileSync(skill, "utf8"), readFileSync(join(root, ".agents", "skills", "deptrust-package-check", "SKILL.md"), "utf8"));
  const backups = readdirSync(dirname(dirname(skill))).filter((x) => x.startsWith("deptrust-package-check.bak-"));
  assert.equal(backups.length, 1);
  assert.match(readFileSync(join(dirname(dirname(skill)), backups[0], "SKILL.md"), "utf8"), /local customization/);
  const before = [readFileSync(codexPath, "utf8"), readFileSync(hooksPath, "utf8")];
  const runningMCP = spawn(installed, ["mcp"], { stdio: ["pipe", "ignore", "ignore"] });
  t.after(() => { runningMCP.stdin.end(); runningMCP.kill(); });
  await new Promise((resolve, reject) => { runningMCP.once("spawn", resolve); runningMCP.once("error", reject); });
  await upgrade();
  assert.equal(runningMCP.exitCode, null, "upgrade interrupted the running MCP process");
  assert.deepEqual([readFileSync(codexPath, "utf8"), readFileSync(hooksPath, "utf8")], before);
  assert.equal(readdirSync(dirname(dirname(skill))).filter((x) => x.startsWith("deptrust-package-check.bak-")).length, 1);
  // Existing registration invokes the upgraded hook, including the redirect fix.
  const response = JSON.parse(execFileSync(installed, ["hook", "tool"], {
    input: JSON.stringify({ tool_input: { command: "pnpm install 2>&1" } }), encoding: "utf8",
  })).hookSpecificOutput;
  assert.equal(response.permissionDecision, "deny");
  assert.match(response.permissionDecisionReason, /Retry/);
});

test("wrong release version leaves the installed binary untouched", async (t) => {
  if (process.platform === "win32") return t.skip("tar.gz fixture");
  const home = isolatedHome(t);
  const options = parseOptions(["--version", "9.9.9", "--bin-dir", join(home, "bin"), "--yes"]);
  const installed = join(options.binDir, "deptrust");
  write(installed, "keep this installed binary");
  await assert.rejects(installBinary(options, fixtureDownload("v9.9.9")), /did not report deptrust v9\.9\.9/);
  assert.equal(readFileSync(installed, "utf8"), "keep this installed binary");
});

test("binary-only install does not enable integrations", async (t) => {
  if (process.platform === "win32") return t.skip("tar.gz fixture");
  const home = isolatedHome(t);
  const options = parseOptions(["--version", "1.2.3", "--bin-dir", join(home, "bin"), "--yes"]);
  await runInstallation(options, (o) => installBinary(o, fixtureDownload("v1.2.3")));
  assert.deepEqual(readdirSync(home), ["bin"]);
});

test("npm bin symlink still runs the installer entrypoint", (t) => {
  if (process.platform === "win32") return t.skip("npm uses command shims on Windows");
  const link = join(mkdtempSync(join(fixture, "bin-")), "deptrust");
  symlinkSync(join(root, "scripts", "npm-install.mjs"), link);
  const help = execFileSync(process.execPath, [link, "--help"], { encoding: "utf8" });
  assert.match(help, /deptrust@latest upgrade/);
});

test("standalone binary installs and refreshes skills without npm or repository files", (t) => {
  const home = isolatedHome(t);
  const env = { ...process.env, PATH: "" };
  const skill = join(home, ".agents", "skills", "deptrust-package-check", "SKILL.md");
  execFileSync(binary, ["setup", "--refresh"], { cwd: home, env });
  assert.equal(existsSync(skill), false);
  execFileSync(binary, ["setup", "--skill"], { cwd: home, env });
  const canonical = readFileSync(join(root, ".agents", "skills", "deptrust-package-check", "SKILL.md"), "utf8");
  assert.equal(readFileSync(skill, "utf8"), canonical);
  writeFileSync(skill, "older skill with customization");
  execFileSync(binary, ["setup", "--refresh"], { cwd: home, env });
  assert.equal(readFileSync(skill, "utf8"), canonical);
  const parent = dirname(dirname(skill));
  const backups = readdirSync(parent).filter((x) => x.startsWith("deptrust-package-check.bak-"));
  assert.equal(backups.length, 1);
  assert.equal(readFileSync(join(parent, backups[0], "SKILL.md"), "utf8"), "older skill with customization");
});

test("automatic npm refresh failure warns without failing the binary upgrade", async (t) => {
  if (process.platform === "win32") return t.skip("tar.gz fixture");
  const home = isolatedHome(t);
  const config = join(home, ".codex", "config.toml");
  write(config, "[invalid configuration");
  const warnings = [];
  const messages = [];
  t.mock.method(console, "warn", (s) => warnings.push(s));
  t.mock.method(console, "log", (s) => messages.push(s));
  const installed = join(home, "bin", "deptrust");
  await main(["upgrade", "--version", "1.2.3", "--bin-dir", dirname(installed)],
    (o) => installBinary(o, fixtureDownload("v1.2.3")));
  assert.match(execFileSync(installed, ["version"], { encoding: "utf8" }), /^deptrust v1\.2\.3\n/);
  assert.equal(readFileSync(config, "utf8"), "[invalid configuration");
  assert.ok(warnings.some((s) => s.includes(installed) && s.includes("setup --refresh")));
  assert.ok(messages.some((s) => s.includes("Agent settings still need refreshing")));
  assert.equal(messages.some((s) => s.includes("Your existing agent setup is ready")), false);
});

test("source installer refreshes existing skills and keeps install success on refresh failure", async (t) => {
  if (process.platform === "win32") return t.skip("Bash source installer");
  const home = isolatedHome(t);
  const skill = join(home, ".agents", "skills", "deptrust-package-check", "SKILL.md");
  write(skill, "previous source-install skill");
  const binDir = join(home, "bin with spaces");
  const env = { ...process.env, ...goCache, GOPROXY: "off" };
  const args = [join(root, "scripts", "install.sh"), "--bin-dir", binDir, "--check"];
  const output = execFileSync("bash", args, { cwd: home, env, encoding: "utf8", timeout: 60_000 });
  assert.match(output, /Existing deptrust integrations are up to date/);
  const canonical = readFileSync(join(root, ".agents", "skills", "deptrust-package-check", "SKILL.md"), "utf8");
  assert.equal(readFileSync(skill, "utf8"), canonical);
  assert.equal(existsSync(join(home, ".codex")), false);
  assert.equal(existsSync(join(home, ".claude.json")), false);
  const installed = join(binDir, "deptrust");
  const runningMCP = spawn(installed, ["mcp"], { stdio: ["pipe", "ignore", "ignore"] });
  t.after(() => { runningMCP.stdin.end(); runningMCP.kill(); });
  await new Promise((resolve, reject) => { runningMCP.once("spawn", resolve); runningMCP.once("error", reject); });
  const config = join(home, ".codex", "config.toml");
  write(config, "[invalid configuration");
  const result = spawnSync("bash", args, { cwd: home, env, encoding: "utf8", timeout: 60_000 });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Installed deptrust/);
  assert.match(result.stderr, /installed successfully, but existing agent settings could not be refreshed/);
  assert.match(result.stderr, /setup --refresh/);
  assert.equal(readFileSync(config, "utf8"), "[invalid configuration");
  assert.match(execFileSync(installed, ["version"], { encoding: "utf8" }), /^deptrust /);
  assert.equal(runningMCP.exitCode, null);
});
