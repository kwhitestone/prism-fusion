import assert from "node:assert/strict";
import test from "node:test";
import type { RouteRecordRaw } from "vue-router";
import { mergeNavigationMetadata } from "./navigation.js";

const View = { render: () => null };
const declared = (): RouteRecordRaw[] => [{
  path: "/orders", name: "Orders", component: View, redirect: "/orders/list",
  meta: { title: "Orders", roles: ["operator"], rank: 2, keepAlive: true },
  children: [{ path: "list", name: "OrderList", component: View,
    meta: { title: "List", auths: ["orders:read"], rank: 1 } }]
}];

test("backend navigation updates only four display metadata fields on declared paths", () => {
  const routes = declared();
  const merged = mergeNavigationMetadata(routes, [{ path: "/orders", name: "Hijacked", component: "attacker.vue", redirect: "/evil",
    meta: { title: "Purchases", icon: "ep:goods", rank: 7, showLink: false, roles: [], auths: ["*"], frameSrc: "https://evil.example", keepAlive: false },
    children: [{ path: "list", name: "WrongName", meta: { title: "Order history" } }, { path: "/injected", component: "evil.vue" }]
  }]);
  assert.equal(merged[0].name, "Orders");
  assert.equal(merged[0].component, View);
  assert.equal(merged[0].redirect, "/orders/list");
  assert.deepEqual(merged[0].meta, { title: "Purchases", icon: "ep:goods", rank: 7, showLink: false, roles: ["operator"], keepAlive: true });
  assert.equal(merged[0].children?.length, 1);
  assert.equal(merged[0].children?.[0].name, "OrderList");
  assert.equal(merged[0].children?.[0].meta?.title, "Order history");
  assert.deepEqual(merged[0].children?.[0].meta?.auths, ["orders:read"]);
  assert.deepEqual(routes, declared());
});

test("unknown backend paths and duplicate metadata never create route or menu records", () => {
  const metadata = [{ path: "/unknown", meta: { title: "Unknown" } },
    { path: "/orders", meta: { title: "First" } }, { path: "/orders", meta: { title: "Duplicate" } }];
  const first = mergeNavigationMetadata(declared(), metadata);
  const second = mergeNavigationMetadata(first, metadata);
  assert.equal(first.length, 1);
  assert.equal(first[0].meta?.title, "First");
  assert.deepEqual(first, second);
});

test("resolved paths match nested relative declarations without name-based matching", () => {
  const merged = mergeNavigationMetadata(declared(), [{ path: "/orders/list/", name: "AnyName", meta: { title: "Recent orders" } },
    { path: "/unknown", name: "Orders", meta: { title: "Wrong" } }]);
  assert.equal(merged[0].meta?.title, "Orders");
  assert.equal(merged[0].children?.[0].meta?.title, "Recent orders");
  assert.equal(merged[0].children?.[0].path, "list");
});

test("invalid remote metadata, external paths and traversal are ignored", () => {
  for (const remote of [null, {}, 3, "routes", [null, 3, {}], [{ path: "https://evil.example/orders" }]]) {
    assert.deepEqual(mergeNavigationMetadata(declared(), remote), declared());
  }
  const invalid = mergeNavigationMetadata(declared(), [{ path: "/orders", meta: { title: {}, icon: false, rank: Infinity, showLink: "true" } },
    { path: "/orders/../orders", meta: { title: "Wrong" } }]);
  assert.deepEqual(invalid, declared());
});

test("output copies metadata while preserving component definitions", () => {
  const input = declared();
  const output = mergeNavigationMetadata(input, []);
  output[0].meta!.roles = ["changed"];
  output[0].children!.length = 0;
  assert.equal(input[0].children?.length, 1);
  assert.deepEqual(input[0].meta?.roles, ["operator"]);
  assert.equal(output[0].component, View);
});

test("backend child injection cannot change declaration hierarchy", () => {
  const root = [...declared(), { path: "/reports", name: "Reports", component: View, meta: { title: "Reports", rank: 1 } } as RouteRecordRaw];
  const output = mergeNavigationMetadata(root, [{ path: "/orders", children: [{ path: "/reports", meta: { title: "Updated" } }] }]);
  assert.equal(output.length, 2);
  assert.equal(output.find(route => route.path === "/orders")?.children?.length, 1);
  assert.equal(output.find(route => route.path === "/reports")?.meta?.title, "Updated");
});

test("cyclic or excessively deep remote input is bounded", () => {
  const cyclic = { path: "/orders", meta: { title: "Safe" }, children: [] as unknown[] };
  cyclic.children.push(cyclic);
  assert.equal(mergeNavigationMetadata(declared(), [cyclic])[0].meta?.title, "Safe");
});
