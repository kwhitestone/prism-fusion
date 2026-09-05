import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const projectDirectory = process.argv[2]
  ? resolve(process.argv[2])
  : fileURLToPath(new URL("..", import.meta.url));
const hostRequire = createRequire(resolve(projectDirectory, "package.json"));
const elementPackagePath = hostRequire.resolve("element-plus/package.json");
const elementRequire = createRequire(elementPackagePath);
// Resolve from Element Plus itself: a hoisted root dependency can be a different version.
const fromPairsPath = elementRequire.resolve("lodash-es/fromPairs.js");
const { default: fromPairs } = await import(pathToFileURL(fromPairsPath).href);
assert.deepEqual(fromPairs([["answer", 42]]), { answer: 42 });
const prototypeInput = fromPairs([["__proto__", { injected: true }]]);
assert.equal(Object.getPrototypeOf(prototypeInput), Object.prototype);
assert.equal(Object.hasOwn(prototypeInput, "__proto__"), true);

const elementPackage = JSON.parse(await readFile(elementPackagePath, "utf8"));
const element = await import(
  pathToFileURL(resolve(dirname(elementPackagePath), elementPackage.module))
    .href
);
assert.equal(typeof element.default.install, "function");
assert.ok(element.ElButton);
const lodashPackage = JSON.parse(
  await readFile(elementRequire.resolve("lodash-es/package.json"), "utf8")
);
console.log(
  `Runtime dependencies OK: Element Plus ${elementPackage.version}, transitive lodash-es ${lodashPackage.version}; fromPairs and ESM import passed`
);
