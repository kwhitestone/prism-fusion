import assert from "node:assert/strict";
import test from "node:test";
import { mkdtemp, mkdir, writeFile, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { validateWorkspaceLock, verifyWorkspace } from "./verify.mjs";
import * as verification from "./verify.mjs";

const lock = (commit, dependency = {}) => ({
  version: 1,
  dependencies: [{
    name: "prism-fusion", repository: "kwhitestone/prism-fusion", commit,
    role: "verification", ...dependency
  }]
});
const git = (cwd, ...args) => {
  const result = spawnSync("git", args, { cwd, encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim();
};

test("build stages cannot change the original pinned inputs or rewrite the lock to hide changes", async () => {
  assert.equal(typeof verification.verifyWorkspaceUnchanged, "function");
  const root = await mkdtemp(path.join(tmpdir(), "prism-workspace-stages-"));
  try {
    const project = path.join(root, "consumer");
    const dependency = path.join(root, "prism-fusion");
    await mkdir(project); await mkdir(dependency);
    git(dependency, "init", "-q");
    git(dependency, "config", "user.name", "Plugin acceptance");
    git(dependency, "config", "user.email", "acceptance@example.invalid");
    await writeFile(path.join(dependency, "runtime.txt"), "reviewed runtime\n");
    git(dependency, "add", "runtime.txt"); git(dependency, "commit", "-qm", "test: pin original runtime");
    const initial = git(dependency, "rev-parse", "HEAD");
    await writeFile(path.join(project, "workspace.lock.json"), JSON.stringify(lock(initial)));
    const original = JSON.stringify(await verifyWorkspace(project, { requireFramework: true }));
    await verification.verifyWorkspaceUnchanged(project, original);
    await writeFile(path.join(dependency, "runtime.txt"), "changed by a build stage\n");
    await assert.rejects(verification.verifyWorkspaceUnchanged(project, original), /uncommitted/i);
    git(dependency, "add", "runtime.txt"); git(dependency, "commit", "-qm", "test: mutate runtime during build");
    await assert.rejects(verification.verifyWorkspaceUnchanged(project, original), /commit mismatch/i);
    const changed = git(dependency, "rev-parse", "HEAD");
    await writeFile(path.join(project, "workspace.lock.json"), JSON.stringify(lock(changed)));
    await assert.rejects(verification.verifyWorkspaceUnchanged(project, original), /changed.*verification|original.*pin/i);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("dependency lock accepts only explicit immutable commits and safe sibling names", () => {
  assert.doesNotThrow(() => validateWorkspaceLock(lock("a".repeat(40))));
  for (const entry of [
    { commit: "master" }, { name: "../prism-fusion" }, { name: "/tmp/framework" },
    { repository: "https://token@github.com/org/repo" }, { commit: "" }
  ]) assert.throws(() => validateWorkspaceLock({ ...lock("a".repeat(40)), dependencies: [{ ...lock("a".repeat(40)).dependencies[0], ...entry }] }));
  assert.throws(() => validateWorkspaceLock({ version: 2, dependencies: [] }));
  assert.throws(() => validateWorkspaceLock({ version: 1, dependencies: [...lock("a".repeat(40)).dependencies, ...lock("b".repeat(40)).dependencies] }), /duplicate/i);
  assert.throws(() => validateWorkspaceLock(lock("a".repeat(40), { role: "runtime" })), /binding/i);
  assert.doesNotThrow(() => validateWorkspaceLock(lock("a".repeat(40), {
    role: "runtime",
    goModules: [{ module: "github.com/kwhitestone/prism-fusion", path: "src/server" }],
    packages: [{ package: "@prism-fusion/plugin-runtime", path: "src/web/src/plugin", sharedPeers: ["vue", "vue-router"] }]
  })));
  assert.throws(() => validateWorkspaceLock(lock("a".repeat(40), {
    role: "runtime", goModules: [{ module: "example.test/runtime", path: "../outside" }]
  })), /path/i);
  assert.throws(() => validateWorkspaceLock(lock("a".repeat(40), {
    repository: "attacker/lookalike"
  })), /official.*repository|repository.*prism/i);
});

test("verification checks actual dependency HEAD and dirty files, not just the lock text", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "prism-workspace-lock-"));
  try {
    const project = path.join(root, "consumer");
    const dependency = path.join(root, "prism-fusion");
    await mkdir(project); await mkdir(dependency);
    git(dependency, "init", "-q");
    git(dependency, "config", "user.name", "Plugin acceptance");
    git(dependency, "config", "user.email", "acceptance@example.invalid");
    await writeFile(path.join(dependency, "runtime.txt"), "reviewed runtime\n");
    git(dependency, "add", "runtime.txt"); git(dependency, "commit", "-qm", "test: create pinned runtime");
    const commit = git(dependency, "rev-parse", "HEAD");
    await writeFile(path.join(project, "workspace.lock.json"), JSON.stringify(lock(commit)));
    assert.deepEqual(await verifyWorkspace(project), [{
      name: "prism-fusion", repository: "kwhitestone/prism-fusion", commit,
      role: "verification", goModules: [], packages: [], directory: dependency
    }]);
    await writeFile(path.join(dependency, "runtime.txt"), "unreviewed runtime\n");
    await assert.rejects(verifyWorkspace(project), /uncommitted/i);
    git(dependency, "add", "runtime.txt"); git(dependency, "commit", "-qm", "test: advance dependency");
    await assert.rejects(verifyWorkspace(project), /commit mismatch/i);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("consumer verification requires a framework pin and rejects a self pin", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "prism-workspace-consumer-pin-"));
  try {
    const project = path.join(root, "consumer");
    const dependency = path.join(root, "utility");
    await mkdir(project); await mkdir(dependency);
    git(dependency, "init", "-q");
    git(dependency, "config", "user.name", "Plugin acceptance");
    git(dependency, "config", "user.email", "acceptance@example.invalid");
    await writeFile(path.join(dependency, "runtime.txt"), "runtime\n");
    git(dependency, "add", "runtime.txt"); git(dependency, "commit", "-qm", "test: create utility");
    const commit = git(dependency, "rev-parse", "HEAD");
    await writeFile(path.join(project, "workspace.lock.json"), JSON.stringify({
      version: 1,
      dependencies: [{ name: "utility", repository: "example/utility", commit, role: "verification" }]
    }));
    await assert.rejects(verifyWorkspace(project, { requireFramework: true }), /framework pin/i);

    await writeFile(path.join(project, "workspace.lock.json"), JSON.stringify({
      version: 1,
      dependencies: [{ name: "consumer", repository: "example/consumer", commit: "a".repeat(40), role: "verification" }]
    }));
    await assert.rejects(verifyWorkspace(project), /itself|self/i);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("verification rejects missing and symlinked dependency directories", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "prism-workspace-missing-"));
  try {
    const project = path.join(root, "consumer");
    await mkdir(project);
    await writeFile(path.join(project, "workspace.lock.json"), JSON.stringify(lock("a".repeat(40))));
    await assert.rejects(verifyWorkspace(project), /missing/i);
    await mkdir(path.join(root, "other"));
    await symlink(path.join(root, "other"), path.join(root, "prism-fusion"));
    await assert.rejects(verifyWorkspace(project), /symlink/i);
  } finally { await rm(root, { recursive: true, force: true }); }
});
