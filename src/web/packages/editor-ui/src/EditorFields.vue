<script setup lang="ts">
import EditField, { type Choice } from "./EditField.vue";

export interface EditorField {
  path: string;
  id: string;
  label: string;
  modelValue: unknown;
  kind?: "text" | "textarea" | "boolean" | "select";
  editable: boolean;
  choices?: Choice[];
  emptyLabel?: string;
  emptyDisabled?: boolean;
  invalid?: boolean;
  rows?: number;
  placeholder?: string;
  required?: boolean;
  controlAttrs?: Record<string, unknown>;
  hint?: string;
  max?: number;
}
const props = defineProps<{ fields: EditorField[] }>();
const emit = defineEmits<{ change: [path: string, value: string | boolean]; touch: [path: string] }>();
function change(path: string, value: string | boolean) {
  if (props.fields.find(field => field.path === path)?.editable) emit("change", path, value);
}
</script>

<template>
  <div class="editor-fields">
    <div v-for="field in fields" :key="field.path" class="editor-field" :data-path="field.path" @focusout="emit('touch', field.path)">
      <EditField v-bind="field" @update:model-value="change(field.path, $event)">
        <template #hint>
          <span class="field-hint"><span>{{ field.hint }}</span>
            <span v-if="field.max" class="field-count">{{ [...String(field.modelValue ?? '')].length }}/{{ field.max }}</span>
          </span>
        </template>
      </EditField>
    </div>
  </div>
</template>

<style scoped>
.editor-fields { display: grid; gap: 16px; min-width: 0; }
.editor-field { min-width: 0; }
.field-hint { display: flex; justify-content: space-between; gap: 8px; }
.field-hint { overflow-wrap: anywhere; }
.field-count { flex-shrink: 0; }
</style>
