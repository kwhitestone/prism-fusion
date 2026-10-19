import assert from "node:assert/strict";
import { test } from "node:test";
import { createSSRApp, createRenderer, h, nextTick, ref } from "vue";
import { renderToString } from "vue/server-renderer";
import { EditorView } from "../dist/editor.js";

const groups = [{ id: "basic", label: "Basic", fields: [
  { path: "/name", id: "name", label: "Name", modelValue: "Synthetic", editable: true },
  { path: "/readonly", id: "readonly", label: "Readonly", modelValue: "Protected", editable: false },
] }];
test("shared view renders fields and scoped extension slots without a host template", async () => {
  const html = await renderToString(createSSRApp({ render: () => h(EditorView, {
    groups, active: "basic", ariaLabel: "Fields",
  }, { bindings: ({ groupId, busy }) => h("span", { "data-binding": groupId }, String(busy)) }) }));
  assert.match(html, /data-editor-view="prism"/);
  assert.match(html, /id="name"/);
  assert.match(html, /id="readonly"[^>]*readonly/);
  assert.match(html, /data-binding="basic">false/);
});
const node = type => ({ type, props: {}, children: [], parent: null });
const renderer = createRenderer({
  createElement: node, createText: text => ({ ...node("#text"), text }), createComment: node,
  setText: (n, text) => { n.text = text; }, setElementText: (n, text) => { n.text = text; },
  patchProp: (n, key, _old, value) => { n.props[key] = value; },
  insert: (n, parent, anchor) => { n.parent = parent; const index = parent.children.indexOf(anchor); parent.children.splice(index < 0 ? parent.children.length : index, 0, n); },
  remove: n => { n.parent?.children.splice(n.parent.children.indexOf(n), 1); },
  parentNode: n => n.parent, nextSibling: () => null,
});
const walk = n => [n, ...n.children.flatMap(walk)];
test("readonly and busy events cannot escape the shared view", async () => {
  const busy = ref(false), changes = [], root = node("root");
  const app = renderer.createApp({ render: () => h(EditorView, {
    groups, active: "basic", ariaLabel: "Fields", busy: busy.value, onChange: (...args) => changes.push(args),
  }) });
  app.mount(root);
  try {
    const input = id => walk(root).find(n => n.type === "input" && n.props.id === id);
    input("name").props.onInput({ target: { value: "Changed" } });
    input("readonly").props.onInput({ target: { value: "Forbidden" } });
    assert.deepEqual(changes, [["/name", "Changed"]]);
    busy.value = true; await nextTick();
    input("name").props.onInput({ target: { value: "Busy" } });
    assert.equal(changes.length, 1);
  } finally { app.unmount(); }
});
