<script setup lang="ts">
/**
 * Builtin Auth 登录表单组件
 *
 * 用户名 + 密码表单，由 auth 插件注入到登录页
 */
import Motion from "@/views/login/utils/motion";
import { useRouter } from "vue-router";
import { message } from "@/utils/message";
import { ref, reactive } from "vue";
import { debounce } from "@pureadmin/utils";
import { useEventListener } from "@vueuse/core";
import type { FormInstance } from "element-plus";
import { useUserStoreHook } from "@/store/modules/user";
import { initRouter, getTopMenu } from "@/router/utils";

const router = useRouter();
const loading = ref(false);
const disabled = ref(false);
const ruleFormRef = ref<FormInstance>();

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
