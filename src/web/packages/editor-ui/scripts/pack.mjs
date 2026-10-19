import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync, mkdirSync, renameSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
const root = fileURLToPath(new URL("../", import.meta.url));
const web = resolve(root, "../..");
const hash = (bytes, algorithm = "sha256", encoding = "hex") => createHash(algorithm).update(bytes).digest(encoding);
const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
assert.equal(git("status", "--porcelain", "--untracked-files=all", "--", "."), "", "Commit package sources before packing");
const sourceCommit = git("rev-parse", "HEAD");
execFileSync("npm", ["run", "build:editor"], { cwd: web, stdio: "inherit" });
const files = Object.fromEntries(readdirSync(resolve(root, "dist")).sort().map(name => [
  `dist/${name}`, hash(readFileSync(resolve(root, "dist", name))),
]));
assert.ok(!/\b(?:fetch|WebSocket|XMLHttpRequest)\s*\(/.test(readFileSync(resolve(root, "dist/editor.js"), "utf8")));
const provenance = {
  sourceCommit, coreSource: "21d2c188c1623682b008bc1b360fb01afa36079f",
  builder: { node: process.version, vite: JSON.parse(readFileSync(resolve(web, "node_modules/vite/package.json"))).version,
    lockSha256: hash(readFileSync(resolve(web, "pnpm-lock.yaml"))) }, files,
};
writeFileSync(resolve(root, "PROVENANCE.json"), JSON.stringify(provenance, null, 2) + "\n");
const pack = JSON.parse(execFileSync("npm", ["pack", "--json", "--offline", "--ignore-scripts"], { cwd: root, encoding: "utf8" }))[0];
assert.ok(pack.files.every(file => file.path.startsWith("dist/") || ["package.json", "PROVENANCE.json"].includes(file.path)));
const bytes = readFileSync(resolve(root, pack.filename));
const sha256 = hash(bytes);
assert.equal(pack.integrity, `sha512-${hash(bytes, "sha512", "base64")}`);
mkdirSync(resolve(root, "artifacts"), { recursive: true });
const filename = `editor-ui-${sha256}.tgz`;
renameSync(resolve(root, pack.filename), resolve(root, "artifacts", filename));
writeFileSync(resolve(root, "artifacts/editor-ui.provenance.json"), JSON.stringify({
  ...provenance, name: pack.name, version: pack.version, filename, sha256, integrity: pack.integrity,
}, null, 2) + "\n");
console.log(JSON.stringify({ filename, sha256, integrity: pack.integrity, sourceCommit }));
