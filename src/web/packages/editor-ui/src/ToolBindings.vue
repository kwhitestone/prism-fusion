<script setup lang="ts">
export interface BindingToolRow { id: number; name: string; missing: boolean; covered: boolean }
const props = withDefaults(defineProps<{
  rows: BindingToolRow[];
  selected: number[];
  unavailable: number[];
  editable: boolean;
  manageable?: boolean;
  omitCovered?: boolean;
  query: string;
  selectedOnly: boolean;
  searchLabel: string;
  selectedLabel: string;
  coveredLabel: string;
  unavailableLabel: string;
  editLabel: string;
}>(), { manageable: false, omitCovered: false });
const emit = defineEmits<{
  toggle: [id: number, checked: boolean];
  edit: [id: number];
  "update:query": [value: string];
  "update:selectedOnly": [value: boolean];
}>();
const disabled = (row: BindingToolRow) => !props.editable
  || (props.omitCovered && row.covered)
  || ((row.covered || row.missing || props.selected.length >= 50) && !props.selected.includes(row.id));
function toggle(row: BindingToolRow, event: Event) {
  if (!disabled(row)) emit("toggle", row.id, (event.target as HTMLInputElement).checked);
}
function edit(id: number) {
  if (props.manageable && props.rows.some(row => row.id === id && !row.missing)) emit("edit", id);
}
</script>

<template>
  <div class="tool-selector" @focusin.stop>
    <label class="tool-search"><input :value="query" type="search" :aria-label="searchLabel" :placeholder="searchLabel"
      @input="emit('update:query', ($event.target as HTMLInputElement).value)" /></label>
    <label><input :checked="selectedOnly" type="checkbox"
      @change="emit('update:selectedOnly', ($event.target as HTMLInputElement).checked)" />{{ selectedLabel }}</label>
    <ul class="tool-bindings">
      <li v-for="row in rows" :key="row.id">
        <label>
          <input type="checkbox" :checked="selected.includes(row.id) && !(omitCovered && row.covered)"
            :disabled="disabled(row)" @change="toggle(row, $event)" />
          <span>{{ row.name }} <small v-if="row.missing">{{ unavailableLabel }}</small><small v-else-if="row.covered">{{ coveredLabel }}</small></span>
        </label>
        <button v-if="manageable && !row.missing" type="button" :aria-label="`${editLabel} ${row.name}`"
          :title="editLabel" @click="edit(row.id)"><slot name="edit-icon" /></button>
      </li>
    </ul>
    <div v-if="unavailable.length" class="tool-missing">
      <label v-for="id in unavailable" :key="id"><input type="checkbox" checked :disabled="!editable"
        @change="editable && emit('toggle', id, false)" />#{{ id }} <small>{{ unavailableLabel }}</small></label>
    </div>
    <slot name="pager" />
  </div>
</template>

<style scoped>
.tool-bindings { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
.tool-bindings li { display: flex; align-items: center; gap: 8px; }
.tool-bindings li label { flex: 1; min-width: 0; }
label { display: flex; align-items: center; gap: 10px; min-height: 44px; font-size: 13px; cursor: pointer; }
input { width: 18px; height: 18px; flex-shrink: 0; accent-color: var(--accent); }
.tool-search input { width: 100%; box-sizing: border-box; height: 40px; padding: 0 10px; border: 1px solid var(--border); border-radius: var(--r-sm); background: var(--bg-card); color: var(--text-primary); }
.tool-missing { display: flex; flex-wrap: wrap; gap: 12px; }
span { min-width: 0; overflow-wrap: anywhere; }
small { color: var(--text-secondary); font-size: 12px; }
button { display: grid; place-items: center; flex: 0 0 36px; width: 36px; height: 36px; background: var(--bg-card); color: var(--text-primary); border: 1px solid var(--border); border-radius: var(--r-sm); cursor: pointer; }
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
