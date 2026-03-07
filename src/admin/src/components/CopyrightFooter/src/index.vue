<script setup lang="ts">
import { getConfig } from "@/config";

/**
 * 框架默认版权页脚（仅版权信息，不含备案）
 *
 * 业务项目如需显示备案信息，应在业务侧创建自己的 Footer 组件，
 * 通过 setFooterComponent() 注册覆盖此默认组件。
 */
const siteTitle = getConfig("Title") as string;
const copyrightYear = new Date().getFullYear().toString();
</script>

<template>
  <div class="copyright-footer">
    <span>Copyright &copy; {{ copyrightYear }}&nbsp;{{ siteTitle }}</span>
  </div>
</template>

<style lang="scss" scoped>
.copyright-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: rgb(0 0 0 / 45%);

  .dark & {
    color: rgb(220 220 242 / 55%);
  }
}
</style>
