<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";

defineProps<{ title: string; keepLabel: string; discardLabel: string; busy?: boolean }>();
const emit = defineEmits<{ keep: []; discard: [] }>();
const dialog = ref<HTMLDialogElement>();
const previousFocus = typeof document === "undefined" ? null : document.activeElement as HTMLElement | null;
onMounted(() => dialog.value?.showModal());
onBeforeUnmount(() => { dialog.value?.close(); previousFocus?.focus?.(); });
</script>

<template>
  <dialog ref="dialog" class="discard-dialog" aria-labelledby="editor-discard-title"
    @cancel.prevent="!busy && emit('keep')" @keydown.esc.stop>
    <h3 id="editor-discard-title">{{ title }}</h3>
    <slot />
    <div class="discard-actions">
      <button type="button" autofocus :disabled="busy" @click="emit('keep')">{{ keepLabel }}</button>
      <button type="button" class="danger" :disabled="busy" @click="emit('discard')">{{ discardLabel }}</button>
    </div>
  </dialog>
</template>

<style scoped>
.discard-dialog { box-sizing: border-box; inset: 0; margin: auto; width: min(440px, calc(100vw - 32px)); padding: 20px; border: 1px solid var(--border); border-radius: 8px; background: var(--bg-card); color: var(--text-primary); box-shadow: var(--shadow-lg); }
.discard-dialog::backdrop { background: rgb(15 23 42 / .35); }
h3 { margin: 0 0 16px; font-size: 16px; overflow-wrap: anywhere; }
.discard-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; }
button { min-height: 44px; padding: 8px 12px; border: 1px solid var(--border); border-radius: 6px; font: inherit; background: var(--bg-card); color: inherit; cursor: pointer; }
.danger { color: var(--rose-500); border-color: currentColor; }
button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
