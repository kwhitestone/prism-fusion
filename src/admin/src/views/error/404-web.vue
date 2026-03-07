<template>
  <div class="error-container">
    <div class="error-content">
      <div class="error-icon">
        <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
        </svg>
      </div>
      <h1 class="error-title">404</h1>
      <p class="error-message">页面不存在</p>
      <p class="error-description">抱歉，您访问的页面不存在或已被删除。</p>
      <div class="error-actions">
        <el-button type="primary" @click="goHome"> 返回首页 </el-button>
        <el-button @click="goBack"> 返回上页 </el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router";
import type { Router } from "vue-router";

defineOptions({
  name: "NotFound"
});

const router: Router = useRouter();

const goHome = (): void => {
  router.push("/");
};

const goBack = (): void => {
  if (window.history.length > 1) {
    router.go(-1);
  } else {
    router.push("/");
  }
};
</script>

<style scoped>
.error-container {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100vh;
}

.error-content {
  text-align: center;
}

.error-icon {
  width: 96px;
  height: 96px;
  margin: 0 auto 24px;
  color: #f59e0b;
}

.error-title {
  margin-bottom: 16px;
  font-size: 72px;
  font-weight: bold;
  color: #1f2937;
}

.error-message {
  margin-bottom: 8px;
  font-size: 24px;
  font-weight: 600;
  color: #374151;
}

.error-description {
  margin-bottom: 32px;
  color: #6b7280;
}

.error-actions {
  display: flex;
  gap: 16px;
  justify-content: center;
}

.error-actions .el-button {
  margin: 0;
}
</style>
