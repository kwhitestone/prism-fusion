<template>
  <div class="editor-layout">
    <slot name="navigation" />
    <slot />
    <slot name="overlay" />
  </div>
</template>

<style scoped>
.editor-layout { position: relative; display: grid; grid-template-columns: 176px minmax(0, 1fr); flex: 1; min-height: 0; }
@media (max-width: 600px) {
  .editor-layout { grid-template-columns: minmax(0, 1fr); grid-template-rows: auto minmax(0, 1fr); }
}
</style>
