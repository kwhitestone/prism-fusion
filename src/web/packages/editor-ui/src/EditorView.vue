<script setup lang="ts">
import { ref } from "vue";
import EditorLayout from "./EditorLayout.vue";
import EditSidebar from "./EditSidebar.vue";
import EditorFields, { type EditorField } from "./EditorFields.vue";

export interface EditorGroup {
  id: string;
  label: string;
  dirty?: boolean;
  fields?: EditorField[];
}
const props = withDefaults(defineProps<{
  groups: EditorGroup[];
  active: string;
  ariaLabel: string;
  dirtyLabel?: string;
  idPrefix?: string;
  busy?: boolean;
}>(), { idPrefix: "group-" });
const emit = defineEmits<{
  select: [id: string];
  change: [path: string, value: string | boolean];
  touch: [path: string];
}>();
const body = ref<HTMLElement>();
function select(id: string) {
  if (!props.groups.some(group => group.id === id)) return;
  emit("select", id);
  const section = Array.from(body.value?.querySelectorAll<HTMLElement>("[data-editor-group]") ?? [])
    .find(element => element.dataset.editorGroup === id);
  section?.scrollIntoView({ behavior: "auto", block: "start" });
}
function change(path: string, value: string | boolean) {
  if (!props.busy && props.groups.some(group => group.fields?.some(field => field.path === path && field.editable))) {
    emit("change", path, value);
  }
}
function onScroll() {
  const top = body.value?.getBoundingClientRect().top ?? 0;
  const passed = Array.from(body.value?.querySelectorAll<HTMLElement>("[data-editor-group]") ?? [])
    .filter(element => element.getBoundingClientRect().top - top < 80);
  const id = passed.at(-1)?.dataset.editorGroup;
  if (id && id !== props.active) emit("select", id);
}
</script>

<template>
  <EditorLayout data-editor-view="prism">
    <template #navigation>
      <EditSidebar :groups="groups" :active="active" :aria-label="ariaLabel" :dirty-label="dirtyLabel" @select="select" />
    </template>
    <div ref="body" class="editor-content" @scroll.passive="onScroll">
      <slot name="status" />
      <section v-for="group in groups" :id="`${idPrefix}${group.id}`" :key="group.id"
        :data-editor-group="group.id" class="editor-section" @focusin="emit('select', group.id)">
        <h3>{{ group.label }}</h3>
        <slot name="avatar" :group-id="group.id" :busy="!!busy" />
        <EditorFields v-if="group.fields?.length"
          :fields="group.fields.map(field => ({ ...field, editable: field.editable && !busy }))"
          @change="change" @touch="emit('touch', $event)" />
        <slot name="bindings" :group-id="group.id" :busy="!!busy" />
        <slot name="lifecycle" :group-id="group.id" :busy="!!busy" />
      </section>
    </div>
    <template #overlay><slot name="overlay" /></template>
  </EditorLayout>
</template>

<style scoped>
.editor-content { overflow: auto; padding: 16px 20px 32px; min-width: 0; min-height: 0; }
.editor-section { display: grid; gap: 16px; min-width: 0; padding: 16px 0 20px; border-bottom: 1px solid var(--border); scroll-margin-top: 8px; }
h3 { margin: 0; font-size: 15px; color: var(--text-primary); overflow-wrap: anywhere; }
@media (max-width: 600px) { .editor-content { padding: 12px 16px 24px; } }
</style>
