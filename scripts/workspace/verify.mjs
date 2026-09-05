import { readFile, lstat, realpath } from "node:fs/promises";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { pathToFileURL } from "node:url";

const safeRelativePath = value => typeof value === "string" && value.length > 0 && value.length <= 256 &&
  !path.posix.isAbsolute(value) && !value.includes("\\") && !value.includes("\0") &&
  (value === "." || (path.posix.normalize(value) === value && !value.split("/").includes("..")));
const safeGoModule = value => typeof value === "string" && value.length <= 256 &&
  /^[A-Za-z0-9][A-Za-z0-9._~-]*(?:\/[A-Za-z0-9][A-Za-z0-9._~-]*)+$/.test(value);
const safePackage = value => typeof value === "string" && value.length <= 214 &&
  /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/.test(value);

/** Release dependencies are sibling repositories at exact commits, never branches. */
export function validateWorkspaceLock(lock) {
  if (!lock || lock.version !== 1 || !Array.isArray(lock.dependencies) || !lock.dependencies.length || lock.dependencies.length > 64) {
    throw new Error("Invalid workspace dependency lock");
  }
  const names = new Set();
  const modules = new Set();
  const packages = new Set();
  for (const dependency of lock.dependencies) {
    if (!dependency || !/^[a-z][a-z0-9-]{0,63}$/.test(dependency.name ?? "") ||
      !/^[A-Za-z0-9][A-Za-z0-9_.-]*\/[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(dependency.repository ?? "") ||
      !/^[a-f0-9]{40}$/.test(dependency.commit ?? "")) {
      throw new Error("Dependencies require a safe name, owner/repository and full immutable commit");
    }
    if (names.has(dependency.name)) throw new Error(`Duplicate workspace dependency: ${dependency.name}`);
    names.add(dependency.name);
    if (dependency.name === "prism-fusion" && dependency.repository !== "kwhitestone/prism-fusion") {
      throw new Error("Prism Fusion pin must use the official kwhitestone/prism-fusion repository");
    }
    if (!["runtime", "verification"].includes(dependency.role)) {
      throw new Error(`Dependency ${dependency.name} requires an explicit runtime or verification role`);
    }
    const goModules = dependency.goModules ?? [];
    const frontendPackages = dependency.packages ?? [];
    if (!Array.isArray(goModules) || goModules.length > 64 || !Array.isArray(frontendPackages) || frontendPackages.length > 64) {
      throw new Error(`Dependency ${dependency.name} has invalid runtime bindings`);
    }
    if (dependency.role === "runtime" && goModules.length + frontendPackages.length === 0) {
      throw new Error(`Runtime dependency ${dependency.name} requires at least one binding`);
    }
    if (dependency.role === "verification" && goModules.length + frontendPackages.length > 0) {
      throw new Error(`Verification dependency ${dependency.name} cannot declare runtime bindings`);
    }
    for (const binding of goModules) {
      if (!binding || !safeGoModule(binding.module) || !safeRelativePath(binding.path)) {
        throw new Error(`Dependency ${dependency.name} has an invalid Go module binding or path`);
      }
      if (modules.has(binding.module)) throw new Error(`Duplicate Go module binding: ${binding.module}`);
      modules.add(binding.module);
    }
    for (const binding of frontendPackages) {
      if (!binding || !safePackage(binding.package) || !safeRelativePath(binding.path) ||
        (binding.sharedPeers !== undefined && (!Array.isArray(binding.sharedPeers) || binding.sharedPeers.length > 32 ||
          binding.sharedPeers.some(peer => !safePackage(peer)) || new Set(binding.sharedPeers).size !== binding.sharedPeers.length))) {
        throw new Error(`Dependency ${dependency.name} has an invalid frontend package binding or path`);
      }
      if (packages.has(binding.package)) throw new Error(`Duplicate frontend package binding: ${binding.package}`);
      packages.add(binding.package);
    }
  }
  return {
    version: 1,
    dependencies: lock.dependencies.map(dependency => ({
      ...structuredClone(dependency),
      goModules: structuredClone(dependency.goModules ?? []),
      packages: structuredClone(dependency.packages ?? [])
    }))
  };
}

export async function readWorkspaceLock(file) {
  const source = await readFile(file, "utf8");
  if (source.length > 128_000) throw new Error("Workspace dependency lock is too large");
  return validateWorkspaceLock(JSON.parse(source));
}

function git(directory, args) {
  const result = spawnSync("git", ["-C", directory, ...args], { encoding: "utf8", maxBuffer: 1024 * 1024 });
  if (result.status !== 0) throw new Error(`Cannot inspect dependency ${path.basename(directory)}: git ${args[0]} failed`);
  return result.stdout.trim();
}

/** Verifies pinned sibling checkout identity and cleanliness before binding checks. */
export async function verifyWorkspace(projectRoot, options = {}) {
  const root = await realpath(projectRoot);
  const workspace = path.dirname(root);
  const lock = await readWorkspaceLock(path.resolve(root, options.lockFile ?? "workspace.lock.json"));
  if (options.requireFramework && !lock.dependencies.some(dependency => dependency.name === "prism-fusion")) {
    throw new Error("Prism Fusion framework pin is required");
  }
  const verified = [];
  for (const dependency of lock.dependencies) {
    const directory = path.join(workspace, dependency.name);
    if (directory === root) throw new Error(`Consumer cannot pin itself as dependency: ${dependency.name}`);
    let stats;
    try { stats = await lstat(directory); }
    catch { throw new Error(`Missing workspace dependency: ${dependency.name}`); }
    if (stats.isSymbolicLink() || !stats.isDirectory() || await realpath(directory) !== directory) {
      throw new Error(`Workspace dependency must be a real directory, not a symlink: ${dependency.name}`);
    }
    if (git(directory, ["rev-parse", "--show-toplevel"]) !== directory) {
      throw new Error(`Workspace dependency is not its own repository: ${dependency.name}`);
    }
    const actual = git(directory, ["rev-parse", "HEAD"]);
    if (actual !== dependency.commit) throw new Error(`Dependency commit mismatch: ${dependency.name} expected ${dependency.commit}, found ${actual}`);
    if (git(directory, ["status", "--porcelain", "--untracked-files=all"])) {
      throw new Error(`Dependency has uncommitted changes: ${dependency.name}`);
    }
    verified.push({ ...dependency, directory });
  }
  return verified;
}

/** Recheck against the original snapshot, not a lock rewritten by a build stage. */
export async function verifyWorkspaceUnchanged(projectRoot, originalSnapshot) {
  const current = await verifyWorkspace(projectRoot, { requireFramework: true });
  if (JSON.stringify(current) !== originalSnapshot) {
    throw new Error("Workspace dependency lock changed during verification; original pins must remain unchanged");
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try {
    const args = process.argv.slice(2);
    const rootIndex = args.indexOf("--root");
    const lockIndex = args.indexOf("--lock");
    const result = await verifyWorkspace(path.resolve(rootIndex < 0 ? process.cwd() : args[rootIndex + 1]), {
      lockFile: lockIndex < 0 ? "workspace.lock.json" : args[lockIndex + 1]
    });
    for (const dependency of result) console.log(`Pinned dependency verified: ${dependency.name} ${dependency.commit}`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
