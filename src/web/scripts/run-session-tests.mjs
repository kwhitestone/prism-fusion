import { spawn } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import ts from "typescript";

const sourceDirectory = new URL("../src/addons/auth/", import.meta.url);
const temporaryDirectory = await mkdtemp(
  join(tmpdir(), "prism-fusion-session-tests-")
);

try {
  await writeFile(
    join(temporaryDirectory, "package.json"),
    JSON.stringify({ type: "module" })
  );
  for (const sourceName of ["session.ts", "session.test.ts"]) {
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

  const exitCode = await new Promise(resolve => {
    const child = spawn(
      process.execPath,
      [join(temporaryDirectory, "session.test.js")],
      { stdio: "inherit" }
    );
    child.on("exit", code => resolve(code ?? 1));
  });
  if (exitCode !== 0) process.exitCode = exitCode;
} finally {
  await rm(temporaryDirectory, { recursive: true, force: true });
}
