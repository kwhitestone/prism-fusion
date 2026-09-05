import assert from "node:assert/strict";
import test from "node:test";
import { mkdtemp, mkdir, readFile, rm, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { verifyConsumerBindings } from "./bindings.mjs";

async function write(directory, relative, contents) {
  const file = path.join(directory, relative);
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, contents);
}

const runtimePin = (directory, extra = {}) => ({
  name: "prism-fusion", repository: "kwhitestone/prism-fusion", commit: "a".repeat(40),
  role: "runtime", directory, goModules: [], packages: [], ...extra
});

test("Go binding follows the effective replace or go.work module directory", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "prism-go-binding-"));
  try {
    const expected = path.join(root, "prism-fusion");
    const wrong = path.join(root, "wrong-framework");
    const backend = path.join(root, "consumer", "server");
    for (const directory of [expected, wrong]) {
      await write(directory, "src/server/go.mod", "module example.test/framework\n\ngo 1.25\n");
    }
    await write(backend, "go.mod", `module example.test/consumer\n\ngo 1.25\n\nrequire example.test/framework v0.0.0\n\nreplace example.test/framework => ${path.relative(backend, path.join(expected, "src/server"))}\n`);
    const pin = runtimePin(expected, { goModules: [{ module: "example.test/framework", path: "src/server" }] });
    await assert.doesNotReject(verifyConsumerBindings([pin], { backendDirectory: backend }));

    await write(backend, "go.mod", `module example.test/consumer\n\ngo 1.25\n\nrequire example.test/framework v0.0.0\n\nreplace example.test/framework => ${path.relative(backend, path.join(wrong, "src/server"))}\n`);
    await assert.rejects(verifyConsumerBindings([pin], { backendDirectory: backend }), /Go module.*expected|directory/i);
    const checker = {
      name: "prism-fusion", repository: "kwhitestone/prism-fusion", commit: "a".repeat(40),
      role: "verification", directory: expected, goModules: [], packages: []
    };
    await assert.rejects(verifyConsumerBindings([
      checker, { ...pin, name: "shared-contracts" }
    ], { backendDirectory: backend }), /Go module.*expected|directory/i,
    "non-framework runtime pins must receive the same effective-directory check");

    await write(backend, "go.mod", "module example.test/consumer\n\ngo 1.25\n\nrequire example.test/framework v0.0.0\n");
    await write(path.join(root, "consumer"), "go.work", `go 1.25\n\nuse (\n ./server\n ${path.relative(path.join(root, "consumer"), path.join(wrong, "src/server"))}\n)\n`);
    await assert.rejects(verifyConsumerBindings([pin], { backendDirectory: backend }), /Go module.*expected|directory/i);
  } finally { await rm(root, { recursive: true, force: true }); }
});

async function frontendFixture(root, { declared = "prism-fusion", locked = declared, installed = declared, installedSource = "reviewed\n" } = {}) {
  const frontend = path.join(root, "consumer", "web");
  const expected = path.join(root, "prism-fusion", "src/web/src/plugin");
  const wrong = path.join(root, "wrong-framework", "src/web/src/plugin");
  const runtimePackage = JSON.stringify({
    name: "@prism-fusion/plugin-runtime", version: "2.1.0",
    peerDependencies: { vue: "^3.5.0", "vue-router": "^4.6.0" }
  });
  await write(expected, "package.json", runtimePackage);
  await write(expected, "runtime.ts", "reviewed\n");
  await write(wrong, "package.json", runtimePackage);
  await write(wrong, "runtime.ts", "wrong\n");
  const locations = { "prism-fusion": expected, "wrong-framework": wrong };
  const declaredPath = path.relative(frontend, locations[declared]);
  const lockedPath = path.relative(frontend, locations[locked]);
  await write(frontend, "package.json", JSON.stringify({
    name: "consumer-web", dependencies: { "@prism-fusion/plugin-runtime": `file:${declaredPath}` }
  }));
  await write(frontend, "package-lock.json", JSON.stringify({
    name: "consumer-web", lockfileVersion: 3,
    packages: {
      "": { dependencies: { "@prism-fusion/plugin-runtime": `file:${lockedPath}` } },
      "node_modules/@prism-fusion/plugin-runtime": { resolved: lockedPath, link: installed !== "copy" }
    }
  }));
  const installation = path.join(frontend, "node_modules/@prism-fusion/plugin-runtime");
  await mkdir(path.dirname(installation), { recursive: true });
  if (installed === "copy") {
    await mkdir(installation);
    await write(installation, "package.json", runtimePackage);
    await write(installation, "runtime.ts", installedSource);
  } else {
    await symlink(locations[installed], installation);
  }
  return { frontend, expected };
}

const packageBinding = directory => runtimePin(path.join(directory, "prism-fusion"), {
  packages: [{ package: "@prism-fusion/plugin-runtime", path: "src/web/src/plugin" }]
});

test("frontend binding rejects a declared or locked file target outside the pin", async () => {
  for (const configuration of [
    { declared: "wrong-framework", locked: "wrong-framework", installed: "wrong-framework" },
    { declared: "prism-fusion", locked: "wrong-framework", installed: "prism-fusion" }
  ]) {
    const root = await mkdtemp(path.join(tmpdir(), "prism-frontend-binding-"));
    try {
      const { frontend } = await frontendFixture(root, configuration);
      await assert.rejects(
        verifyConsumerBindings([packageBinding(root)], { frontendDirectory: frontend, packageManager: "npm" }),
        /file dependency|lock.*target|expected/i
      );
    } finally { await rm(root, { recursive: true, force: true }); }
  }
});

test("frontend binding detects a stale installed file-package copy", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "prism-frontend-copy-"));
  try {
    const { frontend } = await frontendFixture(root, { installed: "copy", installedSource: "stale\n" });
    await assert.rejects(
      verifyConsumerBindings([packageBinding(root)], { frontendDirectory: frontend, packageManager: "npm" }),
      /installed.*differs|stale/i
    );
    await writeFile(path.join(frontend, "node_modules/@prism-fusion/plugin-runtime/runtime.ts"),
      await readFile(path.join(root, "prism-fusion/src/web/src/plugin/runtime.ts")));
    await assert.doesNotReject(verifyConsumerBindings([packageBinding(root)], { frontendDirectory: frontend, packageManager: "npm" }));
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("pnpm lock binding verifies both the importer specifier and resolved file version", async () => {
  const root = await mkdtemp(path.join(tmpdir(), "prism-pnpm-binding-"));
  try {
    const { frontend } = await frontendFixture(root, { installed: "copy" });
    const expectedPath = path.relative(frontend, path.join(root, "prism-fusion/src/web/src/plugin"));
    const wrongPath = path.relative(frontend, path.join(root, "wrong-framework/src/web/src/plugin"));
    const pnpmLock = version => `lockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    dependencies:\n      '@prism-fusion/plugin-runtime':\n        specifier: file:${expectedPath}\n        version: file:${version}\n`;
    await write(frontend, "pnpm-lock.yaml", pnpmLock(wrongPath));
    await assert.rejects(
      verifyConsumerBindings([packageBinding(root)], { frontendDirectory: frontend, packageManager: "pnpm" }),
      /pnpm lock target/i
    );
    await write(frontend, "pnpm-lock.yaml", pnpmLock(expectedPath));
    await assert.doesNotReject(
      verifyConsumerBindings([packageBinding(root)], { frontendDirectory: frontend, packageManager: "pnpm" })
    );
  } finally { await rm(root, { recursive: true, force: true }); }
});

test("shared Vue peers use consumer identity or an effective Vite dedupe rule", async () => {
  for (const dedupe of [["vue", "vue-router"], []]) {
    const root = await mkdtemp(path.join(tmpdir(), "prism-peer-identity-"));
    try {
      const { frontend } = await frontendFixture(root);
      for (const peer of ["vue", "vue-router"]) {
        const metadata = JSON.stringify({ name: peer, version: "1.0.0" });
        await write(frontend, `node_modules/${peer}/package.json`, metadata);
        await write(path.join(root, "prism-fusion/src/web"), `node_modules/${peer}/package.json`, metadata);
      }
      await write(frontend, "node_modules/vite/package.json", JSON.stringify({
        name: "vite", version: "1.0.0", type: "module", exports: "./index.js"
      }));
      await write(frontend, "node_modules/vite/index.js",
        `export async function resolveConfig() { return { resolve: { dedupe: ${JSON.stringify(dedupe)} } }; }\n`);
      const pin = runtimePin(path.join(root, "prism-fusion"), {
        packages: [{
          package: "@prism-fusion/plugin-runtime", path: "src/web/src/plugin", sharedPeers: ["vue", "vue-router"]
        }]
      });
      if (dedupe.length) {
        await assert.doesNotReject(verifyConsumerBindings([pin], { frontendDirectory: frontend, packageManager: "npm" }));
      } else {
        await assert.rejects(
          verifyConsumerBindings([pin], { frontendDirectory: frontend, packageManager: "npm" }),
          /second vue instance|dedupe/i
        );
      }
    } finally { await rm(root, { recursive: true, force: true }); }
  }
});

test("verification-only framework pins need no runtime binding", async () => {
  const pin = {
    name: "prism-fusion", repository: "kwhitestone/prism-fusion", commit: "a".repeat(40),
    role: "verification", directory: "/unused", goModules: [], packages: []
  };
  await assert.doesNotReject(verifyConsumerBindings([pin], {}));
  await assert.rejects(verifyConsumerBindings([pin], { requireFrameworkRuntime: true }), /runtime framework pin/i);
});
