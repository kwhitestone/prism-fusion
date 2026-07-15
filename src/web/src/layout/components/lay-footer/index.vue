<script setup lang="ts">
import { computed } from "vue";
import CopyrightFooter from "@/components/CopyrightFooter";
import { useLoginUIStoreHook } from "@/store/modules/loginUI";

// 业务项目可通过 setFooterComponent() 覆盖此默认页脚
const footerComponent = computed(
  () => useLoginUIStoreHook().footerComponent || CopyrightFooter
);
</script>

<template>
  <footer class="layout-footer">
    <component :is="footerComponent" />
  </footer>
</template>

<style lang="scss" scoped>
.layout-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  padding: 6px 0 10px;
}
</style>
