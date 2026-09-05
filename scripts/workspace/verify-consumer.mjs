import { existsSync } from "node:fs";
import { realpath, readFile } from "node:fs/promises";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { verifyWorkspace, verifyWorkspaceUnchanged } from "./verify.mjs";
import { verifyConsumerBindings } from "./bindings.mjs";
import { checkRepository } from "../architecture/guard.mjs";

const args = process.argv.slice(2);
const value = (flag, fallback = "") => args.includes(flag) ? args[args.indexOf(flag) + 1] : fallback;
const root = await realpath(path.resolve(value("--root", process.cwd())));

function run(command, arguments_, cwd) {
  const result = spawnSync(command, arguments_, { cwd, stdio: "inherit", env: process.env });
  if (result.status !== 0) throw new Error(`${command} ${arguments_.join(" ")} failed in ${path.relative(root, cwd) || "."}`);
}

async function sourceDirectory(relative) {
  if (path.isAbsolute(relative) || relative.split(/[\\/]/).includes("..")) throw new Error("Verification source must stay inside the consumer repository");
  const directory = await realpath(path.resolve(root, relative));
  if (directory !== root && !directory.startsWith(`${root}${path.sep}`)) throw new Error("Verification source escapes the consumer repository");
  return directory;
}

try {
  const dependencies = await verifyWorkspace(root, { requireFramework: true });
  const originalSnapshot = JSON.stringify(dependencies);
  console.log(`Verified ${dependencies.length} pinned workspace dependencies`);
  const policyFile = value("--policy", "architecture.json");
  if (path.isAbsolute(policyFile) || policyFile.split(/[\\/]/).includes("..")) {
    throw new Error("Architecture policy must stay inside the consumer repository");
  }
  const policyPath = await realpath(path.resolve(root, policyFile));
  if (policyPath !== root && !policyPath.startsWith(`${root}${path.sep}`)) {
    throw new Error("Architecture policy escapes the consumer repository");
  }
  const policy = JSON.parse(await readFile(policyPath, "utf8"));
  const frontend = value("--frontend");
  const backend = value("--backend");
  let frontendDirectory;
  let backendDirectory;
  let packageManager;
  if (frontend) {
    frontendDirectory = await sourceDirectory(frontend);
    packageManager = existsSync(path.join(frontendDirectory, "pnpm-lock.yaml")) ? "pnpm" : "npm";
    run(packageManager, packageManager === "pnpm" ? ["install", "--frozen-lockfile"] : ["ci"], frontendDirectory);
    await verifyWorkspaceUnchanged(root, originalSnapshot);
  }
  if (backend) backendDirectory = await sourceDirectory(backend);
  await verifyConsumerBindings(dependencies, {
    backendDirectory, frontendDirectory, packageManager,
    requireFrameworkRuntime: Boolean(policy.serverModule)
  });
  await verifyWorkspaceUnchanged(root, originalSnapshot);
  console.log("Pinned runtime dependency bindings verified");
  const architecture = await checkRepository(root, "consumer", policyFile);
  if (architecture.errors.length) {
    for (const finding of architecture.errors) console.error(`${finding.file} [${finding.rule}] ${finding.message}`);
    throw new Error("Consumer architecture validation failed");
  }
  console.log(`Consumer architecture passed (${architecture.sourceCount} runtime files)`);
  if (backendDirectory) {
    run("go", ["test", "-race", "./..."], backendDirectory);
    run("go", ["vet", "./..."], backendDirectory);
    run("go", ["build", "./..."], backendDirectory);
  }
  if (frontendDirectory) {
    const pkg = JSON.parse(await readFile(path.join(frontendDirectory, "package.json"), "utf8"));
    if (!pkg.scripts?.["test:unit"] || !pkg.scripts?.build) throw new Error("Consumer must provide frontend test:unit and build gates");
    run(packageManager, ["run", "test:unit"], frontendDirectory);
    run(packageManager, ["run", "build"], frontendDirectory);
  }
  await verifyWorkspaceUnchanged(root, originalSnapshot);
  console.log("Original pinned inputs remained unchanged throughout verification");
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
