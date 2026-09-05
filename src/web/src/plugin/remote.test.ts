import assert from "node:assert/strict";
import test from "node:test";
import { RemoteApplicationRegistry, createRemoteHostChannel, createRemoteChildChannel, REMOTE_PROTOCOL, REMOTE_PROTOCOL_VERSION } from "./remote.js";
import type { RemoteApplicationManifest, RemoteEnvelope } from "./remote.js";

const app = (id = "orders"): RemoteApplicationManifest => ({
  apiVersion: "prism-fusion/v2", kind: "remote-application", id, version: "1.0.0",
  entryUrl: `http://localhost:26630/${id}/`, allowedOrigins: ["http://localhost:26630"],
  protocolVersion: 1, routeScopes: [`/${id}`],
  routes: [{ path: `/${id}`, name: `${id}-home`, childPath: "/home" }, { path: `/${id}/:id`, name: `${id}-item`, childPath: "/items/:id" }],
  messages: { toChild: ["auth", "view"], fromChild: ["login-request", "items"] }
});

test("remote registry validates and freezes immutable application route ownership", () => {
  const registry = new RemoteApplicationRegistry();
  const original = app();
  registry.register([original]);
  original.routes[0].childPath = "/tampered";
  assert.equal(registry.resolve("/orders")?.childPath, "/home");
  assert.equal(registry.resolve("/orders/abc")?.childPath, "/items/abc");
  assert.equal(registry.resolve("/unknown"), undefined);
  assert.equal(registry.resolve("//evil.test"), undefined);
  const snapshot = registry.getApplications();
  snapshot[0].entryUrl = "https://evil.test";
  assert.equal(registry.resolve("/orders")?.application.entryUrl, "http://localhost:26630/orders/");
  assert.throws(() => registry.register([app("other")]), /frozen/i);
});

test("remote registration is atomic and rejects malformed origins, identities and capabilities", () => {
  for (const patch of [
    { entryUrl: "javascript:alert(1)" }, { entryUrl: "http://user:password@localhost:26630" },
    { allowedOrigins: ["*"] }, { allowedOrigins: ["https://other.test"] },
    { protocolVersion: 2 }, { version: "latest" }, { kind: "frontend-addon" },
    { messages: { toChild: ["host:init"], fromChild: [] } },
    { routes: [{ path: "/orders", name: "orders", childPath: "https://evil.test" }] }
  ]) {
    const registry = new RemoteApplicationRegistry();
    assert.throws(() => registry.register([app("first"), { ...app(), ...patch } as RemoteApplicationManifest]));
    assert.equal(registry.getApplications().length, 0);
  }
});

test("remote route conflicts and undeclared namespaces fail before loading frames", () => {
  const registry = new RemoteApplicationRegistry();
  registry.register([app()]);
  assert.throws(() => registry.register([app()]), /duplicate/i);
  assert.throws(() => registry.register([{ ...app("other"), routeScopes: ["/orders"], routes: [{ path: "/orders/:value", name: "other", childPath: "/" }] }]), /collision/i);
  assert.throws(() => registry.register([{ ...app("other"), routes: [{ path: "/escape", name: "escape", childPath: "/" }] }]), /scope/i);
});

test("remote dependencies order applications and reject missing or conflicting apps", () => {
  const registry = new RemoteApplicationRegistry();
  registry.register([{ ...app("reports"), requires: [{ id: "orders" }] }, app()]);
  assert.deepEqual(registry.freeze().map(item => item.id), ["orders", "reports"]);
  const missing = new RemoteApplicationRegistry();
  missing.register([{ ...app(), requires: [{ id: "missing" }] }]);
  assert.throws(() => missing.freeze(), /missing/);
});

function channels() {
  const parentWindow = { postMessage: (data: unknown, origin: string) => { toParent.push({ data, origin }); } };
  const childWindow = { postMessage: (data: unknown, origin: string) => { toChild.push({ data, origin }); } };
  const toParent: { data: unknown; origin: string }[] = [];
  const toChild: { data: unknown; origin: string }[] = [];
  const parentMessages: unknown[] = [];
  const childMessages: unknown[] = [];
  const host = createRemoteHostChannel({ application: app(), target: childWindow, onMessage: (type, payload) => parentMessages.push({ type, payload }) });
  const child = createRemoteChildChannel({ appId: "orders", hostOrigin: "http://localhost:26600", parent: parentWindow, messages: app().messages, onMessage: (type, payload) => childMessages.push({ type, payload }) });
  const feedChild = () => child.receive({ source: parentWindow, origin: "http://localhost:26600", data: toChild.shift()!.data });
  const feedParent = () => host.receive({ source: childWindow, origin: "http://localhost:26630", data: toParent.shift()!.data });
  return { host, child, childWindow, parentWindow, toChild, toParent, parentMessages, childMessages, feedChild, feedParent };
}

test("remote handshake waits for child mount before sending credentials or business messages", () => {
  const pair = channels();
  assert.equal(pair.host.send("auth", { token: "test-only" }), false);
  pair.host.start();
  assert.equal(pair.host.state, "connecting");
  pair.feedChild();
  assert.equal(pair.toParent.length, 0);
  pair.child.ready();
  pair.feedParent();
  assert.equal(pair.host.state, "ready");
  assert.equal(pair.host.send("auth", { token: "test-only" }), true);
  pair.feedChild();
  assert.deepEqual(pair.childMessages, [{ type: "auth", payload: { token: "test-only" } }]);
  assert.equal(pair.child.send("items", [1]), true);
  pair.feedParent();
  assert.deepEqual(pair.parentMessages, [{ type: "items", payload: [1] }]);
  pair.host.dispose(); pair.child.dispose();
});

test("remote transport rejects wrong origin, source, app, instance and protocol", () => {
  const pair = channels();
  pair.child.ready(); pair.host.start(); pair.feedChild();
  const ready = pair.toParent.shift()!.data as RemoteEnvelope;
  for (const change of [
    { origin: "https://evil.test" }, { source: {} },
    { data: { ...ready, appId: "other" } }, { data: { ...ready, instanceId: "old-instance" } },
    { data: { ...ready, protocol: "other" } }
  ]) {
    assert.equal(pair.host.receive({ source: pair.childWindow, origin: "http://localhost:26630", data: ready, ...change }), false);
    assert.equal(pair.host.state, "connecting");
  }
  pair.host.receive({ source: pair.childWindow, origin: "http://localhost:26630", data: ready });
  assert.equal(pair.host.state, "ready");
  const old = ready;
  pair.host.start();
  assert.equal(pair.host.receive({ source: pair.childWindow, origin: "http://localhost:26630", data: old }), false);
  assert.equal(pair.host.state, "connecting");
  pair.host.dispose(); pair.child.dispose();
});

test("remote capability allowlists reject unregistered messages in both directions", () => {
  const pair = channels();
  pair.child.ready(); pair.host.start(); pair.feedChild(); pair.feedParent();
  assert.equal(pair.host.send("delete-everything", {}), false);
  assert.equal(pair.child.send("admin", {}), false);
  const forged = { protocol: REMOTE_PROTOCOL, version: REMOTE_PROTOCOL_VERSION, appId: "orders", instanceId: pair.host.instanceId, type: "admin", payload: {} };
  assert.equal(pair.host.receive({ source: pair.childWindow, origin: "http://localhost:26630", data: forged }), false);
  assert.equal(pair.parentMessages.length, 0);
  pair.host.dispose(); pair.child.dispose();
});

test("remote incompatible versions fail closed and timeout is recoverable", async () => {
  const states: string[] = [];
  const target = { postMessage() {} };
  const host = createRemoteHostChannel({ application: app(), target, timeoutMs: 5, onStateChange: state => states.push(state) });
  host.start();
  host.receive({ source: target, origin: "http://localhost:26630", data: { protocol: REMOTE_PROTOCOL, version: 99, appId: "orders", instanceId: host.instanceId, type: "child:ready" } });
  assert.equal(host.state, "failed");
  host.start();
  await new Promise(resolve => setTimeout(resolve, 15));
  assert.equal(host.state, "failed");
  assert.deepEqual(states, ["connecting", "failed", "connecting", "failed"]);
  host.dispose();
  assert.equal(host.state, "disposed");
  assert.throws(() => host.start(), /disposed/);
});
