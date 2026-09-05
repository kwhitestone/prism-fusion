import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const sourceDirectory = new URL("../src/plugin/", import.meta.url);
const temporaryDirectory = await mkdtemp(
  join(tmpdir(), "prism-fusion-plugin-tests-")
);

try {
  await writeFile(
    join(temporaryDirectory, "package.json"),
    JSON.stringify({ type: "module" })
  );
  await symlink(
    fileURLToPath(new URL("../node_modules", import.meta.url)),
    join(temporaryDirectory, "node_modules"),
    "dir"
  );
  for (const sourceName of [
    "types.ts",
    "registry.ts",
    "registry.test.ts",
    "runtime.ts",
    "runtime.test.ts",
    "host-routes.ts",
    "headless.ts",
    "headless.test.ts",
    "remote.ts",
    "remote-registry.ts",
    "remote-channel.ts",
    "remote.test.ts",
    "navigation.ts",
    "navigation.test.ts"
  ]) {
    const source = await readFile(new URL(sourceName, sourceDirectory), "utf8");
    const output = ts.transpileModule(source, {
      compilerOptions: {
        module: ts.ModuleKind.ES2022,
        target: ts.ScriptTarget.ES2022
      },
      fileName: sourceName
    }).outputText;
    await writeFile(
      join(temporaryDirectory, basename(sourceName, ".ts") + ".js"),
      output
    );
  }
  const args = ["--test", "--test-reporter=spec"];
  if (Number(process.versions.node.split(".")[0]) >= 22) {
    args.push("--experimental-test-isolation=none");
  }
  if (process.argv.includes("--coverage")) {
    args.push(
      "--experimental-test-coverage",
      "--test-coverage-include=**/registry.js",
      "--test-coverage-include=**/runtime.js",
      "--test-coverage-include=**/navigation.js",
      "--test-coverage-include=**/headless.js",
      "--test-coverage-include=**/remote-registry.js",
      "--test-coverage-include=**/remote-channel.js",
      "--test-coverage-lines=80"
    );
  }
  const exitCode = await new Promise(resolve => {
    const child = spawn(
      process.execPath,
      [...args, "registry.test.js", "runtime.test.js", "navigation.test.js", "headless.test.js", "remote.test.js"],
      { cwd: temporaryDirectory, stdio: "inherit" }
    );
    child.on("error", error => {
      console.error(error);
      resolve(1);
    });
    child.on("exit", code => resolve(code ?? 1));
  });
  if (exitCode !== 0) process.exitCode = exitCode;
} finally {
  await rm(temporaryDirectory, { recursive: true, force: true });
}
