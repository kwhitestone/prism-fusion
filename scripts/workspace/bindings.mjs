import { createRequire } from "node:module";
import { lstat, readFile, readdir, realpath } from "node:fs/promises";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { pathToFileURL } from "node:url";

const MAX_PACKAGE_FILES = 2_000;
const MAX_PACKAGE_BYTES = 32 * 1024 * 1024;

const inside = (root, candidate) => candidate === root || candidate.startsWith(`${root}${path.sep}`);

async function pinnedPath(pin, relative) {
  const root = await realpath(pin.directory);
  const candidate = await realpath(path.resolve(root, relative));
  if (!inside(root, candidate)) throw new Error(`Pinned dependency path escapes ${pin.name}: ${relative}`);
  return candidate;
}

function run(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd, encoding: "utf8", maxBuffer: 4 * 1024 * 1024, env: process.env
  });
  if (result.status !== 0) {
    const detail = (result.stderr || result.stdout).trim().split("\n").at(-1) ?? "unknown error";
    throw new Error(`${command} ${args.join(" ")} failed: ${detail}`);
  }
  return result.stdout;
}

async function verifyGoBinding(pin, binding, backendDirectory) {
  const expected = await pinnedPath(pin, binding.path);
  const output = run("go", ["list", "-m", "-json", binding.module], backendDirectory);
  let metadata;
  try { metadata = JSON.parse(output); }
  catch { throw new Error(`Cannot parse Go module resolution for ${binding.module}`); }
  if (metadata.Path !== binding.module) throw new Error(`Go resolved the wrong module for ${binding.module}`);
  const directory = metadata.Replace?.Dir ?? metadata.Dir;
  if (!directory) throw new Error(`Go module ${binding.module} has no resolved source directory`);
  const actual = await realpath(directory);
  if (actual !== expected) {
    throw new Error(`Go module ${binding.module} resolved to ${actual}; expected pinned directory ${expected}`);
  }
}

function dependencySpec(pkg, name) {
  for (const section of ["dependencies", "devDependencies", "optionalDependencies"]) {
    if (typeof pkg[section]?.[name] === "string") return pkg[section][name];
  }
  return undefined;
}

async function fileTarget(frontendDirectory, spec, label) {
  if (typeof spec !== "string" || !spec.startsWith("file:") || spec.length <= 5) {
    throw new Error(`${label} must be an explicit file dependency`);
  }
  const relative = spec.slice(5);
  if (path.isAbsolute(relative) || relative.includes("\0")) throw new Error(`${label} must use a portable relative file path`);
  return realpath(path.resolve(frontendDirectory, relative));
}

async function verifyNpmLock(frontendDirectory, binding, expected) {
  const lock = JSON.parse(await readFile(path.join(frontendDirectory, "package-lock.json"), "utf8"));
  const root = lock.packages?.[""] ?? {};
  const rootSpec = dependencySpec(root, binding.package);
  const installed = lock.packages?.[`node_modules/${binding.package}`];
  const resolved = installed?.resolved;
  if (!rootSpec || typeof resolved !== "string") throw new Error(`npm lock is missing ${binding.package}`);
  const lockTargets = [
    await fileTarget(frontendDirectory, rootSpec, `npm lock entry for ${binding.package}`),
    await fileTarget(frontendDirectory, resolved.startsWith("file:") ? resolved : `file:${resolved}`, `npm resolved entry for ${binding.package}`)
  ];
  if (lockTargets.some(target => target !== expected)) {
    throw new Error(`npm lock target for ${binding.package} does not match pinned directory ${expected}`);
  }
}

async function verifyPnpmLock(frontendDirectory, binding, expected) {
  const source = await readFile(path.join(frontendDirectory, "pnpm-lock.yaml"), "utf8");
  const scalar = value => {
    const trimmed = value.trim();
    if (trimmed.startsWith("'") && trimmed.endsWith("'")) return trimmed.slice(1, -1).replaceAll("''", "'");
    if (trimmed.startsWith('"') && trimmed.endsWith('"')) return JSON.parse(trimmed);
    return trimmed;
  };
  let fields = {};
  let inRootImporter = false;
  let section = false;
  let selected = false;
  for (const line of source.split(/\r?\n/)) {
    const match = /^( *)([^#][^:]*):(.*)$/.exec(line);
    if (!match) continue;
    const indent = match[1].length;
    const key = scalar(match[2]);
    const value = scalar(match[3]);
    if (indent === 0) {
      inRootImporter = key === "importers";
      section = selected = false;
    } else if (inRootImporter && indent === 2) {
      inRootImporter = key === ".";
      section = selected = false;
    } else if (inRootImporter && indent === 4) {
      section = ["dependencies", "devDependencies", "optionalDependencies"].includes(key);
      selected = false;
    } else if (inRootImporter && section && indent === 6) {
      selected = key === binding.package;
    } else if (inRootImporter && section && selected && indent === 8 && ["specifier", "version"].includes(key)) {
      fields = { ...fields, [key]: value };
    }
  }
  const peerSuffix = fields.version?.indexOf("(") ?? -1;
  const version = peerSuffix < 0 ? fields.version : fields.version.slice(0, peerSuffix);
  if (!fields.specifier || !version) throw new Error(`pnpm lock is missing ${binding.package}`);
  const targets = await Promise.all([
    fileTarget(frontendDirectory, fields.specifier, `pnpm specifier for ${binding.package}`),
    fileTarget(frontendDirectory, version, `pnpm version for ${binding.package}`)
  ]);
  if (targets.some(target => target !== expected)) {
    throw new Error(`pnpm lock target for ${binding.package} does not match pinned directory ${expected}`);
  }
}

async function packageSnapshot(root) {
  const entries = new Map();
  let bytes = 0;
  async function walk(directory, prefix = "") {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      if (["node_modules", ".git"].includes(entry.name)) continue;
      const relative = prefix ? `${prefix}/${entry.name}` : entry.name;
      const absolute = path.join(directory, entry.name);
      if (entry.isSymbolicLink()) throw new Error(`Package source contains unsupported symlink: ${relative}`);
      if (entry.isDirectory()) {
        await walk(absolute, relative);
      } else if (entry.isFile()) {
        const content = await readFile(absolute);
        bytes += content.length;
        if (entries.size >= MAX_PACKAGE_FILES || bytes > MAX_PACKAGE_BYTES) {
          throw new Error(`Package source is too large to verify: ${root}`);
        }
        entries.set(relative, content);
      }
    }
  }
  await walk(root);
  return entries;
}

async function assertPackageCopy(expected, installed, packageName) {
  const [source, copy] = await Promise.all([packageSnapshot(expected), packageSnapshot(installed)]);
  if (source.size !== copy.size) throw new Error(`Installed ${packageName} differs from pinned source (stale file-package copy)`);
  for (const [file, content] of source) {
    const installedContent = copy.get(file);
    if (!installedContent?.equals(content)) {
      throw new Error(`Installed ${packageName} differs from pinned source at ${file} (stale file-package copy)`);
    }
  }
}

function resolveFrom(directory, packageName) {
  const require = createRequire(path.join(directory, "package.json"));
  return realpath(require.resolve(`${packageName}/package.json`));
}

async function viteDedupe(frontendDirectory) {
  const require = createRequire(path.join(frontendDirectory, "package.json"));
  let vite;
  try { vite = await import(pathToFileURL(require.resolve("vite")).href); }
  catch (error) { throw new Error(`Cannot load the consumer bundler to verify shared Vue identity: ${error.message}`); }
  const resolveConfig = vite.resolveConfig ?? vite.default?.resolveConfig;
  if (typeof resolveConfig !== "function") throw new Error("Consumer Vite installation does not expose resolveConfig");
  const config = await resolveConfig({ root: frontendDirectory, logLevel: "silent" }, "build", "production");
  return new Set(config.resolve?.dedupe ?? []);
}

async function verifySharedPeers(frontendDirectory, installed, binding) {
  if (!binding.sharedPeers?.length) return;
  const packageJSON = JSON.parse(await readFile(path.join(installed, "package.json"), "utf8"));
  let dedupe;
  for (const peer of binding.sharedPeers) {
    if (!packageJSON.peerDependencies?.[peer]) {
      throw new Error(`${binding.package} does not declare shared peer ${peer}`);
    }
    const consumerResolution = await resolveFrom(frontendDirectory, peer);
    let packageResolution;
    try { packageResolution = await resolveFrom(installed, peer); }
    catch { packageResolution = undefined; }
    if (packageResolution === consumerResolution) continue;
    dedupe ??= await viteDedupe(frontendDirectory);
    if (!dedupe.has(peer)) {
      throw new Error(`${binding.package} may load a second ${peer} instance; consumer bundler must dedupe it`);
    }
  }
}

async function verifyFrontendBinding(pin, binding, frontendDirectory, packageManager) {
  const expected = await pinnedPath(pin, binding.path);
  const pkg = JSON.parse(await readFile(path.join(frontendDirectory, "package.json"), "utf8"));
  const declaredTarget = await fileTarget(frontendDirectory, dependencySpec(pkg, binding.package), binding.package);
  if (declaredTarget !== expected) {
    throw new Error(`Frontend file dependency ${binding.package} resolves to ${declaredTarget}; expected ${expected}`);
  }
  if (packageManager === "pnpm") await verifyPnpmLock(frontendDirectory, binding, expected);
  else if (packageManager === "npm") await verifyNpmLock(frontendDirectory, binding, expected);
  else throw new Error(`Unsupported frontend package manager: ${packageManager}`);

  const installedPath = path.join(frontendDirectory, "node_modules", ...binding.package.split("/"));
  const stats = await lstat(installedPath);
  if (!stats.isDirectory() && !stats.isSymbolicLink()) throw new Error(`Installed package is invalid: ${binding.package}`);
  const installed = await realpath(installedPath);
  if (installed !== expected) {
    const modules = await realpath(path.join(frontendDirectory, "node_modules"));
    if (!inside(modules, installed)) throw new Error(`Installed ${binding.package} is outside the consumer and pinned source`);
    await assertPackageCopy(expected, installed, binding.package);
  }
  const installedPackage = JSON.parse(await readFile(path.join(installed, "package.json"), "utf8"));
  if (installedPackage.name !== binding.package) throw new Error(`Installed package identity mismatch: ${binding.package}`);
  await verifySharedPeers(frontendDirectory, installed, binding);
}

/** Proves every runtime pin is the source actually selected by Go and the frontend package manager. */
export async function verifyConsumerBindings(pins, options = {}) {
  const runtime = pins.filter(pin => pin.role === "runtime");
  if (options.requireFrameworkRuntime && !runtime.some(pin => pin.name === "prism-fusion")) {
    throw new Error("Consumer service architecture requires a runtime framework pin");
  }
  const needsBackend = runtime.some(pin => pin.goModules.length > 0);
  const needsFrontend = runtime.some(pin => pin.packages.length > 0);
  if (needsBackend && !options.backendDirectory) throw new Error("Runtime Go bindings require a configured backend directory");
  if (needsFrontend && (!options.frontendDirectory || !options.packageManager)) {
    throw new Error("Runtime frontend bindings require a configured frontend directory and package manager");
  }
  const backend = options.backendDirectory ? await realpath(options.backendDirectory) : undefined;
  const frontend = options.frontendDirectory ? await realpath(options.frontendDirectory) : undefined;
  for (const pin of runtime) {
    for (const binding of pin.goModules) await verifyGoBinding(pin, binding, backend);
    for (const binding of pin.packages) await verifyFrontendBinding(pin, binding, frontend, options.packageManager);
  }
}
