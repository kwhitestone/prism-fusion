<script setup lang="ts" generic="T extends string">
/**
 * Shared left group navigation for agent editors (admin DefinitionDrawer and user UserAgentWizard).
 * Presentational only: the caller decides which groups exist (admin = all, user = the editable subset)
 * and scrolls to the chosen group; this owns the tab semantics, arrow-key roving focus and look.
 */
import { nextTick, ref } from "vue";

export interface SidebarGroup<K extends string = string> { id: K; label: string; dirty?: boolean }

const props = defineProps<{ groups: SidebarGroup<T>[]; active: T; ariaLabel: string; dirtyLabel?: string }>();
const emit = defineEmits<{ select: [id: T] }>();
const nav = ref<HTMLElement>();

function onKey(event: KeyboardEvent) {
  const step = ["ArrowDown", "ArrowRight"].includes(event.key) ? 1 : ["ArrowUp", "ArrowLeft"].includes(event.key) ? -1 : 0;
  if (!step || !props.groups.length) return;
  event.preventDefault();
  const ids = props.groups.map(group => group.id);
  const next = ids[(ids.indexOf(props.active) + step + ids.length) % ids.length]!;
  emit("select", next);
  void nextTick(() => (nav.value?.querySelector(`[role=tab][data-group="${next}"]`) as HTMLElement | null)?.focus());
}
</script>

<template>
  <nav ref="nav" class="group-nav" role="tablist" aria-orientation="vertical" :aria-label="ariaLabel" data-testid="edit-sidebar" @keydown="onKey">
    <slot name="head" />
    <button v-for="group in groups" :key="group.id" type="button" role="tab" :data-group="group.id" :aria-selected="active === group.id"
      :tabindex="active === group.id ? 0 : -1" @click="emit('select', group.id)">{{ group.label }}<span v-if="group.dirty" class="dirty-dot" :aria-label="dirtyLabel"></span></button>
  </nav>
</template>

<style scoped>
.group-nav { display: flex; flex-direction: column; gap: 2px; padding: 14px 10px; border-right: 1px solid var(--border); background: var(--bg-card); overflow: auto; }
.group-nav button { display: inline-flex; align-items: center; justify-content: space-between; gap: 6px; min-height: 40px; border: 0; border-left: 3px solid transparent; border-radius: var(--r-sm); background: transparent; padding: 8px 10px; color: var(--text-secondary); font: inherit; font-size: 13px; text-align: start; cursor: pointer; transition: all .25s var(--ease); }
.group-nav button:hover { border-color: var(--border-strong); background: var(--bg-hover); color: var(--text-primary); }
.group-nav [aria-selected=true] { border-left-color: var(--accent); background: var(--teal-50); color: var(--teal-700); font-weight: 600; }
.group-nav button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.dirty-dot { display: inline-block; width: 7px; height: 7px; margin-left: 6px; border-radius: 50%; background: var(--amber-500); flex-shrink: 0; animation: scale-in .4s var(--ease-spring) both; }
@media (max-width: 600px) {
  .group-nav { flex-direction: row; overflow-x: auto; padding: 8px 10px; border-right: 0; border-bottom: 1px solid var(--border); scrollbar-width: none; }
  .group-nav button { flex-shrink: 0; border-left: 0; border-radius: var(--r-full); padding: 6px 12px; min-height: 36px; border: 1px solid var(--border); white-space: nowrap; }
  .group-nav [aria-selected=true] { border-color: var(--accent); }
}
@media (prefers-reduced-motion: reduce) { .dirty-dot { animation: none; } }
</style>
