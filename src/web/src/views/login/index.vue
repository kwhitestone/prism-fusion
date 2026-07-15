<script setup lang="ts">
/**
 * 登录页 - 插件化架构
 *
 * 登录表单区域由认证插件动态注入：
 * - builtin auth → 用户名 + 密码表单
 * - oauth auth → OAuth 登录按钮（含回调处理）
 * - 未注册任何插件 → 显示默认 mock 登录表单
 *
 * 认证插件通过 setLoginComponent() 注册自己的登录组件，
 * 登录页只负责渲染，不关心具体认证方式。
 */
import Motion from "./utils/motion";
import { useRouter } from "vue-router";
import { message } from "@/utils/message";
import { ref, reactive, toRaw, computed } from "vue";
import { debounce } from "@pureadmin/utils";
import { useNav } from "@/layout/hooks/useNav";
import { useEventListener } from "@vueuse/core";
import type { FormInstance } from "element-plus";
import { useLayout } from "@/layout/hooks/useLayout";
import { useUserStoreHook } from "@/store/modules/user";
import { useLoginUIStoreHook } from "@/store/modules/loginUI";
import { initRouter, getTopMenu } from "@/router/utils";
import { avatar, illustration } from "./utils/static";
import { useDataThemeChange } from "@/layout/hooks/useDataThemeChange";
import CopyrightFooter from "@/components/CopyrightFooter";

import dayIcon from "@/assets/svg/day.svg?component";
import darkIcon from "@/assets/svg/dark.svg?component";

defineOptions({
  name: "Login"
});

const router = useRouter();
const loading = ref(false);
const disabled = ref(false);
const ruleFormRef = ref<FormInstance>();

const { initStorage } = useLayout();
initStorage();

const { dataTheme, overallStyle, dataThemeChange } = useDataThemeChange();
dataThemeChange(overallStyle.value);
const { title } = useNav();

// 获取认证插件注册的登录组件
const loginComponent = computed(() => useLoginUIStoreHook().loginComponent);
const footerComponent = computed(
  () => useLoginUIStoreHook().footerComponent || CopyrightFooter
);

// ========== 以下为默认 fallback 登录（无插件注册时使用） ==========
const ruleForm = reactive({
  username: "",
  password: ""
});

const onLogin = async (formEl: FormInstance | undefined) => {
  if (!formEl) return;
  await formEl.validate(valid => {
    if (valid) {
      loading.value = true;

      useUserStoreHook()
        .loginByPassword({
          username: ruleForm.username,
          password: ruleForm.password
        })
        .then(res => {
          if (res.success) {
            return initRouter().then(() => {
              disabled.value = true;
              const topPath = getTopMenu(true)?.path || "/welcome";
              router
                .push(topPath)
                .then(() => {
                  message("登录成功", { type: "success" });
                })
                .finally(() => (disabled.value = false));
            });
          } else {
            message(res.message || "登录失败", { type: "error" });
          }
        })
        .catch(() => {
          message("登录失败，请检查网络连接", { type: "error" });
        })
        .finally(() => (loading.value = false));
    }
  });
};

const immediateDebounce: any = debounce(
  (formRef: FormInstance | undefined) => onLogin(formRef),
  1000,
  true
);

useEventListener(document, "keydown", ({ code }) => {
  if (
    ["Enter", "NumpadEnter"].includes(code) &&
    !disabled.value &&
    !loading.value
  )
    immediateDebounce(ruleFormRef.value);
});
</script>

<template>
  <div class="select-none">
    <div class="wave" />
    <div class="flex-c absolute right-5 top-3">
      <!-- 主题 -->
      <el-switch
        v-model="dataTheme"
        inline-prompt
        :active-icon="dayIcon"
        :inactive-icon="darkIcon"
        @change="dataThemeChange"
      />
    </div>
    <div class="login-container">
      <div class="img">
        <component :is="toRaw(illustration)" />
      </div>
      <div class="login-box">
        <div class="login-form">
          <avatar class="avatar" />
          <Motion>
            <h2 class="outline-hidden">{{ title }}</h2>
          </Motion>

          <!-- 插件注入的登录组件 -->
          <component :is="loginComponent" v-if="loginComponent" />

          <!-- 默认 fallback：用户名密码表单（无认证插件注册时使用） -->
          <template v-else>
            <el-form ref="ruleFormRef" :model="ruleForm" size="large">
              <Motion :delay="100">
                <el-form-item
                  :rules="[
                    {
                      required: true,
                      message: '请输入用户名',
                      trigger: 'blur'
                    }
                  ]"
                  prop="username"
                >
                  <el-input
                    v-model="ruleForm.username"
                    clearable
                    placeholder="用户名"
                    prefix-icon="User"
                  />
                </el-form-item>
              </Motion>

              <Motion :delay="150">
                <el-form-item
                  :rules="[
                    {
                      required: true,
                      message: '请输入密码',
                      trigger: 'blur'
                    }
                  ]"
                  prop="password"
                >
                  <el-input
                    v-model="ruleForm.password"
                    clearable
                    show-password
                    type="password"
                    placeholder="密码"
                    prefix-icon="Lock"
                  />
                </el-form-item>
              </Motion>

              <Motion :delay="250">
                <el-button
                  class="w-full mt-4!"
                  size="default"
                  type="primary"
                  :loading="loading"
                  :disabled="disabled"
                  @click="onLogin(ruleFormRef)"
                >
                  登录
                </el-button>
              </Motion>
            </el-form>
          </template>
        </div>
      </div>
    </div>
    <!-- 版权与法规页脚（默认显示版权，业务可通过 setFooterComponent 覆盖） -->
    <div class="login-beian">
      <component :is="footerComponent" />
    </div>
  </div>
</template>

<style scoped>
@import url("@/style/login.css");

.login-beian {
  position: fixed;
  bottom: 12px;
  left: 0;
  width: 100%;
  text-align: center;
  z-index: 1;
}
</style>
