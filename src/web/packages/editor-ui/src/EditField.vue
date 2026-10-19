<script setup lang="ts">
export interface Choice { value: string | number; label: string; disabled?: boolean }
const props = withDefaults(defineProps<{
  id: string;
  label: string;
  modelValue: unknown;
  kind?: "text" | "textarea" | "boolean" | "select";
  editable?: boolean;
  choices?: Choice[];
  emptyLabel?: string;
  emptyDisabled?: boolean;
  invalid?: boolean;
  rows?: number;
  placeholder?: string;
  required?: boolean;
  controlAttrs?: Record<string, unknown>;
}>(), { kind: "text", editable: true, rows: 6 });
const emit = defineEmits<{ "update:modelValue": [value: string | boolean] }>();
function update(event: Event) {
  if (!props.editable) return;
  const target = event.target as HTMLInputElement;
  emit("update:modelValue", props.kind === "boolean" ? target.checked : target.value);
}
</script>

<template>
  <div class="edit-field" :class="{ toggle: kind === 'boolean' }">
    <label :for="id">{{ label }}</label>
    <input v-if="kind === 'boolean'" :id="id" v-bind="controlAttrs" type="checkbox" role="switch" class="switch"
      :checked="modelValue !== false" :disabled="!editable" :aria-describedby="`${id}-hint`" @change="update">
    <textarea v-else-if="kind === 'textarea'" :id="id" v-bind="controlAttrs" :value="String(modelValue ?? '')"
      :readonly="!editable" :rows="rows" :placeholder="placeholder" :aria-invalid="!!invalid"
      :aria-describedby="`${id}-hint`" @input="update" />
    <select v-else-if="kind === 'select'" :id="id" v-bind="controlAttrs" :value="modelValue ?? ''"
      :disabled="!editable" :required="required" :aria-invalid="!!invalid" :aria-describedby="`${id}-hint`" @change="update">
      <option value="" :selected="modelValue == null || modelValue === ''" :disabled="emptyDisabled">{{ emptyLabel }}</option>
      <option v-for="choice in choices" :key="choice.value" :value="choice.value" :selected="choice.value === modelValue" :disabled="choice.disabled">{{ choice.label }}</option>
    </select>
    <input v-else :id="id" v-bind="controlAttrs" :value="modelValue ?? ''" type="text" :readonly="!editable"
      :required="required" :aria-readonly="!editable || undefined" :aria-invalid="!!invalid" :aria-describedby="`${id}-hint`" @input="update">
    <small :id="`${id}-hint`" :class="{ 'field-error': invalid }"><slot name="hint" /></small>
  </div>
</template>

<style scoped>
.edit-field { display: grid; gap: 6px; min-width: 0; }
label { display: flex; align-items: baseline; flex-wrap: wrap; gap: 4px 10px; font-weight: 600; font-size: 13px; color: var(--text-primary); }
small { font-size: 12px; font-weight: 400; overflow-wrap: anywhere; color: var(--text-secondary); }
input:not([type=checkbox]), textarea, select { width: 100%; box-sizing: border-box; min-width: 0; min-height: 44px; padding: 9px 12px; border: 1px solid var(--border); border-radius: var(--r-md); background: var(--bg-card); color: inherit; font: inherit; font-size: 13.5px; transition: border-color .2s var(--ease), box-shadow .2s var(--ease); }
input:not([type=checkbox]):hover, textarea:hover, select:hover { border-color: var(--border-strong); }
input:not([type=checkbox]):focus, textarea:focus, select:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--teal-50); outline: none; }
.toggle { grid-template-columns: 1fr auto; align-items: center; } .toggle small { grid-column: 1 / -1; }
.switch { appearance: none; width: 44px; height: 24px; margin: 0; border-radius: var(--r-full); background: var(--slate-300); position: relative; cursor: pointer; transition: background .2s var(--ease); }
.switch::after { content: ""; position: absolute; top: 3px; left: 3px; width: 18px; height: 18px; border-radius: 50%; background: #fff; box-shadow: var(--shadow-xs); transition: transform .2s var(--ease); }
.switch:checked { background: var(--accent); } .switch:checked::after { transform: translateX(20px); } .switch:disabled { opacity: .5; cursor: not-allowed; }
textarea { resize: vertical; }
input[readonly], textarea[readonly] { background: var(--bg-subtle); color: var(--text-secondary); }
:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
.field-error, [aria-invalid=true] { color: var(--rose-500); }
input:not([type=checkbox])[aria-invalid=true], textarea[aria-invalid=true], select[aria-invalid=true] { border-color: var(--rose-500); }
@media (max-width: 600px) { input, textarea, select { font-size: 16px !important; } }
</style>
