<script setup lang="ts">
/**
 * OAuth2 回调页面（通用壳）
 *
 * History 模式下，OAuth Provider 重定向到 /login/callback?code=xxx
 * 此页面渲染认证插件注册的登录组件，由插件组件自行检测 code 并处理回调。
 *
 * 如果没有插件注册登录组件，显示加载状态后跳转到 /login。
 */
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useLoginUIStoreHook } from "@/store/modules/loginUI";

defineOptions({
  name: "LoginCallback"
});

const router = useRouter();
const loginComponent = computed(() => useLoginUIStoreHook().loginComponent);
const noPlugin = ref(false);

onMounted(() => {
  // 如果没有认证插件注册登录组件，回退到登录页
  if (!loginComponent.value) {
    noPlugin.value = true;
    setTimeout(() => router.push("/login"), 1000);
  }
});
</script>

<template>
  <!-- 渲染插件的登录组件（组件内部自行检测 code 并处理回调） -->
  <component :is="loginComponent" v-if="loginComponent" />

  <!-- 无插件时的 fallback -->
  <div v-else class="callback-container">
    <div class="callback-box">
      <el-icon class="is-loading callback-icon">
        <svg viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg">
          <path
            d="M512 64a32 32 0 0 1 32 32v192a32 32 0 0 1-64 0V96a32 32 0 0 1 32-32zm0 640a32 32 0 0 1 32 32v192a32 32 0 1 1-64 0V736a32 32 0 0 1 32-32zm448-192a32 32 0 0 1-32 32H736a32 32 0 1 1 0-64h192a32 32 0 0 1 32 32zm-640 0a32 32 0 0 1-32 32H96a32 32 0 0 1 0-64h192a32 32 0 0 1 32 32z"
            fill="currentColor"
          />
        </svg>
      </el-icon>
      <p class="callback-text">
        {{ noPlugin ? "未检测到认证插件，正在跳转..." : "正在处理登录..." }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.callback-container {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100vh;
  background: var(--el-bg-color);
}

.callback-box {
  text-align: center;
}

.callback-icon {
  font-size: 48px;
  color: var(--el-color-primary);
  animation: rotating 1.5s linear infinite;
}

.callback-text {
  margin-top: 16px;
  font-size: 16px;
  color: var(--el-text-color-secondary);
}

@keyframes rotating {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}
</style>
