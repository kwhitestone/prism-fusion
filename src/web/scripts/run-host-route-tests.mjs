import { mkdtemp, readFile, writeFile, symlink, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import ts from "typescript";

const directory = await mkdtemp(join(tmpdir(), "prism-host-route-tests-"));
try {
  await writeFile(join(directory, "package.json"), JSON.stringify({ type: "module" }));
  await symlink(fileURLToPath(new URL("../node_modules", import.meta.url)), join(directory, "node_modules"));
  for (const name of ["registry", "host-routes", "host-routes.test"]) {
    const source = await readFile(new URL(`../src/plugin/${name}.ts`, import.meta.url), "utf8");
    const output = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 } }).outputText;
    await writeFile(join(directory, `${name}.js`), output);
  }
  const args = ["--test", "--test-reporter=spec"];
  if (Number(process.versions.node.split(".")[0]) >= 22) args.push("--experimental-test-isolation=none");
  const result = spawnSync(process.execPath, [...args, join(directory, "host-routes.test.js")], { stdio: "inherit" });
  process.exitCode = result.status ?? 1;
} finally {
  await rm(directory, { recursive: true, force: true });
}
