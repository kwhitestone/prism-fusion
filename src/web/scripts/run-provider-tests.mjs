import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const temporaryDirectory = await mkdtemp(
  join(tmpdir(), "prism-fusion-provider-tests-")
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
  const sources = {
    "core/reversible-slot": "reversible-slot",
    "core/reversible-slot.test": "reversible-slot.test",
    "../tests/fixtures/provider-lifecycle": "fixtures",
    "core/provider-lifecycle.test": "provider-lifecycle.test",
    "addons/auth/index": "auth",
    "addons/rbac/index": "rbac",
    "api/routes": "routes",
    "store/modules/user": "user",
    "store/modules/loginUI": "loginUI"
  };
  const aliases = {
    "@/core/reversible-slot": "reversible-slot",
    "@/store/modules/user": "user",
    "@/store/modules/loginUI": "loginUI",
    "@/api/routes": "routes",
    "../addons/auth/index": "auth",
    "../addons/rbac/index": "rbac",
    "../api/routes": "routes",
    "../store/modules/user": "user",
    "../store/modules/loginUI": "loginUI",
    "../../tests/fixtures/provider-lifecycle": "fixtures",
    "../utils": "fixtures",
    "./multiTags": "fixtures",
    "@/core/auth-session": "fixtures",
    "@/utils/auth": "fixtures",
    "@/config": "fixtures",
    "@/plugin/loader": "fixtures",
    "./api": "fixtures",
    "./session": "fixtures",
    "./components/LoginForm.vue": "fixtures"
  };
  for (const [file, output] of Object.entries(sources)) {
    const source = await readFile(
      new URL(`../src/${file}.ts`, import.meta.url),
      "utf8"
    );
    await writeFile(
      join(temporaryDirectory, `${output}.js`),
      ts
        .transpileModule(source, {
          compilerOptions: {
            module: ts.ModuleKind.ES2022,
            target: ts.ScriptTarget.ES2022
          }
        })
        .outputText.replace(
          /(from\s+["'])([^"']+)(["'])/g,
          (match, before, specifier, after) =>
            aliases[specifier]
              ? `${before}./${aliases[specifier]}.js${after}`
              : match
        )
    );
  }
  const args = ["--test", "--test-reporter=spec"];
  if (Number(process.versions.node.split(".")[0]) >= 22)
    args.push("--experimental-test-isolation=none");
  const exitCode = await new Promise(resolve => {
    const child = spawn(
      process.execPath,
      [...args, "reversible-slot.test.js", "provider-lifecycle.test.js"],
      {
        cwd: temporaryDirectory,
        stdio: "inherit"
      }
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
