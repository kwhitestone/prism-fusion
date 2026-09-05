import { readdir, readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { spawnSync } from "node:child_process";

const here = path.dirname(fileURLToPath(import.meta.url));
const runtimeSource = /\.(?:go|[cm]?[jt]sx?|vue)$/;
const testSource = /(?:_test\.go|\.(?:test|spec)\.[cm]?[jt]sx?)$/;
const routeMethods = new Set(["addRoute", "removeRoute", "clearRoutes", "createRouter", "registerExternalRoutes"]);
const allow = (mapping, file, value) => (mapping?.[file] ?? []).includes(value);
const isAddon = (file, policy) => [policy.serverRoot, policy.frontendRoot].some(root => file.startsWith(`${root}/addons/`));
const finding = (file, rule, message) => ({ file, rule, message });

function addonOwner(file, root) {
  if (!root || !file.startsWith(`${root}/addons/`)) return undefined;
  const relative = file.slice(`${root}/addons/`.length);
  return relative.includes("/") ? relative.split("/")[0] : undefined;
}

function frontendImportPath(specifier, file, policy) {
  if (specifier.startsWith(".")) return path.posix.normalize(path.posix.join(path.posix.dirname(file), specifier));
  for (const prefix of ["@/", "@biz/"]) {
    if (specifier.startsWith(prefix)) return path.posix.join(policy.frontendRoot, specifier.slice(prefix.length));
  }
  return undefined;
}

export function inspectInventory(files, policy) {
  const approved = new Set(policy.coreFiles);
  return files.filter(file => runtimeSource.test(file) && !testSource.test(file) && !isAddon(file, policy) && !approved.has(file))
    .map(file => finding(file, "core-inventory", "Runtime source outside addons requires explicit core-policy review."));
}

function scriptSource(file, source) {
  if (!file.endsWith(".vue")) return source;
  return [...source.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script\s*>/gi)].map(match => match[1]).join("\n");
}

function parentFunction(node, ts) {
  for (let current = node.parent; current; current = current.parent) {
    if (ts.isFunctionDeclaration(current) && current.name) return current.name.text;
    if (ts.isVariableDeclaration(current) && ts.isIdentifier(current.name) && current.initializer && (ts.isArrowFunction(current.initializer) || ts.isFunctionExpression(current.initializer))) return current.name.text;
    if (ts.isMethodDeclaration(current) && current.name) return current.name.getText();
  }
  return "<module>";
}

function addonImport(specifier, file, policy) {
  if (/(?:^|\/)addons(?:\/|$)/.test(specifier) && !specifier.startsWith(".")) return true;
  if (!specifier.startsWith(".")) return false;
  return path.posix.normalize(path.posix.join(path.posix.dirname(file), specifier)).startsWith(`${policy.frontendRoot}/addons/`);
}

function escapesSourceRoot(specifier, file, policy) {
  const extension = path.posix.extname(specifier);
  if (extension && !runtimeSource.test(specifier)) return false;
  if (specifier.startsWith(".")) {
    const target = path.posix.normalize(path.posix.join(path.posix.dirname(file), specifier));
    return !target.startsWith(`${policy.frontendRoot}/`);
  }
  for (const prefix of ["@/", "@biz/"]) {
    if (specifier.startsWith(prefix)) {
      const relative = path.posix.normalize(specifier.slice(prefix.length));
      return relative === ".." || relative.startsWith("../");
    }
  }
  return false;
}

/** AST checks complement the reviewed file inventory; they are not a semantic proof. */
export function inspectTypeScript(file, source, policy, ts) {
  const errors = [];
  const report = (rule, message) => errors.push(finding(file, rule, message));
  const addon = isAddon(file, policy);
  const syntax = ts.createSourceFile(file, scriptSource(file, source), ts.ScriptTarget.Latest, true, file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const aliases = new Map();
  const literal = node => node && (ts.isStringLiteralLike(node) || ts.isNoSubstitutionTemplateLiteral(node)) ? node.text : undefined;
  const member = expression => ts.isIdentifier(expression) ? (aliases.get(expression.text) ?? expression.text)
    : ts.isPropertyAccessExpression(expression) ? expression.name.text
      : ts.isElementAccessExpression(expression) ? literal(expression.argumentExpression) : undefined;
  const checkImport = specifier => {
    if (!specifier) return;
    if (escapesSourceRoot(specifier, file, policy)) report("source-root-import", `Runtime import escapes the reviewed source root: ${specifier}`);
    if (/\.(?:test|spec)(?:\.|$)/.test(specifier)) report("test-import", "Runtime code must not import test sources.");
    if (policy.enforceAddonImports) {
      const owner = addonOwner(file, policy.frontendRoot);
      const target = addonOwner(frontendImportPath(specifier, file, policy) ?? "", policy.frontendRoot);
      if (owner && target && owner !== target && !allow(policy.addonImports, file, specifier)) {
        report("cross-addon-import", `Addon ${owner} imports ${target} implementation; use an explicit contract: ${specifier}`);
      }
    }
    if (addon || !addonImport(specifier, file, policy)) return;
    const entrypoint = /(?:^|\/)addons\/[a-zA-Z0-9_-]+(?:\/index(?:\.ts)?)?$/.test(specifier);
    if (!(policy.frontendComposition?.includes(file) && entrypoint)) report("addon-import", `Core imports concrete addon implementation: ${specifier}`);
  };
  const checkURL = value => {
    if (value !== undefined && !allow(policy.apiURLs, file, value)) report("business-api", `Concrete endpoint belongs in an addon: ${value}`);
  };

  const visit = node => {
    if ((ts.isPropertyAccessExpression(node) || ts.isElementAccessExpression(node)) && routeMethods.has(member(node)) && !(ts.isCallExpression(node.parent) && node.parent.expression === node)) {
      if (addon || !allow(policy.routeMutators, file, parentFunction(node, ts))) report("route-registration", "Route mutator references are restricted to the registry/router lifecycle.");
    }
    if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) {
      checkImport(literal(node.moduleSpecifier));
      if (ts.isImportDeclaration(node) && node.importClause?.namedBindings && ts.isNamedImports(node.importClause.namedBindings)) {
        for (const element of node.importClause.namedBindings.elements) aliases.set(element.name.text, element.propertyName?.text ?? element.name.text);
      }
    }
    if (ts.isCallExpression(node)) {
      const name = member(node.expression);
      if (node.expression.kind === ts.SyntaxKind.ImportKeyword || name === "require") checkImport(literal(node.arguments[0]));
      if (name === "glob" || name === "globEager") {
        const args = node.arguments[0] && ts.isArrayLiteralExpression(node.arguments[0]) ? node.arguments[0].elements : [node.arguments[0]];
        for (const arg of args) {
          const pattern = literal(arg);
          if (pattern?.includes("addons/") && !allow(policy.discovery, file, pattern)) report("addon-discovery", `Addon discovery must use the plugin registry: ${pattern}`);
        }
      }
      if (routeMethods.has(name)) {
        const owner = name === "createRouter" ? name : parentFunction(node, ts);
        if (addon || !allow(policy.routeMutators, file, owner)) report("route-registration", `Route mutation ${name} is outside the approved registry/router lifecycle.`);
      }
      if (!addon && name === "defineStore" && !allow(policy.stores, file, literal(node.arguments[0]))) report("business-store", "Only reviewed shell stores may be declared outside addons.");
      if (!addon && ["fetch", "get", "post", "put", "patch", "delete", "request"].includes(name)) {
        const url = literal(node.arguments[0]);
        if (url?.startsWith("/") || /^https?:\/\//.test(url ?? "")) checkURL(url);
        const arg = node.arguments[0];
        if (arg && ts.isTemplateExpression(arg) && /^(?:\/|https?:\/\/)/.test(arg.head.text)) checkURL(arg.getText(syntax));
      }
    }
    if (!addon && ts.isPropertyAssignment(node)) {
      const key = ts.isIdentifier(node.name) ? node.name.text : literal(node.name);
      const value = literal(node.initializer);
      if (key === "url" && ts.isTemplateExpression(node.initializer)) checkURL(node.initializer.getText(syntax));
      if (key === "url" && value !== undefined) checkURL(value);
      if (key === "path" && value !== undefined && !allow(policy.routePaths, file, value)) report("route-literal", `Only declared shell paths may appear outside addons: ${value}`);
    }
    if (!addon && ts.isStringLiteralLike(node) && /^\/api(?:\/|$)/.test(node.text) && !allow(policy.apiURLs, file, node.text)) report("business-api", `Business API path belongs in an addon: ${node.text}`);
    ts.forEachChild(node, visit);
  };
  visit(syntax);
  return errors;
}

export function inspectGoFeatures(features, policy) {
  const { file } = features;
  const addon = isAddon(file, policy);
  const errors = [];
  const report = (rule, message) => errors.push(finding(file, rule, message));
  for (const item of features.imports ?? []) {
    if (item.path.startsWith(".")) report("source-root-import", "Relative Go imports bypass the reviewed module source root.");
    if (item.alias === ".") report("dot-import", "Dot imports hide dependency boundaries and are forbidden in core source.");
    if ((policy.forbiddenImports ?? []).some(prefix => item.path === prefix || item.path.startsWith(`${prefix}/`))) report("forbidden-import", `Module dependency crosses the service boundary: ${item.path}`);
    if (policy.enforceAddonImports && policy.serverModule) {
      const owner = addonOwner(file, policy.serverRoot);
      const prefix = `${policy.serverModule}/addons/`;
      const target = item.path.startsWith(prefix) ? item.path.slice(prefix.length).split("/")[0] : undefined;
      if (owner && target && owner !== target && !allow(policy.addonImports, file, item.path)) report("cross-addon-import", `Addon ${owner} imports ${target} implementation; use an explicit contract: ${item.path}`);
    }
    if (addon) continue;
    const composition = item.alias === "_" && allow(policy.composition, file, item.path);
    const commandEntrypoint = /\/cmd\/[^/]+\/main\.go$/.test(file) && allow(policy.entrypointImports, file, item.path);
    if (/(?:^|\/)addons(?:\/|$)/.test(item.path) && !composition && !commandEntrypoint) report("addon-import", `Core imports concrete addon implementation: ${item.path}`);
  }
  if (features.parseError) report("parse-error", features.parseError);
  if (addon) return errors;
  for (const call of features.calls ?? []) {
    const key = `${call.function}:${call.name}`;
    if (call.routing && !allow(policy.goRouteCalls, file, key)) report("route-registration", `Route registration ${key} is outside approved shell infrastructure.`);
    if (call.database && !allow(policy.goDatabaseCalls, file, key)) report("business-persistence", `Persistence call ${key} belongs in an addon.`);
  }
  for (const value of features.routePaths ?? []) {
    if (!allow(policy.goRoutePaths, file, value)) report("route-literal", `Only declared infrastructure endpoints may appear outside addons: ${value}`);
  }
  for (const type of features.types ?? []) {
    if (type.persisted && !allow(policy.goModelTypes, file, type.name)) report("business-model", `Persistent model ${type.name} belongs in an addon.`);
    if (type.fields > 0 && allow(policy.emptyGoTypes, file, type.name)) report("business-group", `${type.name} is a compatibility shell and must remain empty.`);
  }
  if ((features.selectors ?? []).includes("PRISM_DB") && !policy.goDBAccess?.includes(file)) report("database-access", "Direct framework database access belongs in addons or approved initialization.");
  return errors;
}

async function walk(directory, root, dataRoots, result = []) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if ([".git", "node_modules", "vendor", "dist"].includes(entry.name)) continue;
    const absolute = path.join(directory, entry.name);
    if (entry.isSymbolicLink()) throw new Error(`Source symlinks are not permitted: ${absolute}`);
    if (dataRoots.has(absolute)) continue;
    if (entry.isDirectory()) await walk(absolute, root, dataRoots, result);
    else if (runtimeSource.test(entry.name)) result.push(path.relative(root, absolute).split(path.sep).join("/"));
  }
  return result;
}

export async function checkRepository(root, profile = "framework", policyFile) {
  if (!["framework", "example", "consumer"].includes(profile)) throw new Error(`Unknown architecture profile: ${profile}`);
  if (profile === "consumer" && !policyFile) throw new Error("Consumer architecture profile requires --policy");
  const policy = JSON.parse(await readFile(policyFile ? path.resolve(root, policyFile) : path.join(here, `${profile}.json`), "utf8"));
  const roots = policy.sourceRoots ?? [policy.serverRoot, policy.frontendRoot].filter(Boolean);
  if (!Array.isArray(roots) || !roots.length || roots.some(dir => typeof dir !== "string" || path.isAbsolute(dir) || dir.split(/[\\/]/).some(part => part === "..") || path.resolve(root, dir) !== root && !path.resolve(root, dir).startsWith(`${root}${path.sep}`))) throw new Error("Invalid architecture source root");
  if (!Array.isArray(policy.coreFiles)) throw new Error("Architecture policy requires a reviewed core file inventory");
  const runtimeData = policy.runtimeDataRoots ?? [];
  if (!Array.isArray(runtimeData) || runtimeData.some(dir => typeof dir !== "string" || path.isAbsolute(dir) || dir.split(/[\\/]/).includes("..") || !roots.some(source => path.resolve(root, dir).startsWith(`${path.resolve(root, source)}${path.sep}`)))) throw new Error("Invalid runtime data root: it must be a reviewed subdirectory, never an entire source root");
  const dataRoots = new Set(runtimeData.map(dir => path.resolve(root, dir)));
  if (runtimeData.length) {
    const tracked = spawnSync("git", ["ls-files", "-z", "--", ...runtimeData], { cwd: root, encoding: "utf8" });
    if (tracked.status !== 0) throw new Error("Runtime data exclusions require a Git checkout");
    const trackedSource = tracked.stdout.split("\0").find(file => runtimeSource.test(file));
    if (trackedSource) throw new Error(`Tracked runtime source cannot be excluded as user data: ${trackedSource}`);
  }
  const require = createRequire(path.join(here, "../../src/web/package.json"));
  const ts = require("typescript");
  const files = [...new Set((await Promise.all(roots.map(dir => walk(path.join(root, dir), root, dataRoots)))).flat())].sort();
  const errors = inspectInventory(files, policy);
  const runtimeFiles = files.filter(file => !testSource.test(file));
  const sources = await Promise.all(runtimeFiles.map(async file => ({ file, source: await readFile(path.join(root, file), "utf8") })));
  const goSources = sources.filter(item => item.file.endsWith(".go"));
  const extraction = spawnSync("go", ["run", path.join(here, "go-inspect/main.go")], {
    cwd: root,
    input: JSON.stringify(goSources),
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024
  });
  if (extraction.status !== 0) throw new Error(`Go architecture analysis failed: ${extraction.stderr || extraction.error}`);
  for (const features of JSON.parse(extraction.stdout)) errors.push(...inspectGoFeatures(features, policy));
  for (const { file, source } of sources.filter(item => !item.file.endsWith(".go"))) errors.push(...inspectTypeScript(file, source, policy, ts));
  return { errors, sourceCount: runtimeFiles.length };
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try {
    const args = process.argv.slice(2);
    const value = (flag, fallback) => args.includes(flag) ? args[args.indexOf(flag) + 1] : fallback;
    const root = path.resolve(value("--root", path.join(here, "../..")));
    const result = await checkRepository(root, value("--profile", "framework"), value("--policy", undefined));
    for (const error of result.errors) console.error(`${error.file} [${error.rule}] ${error.message}`);
    if (result.errors.length) process.exitCode = 1;
    else console.log(`Architecture guard passed (${result.sourceCount} runtime source files, reviewed core inventory and addon boundaries).`);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
