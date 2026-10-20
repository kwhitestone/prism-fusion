import assert from "node:assert/strict";
import { test } from "node:test";
import { createSSRApp, createRenderer, h, nextTick, ref } from "vue";
import { renderToString } from "vue/server-renderer";
import { EditorView, ToolBindings } from "../dist/editor.js";

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
  assert.match(html, /<input\b[^>]*id="readonly"[^>]*\sreadonly(?:\s|=|>)/);
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
test("binding rows retain cross-page IDs and enforce selection and management gates", async () => {
  const editable = ref(true), manageable = ref(true), toggles = [], edits = [], root = node("root");
  const app = renderer.createApp({ render: () => h(ToolBindings, {
    rows: [{ id: 47, name: "Third page tool", missing: false, covered: false },
      { id: 48, name: "Covered", missing: false, covered: true }],
    selected: [2, 47], unavailable: [2], editable: editable.value, manageable: manageable.value,
    query: "", selectedOnly: false, searchLabel: "Search", selectedLabel: "Selected", editLabel: "Edit",
    coveredLabel: "Covered", unavailableLabel: "Unavailable",
    onToggle: (...args) => toggles.push(args), onEdit: id => edits.push(id),
  }) });
  app.mount(root);
  try {
    const checks = () => walk(root).filter(n => n.type === "input" && n.props.type === "checkbox");
    assert.equal(checks()[1].props.checked, true);
    checks()[1].props.onChange({ target: { checked: false } });
    checks()[2].props.onChange({ target: { checked: true } });
    assert.deepEqual(toggles, [[47, false]]);
    const button = walk(root).find(n => n.type === "button");
    button.props.onClick();
    assert.deepEqual(edits, [47]);
    editable.value = false; manageable.value = false; await nextTick();
    checks()[1].props.onChange({ target: { checked: true } });
    button.props.onClick();
    assert.deepEqual(toggles, [[47, false]]);
    assert.deepEqual(edits, [47]);
    assert.equal(walk(root).some(n => n.type === "button"), false);
  } finally { app.unmount(); }
});
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
test("a tool brought by a selected skill shows ticked and locked; a manual pick survives the skill going away", async () => {
  const rows = ref([{ id: 1, name: "Skill tool", missing: false, covered: true }, { id: 2, name: "Picked", missing: false, covered: true },
    { id: 3, name: "Free", missing: false, covered: false }, { id: 4, name: "Held", missing: true, covered: true }, { id: 5, name: "Held pick", missing: true, covered: true }]);
  const omit = ref(false), toggles = [], root = node("root");
  const app = renderer.createApp({ render: () => h(ToolBindings, {
    rows: rows.value, selected: [2, 5], unavailable: [], editable: true, omitCovered: omit.value,
    query: "", selectedOnly: false, searchLabel: "Search", selectedLabel: "Selected", editLabel: "Edit",
    coveredLabel: "Covered", unavailableLabel: "Unavailable", onToggle: (...args) => toggles.push(args),
  }) });
  app.mount(root);
  try {
    const checks = () => walk(root).filter(n => n.type === "input" && n.props.type === "checkbox").slice(1);
    assert.deepEqual(checks().map(c => [c.props.checked, c.props.disabled]), [[true, true], [true, false], [false, false], [false, true], [true, false]]);
    checks()[0].props.onChange({ target: { checked: false } });
    assert.deepEqual(toggles, []);
    omit.value = true; await nextTick();
    assert.deepEqual(checks().map(c => [c.props.checked, c.props.disabled]), [[true, true], [true, true], [false, false], [false, true], [false, true]]);
    rows.value = rows.value.map(r => ({ ...r, covered: false })); omit.value = false; await nextTick();
    assert.deepEqual(checks().map(c => c.props.checked), [false, true, false, false, true]);
  } finally { app.unmount(); }
});
