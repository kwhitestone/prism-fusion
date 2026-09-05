import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const sourceDirectory = new URL("../src/plugin/", import.meta.url);
const temporaryDirectory = await mkdtemp(join(tmpdir(), "prism-fusion-loader-tests-"));

// Only browser/store/transport infrastructure is substituted. The production
// loader, registry, runtime and HostRoutes execute against real Vue/Router.
const hostAdapter = `
import { createMemoryHistory, createRouter } from "vue-router";
import { HostRoutes } from "./host-routes.js";
const View = { render: () => null };
export let router;
export let state;
export let counts;
export function resetHost() {
  const core = [{ path: "/", name: "Home", redirect: "/welcome", component: View,
    children: [{ path: "/welcome", name: "Welcome", component: View }] },
    { path: "/login", name: "Login", component: View }];
  router = createRouter({ history: createMemoryHistory(), routes: core });
  state = new HostRoutes(core);
  counts = { commits: 0, clears: 0 };
}
export function commitPluginRoutes(routes) {
  state.commit(routes);
  state.restore(router);
  counts.commits++;
}
export function clearPluginRoutes() {
  state.clear();
  state.restore(router);
  counts.clears++;
}
resetHost();
`;

function transformInfrastructure(source) {
  const discovery = /const pluginModules = import\.meta\.glob[\s\S]*?\n\);/;
  if (!discovery.test(source)) throw new Error("Loader addon discovery fixture no longer matches source");
  const substituted = source.replace(discovery, "const pluginModules = {};");
  return substituted
    .replaceAll('"@/router/index"', '"./loader-host.js"')
    .replaceAll('"@/utils/request"', '"./loader-request.js"')
    .replaceAll('"./registry"', '"./registry.js"')
    .replaceAll('"./runtime"', '"./runtime.js"');
}

try {
  await writeFile(join(temporaryDirectory, "package.json"), JSON.stringify({ type: "module" }));
  await symlink(fileURLToPath(new URL("../node_modules", import.meta.url)), join(temporaryDirectory, "node_modules"), "dir");
  await writeFile(join(temporaryDirectory, "loader-host.js"), hostAdapter);
  await writeFile(join(temporaryDirectory, "loader-request.js"), `
export let requests = [];
let failure;
export function resetRequests(nextFailure) { requests = []; failure = nextFailure; }
export default async function service(request) {
  requests = [...requests, request];
  if (failure) throw failure;
  return { data: {} };
}
`);
  for (const sourceName of ["types.ts", "registry.ts", "runtime.ts", "host-routes.ts", "loader.ts", "loader.test.ts"]) {
    const original = await readFile(new URL(sourceName, sourceDirectory), "utf8");
    const source = sourceName === "loader.ts" ? transformInfrastructure(original) : original;
    const output = ts.transpileModule(source, {
      compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 },
      fileName: sourceName
    }).outputText;
    await writeFile(join(temporaryDirectory, basename(sourceName, ".ts") + ".js"), output);
  }
  const args = ["--test", "--test-reporter=spec", "--test-timeout=10000"];
  if (Number(process.versions.node.split(".")[0]) >= 22) {
    args.push("--experimental-test-isolation=none");
  }
  if (process.argv.includes("--coverage")) {
    args.push("--experimental-test-coverage", "--test-coverage-include=**/loader.js*");
  }
  const exitCode = await new Promise(resolve => {
    const child = spawn(process.execPath, [...args, "loader.test.js"], {
      cwd: temporaryDirectory, stdio: "inherit"
    });
    child.on("error", error => { console.error(error); resolve(1); });
    child.on("exit", code => resolve(code ?? 1));
  });
  if (exitCode !== 0) process.exitCode = exitCode;
} finally {
  await rm(temporaryDirectory, { recursive: true, force: true });
}
