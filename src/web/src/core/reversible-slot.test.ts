import assert from "node:assert/strict";
import { test } from "node:test";
import { createReversibleSlot } from "./reversible-slot.js";

test("disposers restore the most recent active value and are idempotent", () => {
  const slot = createReversibleSlot("default");
  const removeA = slot.set("a");
  const removeB = slot.set("b");
  assert.equal(slot.get(), "b");
  removeB();
  assert.equal(slot.get(), "a");
  removeB();
  assert.equal(slot.get(), "a");
  removeA();
  assert.equal(slot.get(), "default");
});

test("removing an overridden registration never resurrects it later", () => {
  const slot = createReversibleSlot<string | null>(null);
  const removeA = slot.set("a");
  const removeB = slot.set("b");
  const revision = slot.version;
  removeA();
  assert.equal(slot.get(), "b");
  assert.equal(slot.version, revision);
  removeB();
  assert.equal(slot.get(), null);
  assert.ok(slot.version > revision);
});

test("a disposed or replaced callback result is recognizable even after restoring defaults", async () => {
  const slot = createReversibleSlot(async () => false);
  let finish: (value: boolean) => void;
  const dispose = slot.set(
    () =>
      new Promise<boolean>(resolve => {
        finish = resolve;
      })
  );
  const revision = slot.version;
  const pending = slot.get()();
  dispose();
  finish(true);
  assert.equal(await pending, true);
  assert.notEqual(slot.version, revision);
  assert.equal(await slot.get()(), false);
});
