<script setup lang="ts">
/**
 * Builtin Auth 登录表单组件
 *
 * 用户名 + 密码表单，由 auth 插件注入到登录页
 */
import Motion from "@/views/login/utils/motion";
import { useRouter } from "vue-router";
import { message } from "@/utils/message";
import { ref, reactive, watch } from "vue";
import { isAxiosError } from "axios";
import { getConfig } from "@/config";
import { register } from "../api";
import type { FormInstance, FormRules } from "element-plus";
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
  if (!formEl || loading.value || disabled.value) return;
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

// Platform config is loaded before the app mounts. Only explicit true enables UI.
const registerEnabled = getConfig()["register-enabled"] === true;
const activeTab = ref("login");
const registerLoading = ref(false);
const registerFormRef = ref<FormInstance>();
const registerForm = reactive({
  username: "",
  password: "",
  confirmPassword: "",
  nickName: ""
});
const registerRules: FormRules = {
  username: [
    { required: true, message: "请输入用户名", trigger: "blur" },
    { min: 2, max: 64, message: "用户名长度为 2–64 个字符", trigger: "blur" }
  ],
  password: [
    { required: true, message: "请输入密码", trigger: "blur" },
    { min: 6, max: 72, message: "密码长度为 6–72 个字符", trigger: "blur" }
  ],
  confirmPassword: [
    {
      validator: (_rule, value, callback) => {
        if (!value) callback(new Error("请再次输入密码"));
        else if (value !== registerForm.password)
          callback(new Error("两次输入的密码不一致"));
        else callback();
      },
      trigger: "blur"
    }
  ],
  nickName: [{ max: 64, message: "昵称最多 64 个字符", trigger: "blur" }]
};

watch(activeTab, () => {
  registerForm.password = "";
  registerForm.confirmPassword = "";
  registerFormRef.value?.clearValidate();
});

const onRegister = async () => {
  if (!registerEnabled || registerLoading.value || !registerFormRef.value)
    return;
  registerLoading.value = true;
  try {
    const valid = await registerFormRef.value.validate().catch(() => false);
    if (!valid) return;
    const { data } = await register({
      username: registerForm.username,
      password: registerForm.password,
      nickName: registerForm.nickName
    });
    if (data.code !== 0) {
      message(data.message || data.msg || "注册失败，请稍后重试", {
        type: "error"
      });
      return;
    }
    ruleForm.username = registerForm.username;
    ruleForm.password = "";
    activeTab.value = "login";
    message("注册成功，请登录", { type: "success" });
  } catch (error) {
    const data = isAxiosError(error) ? error.response?.data : undefined;
    message(
      data?.detail || data?.message || data?.msg || "注册失败，请检查网络连接",
      {
        type: "error"
      }
    );
  } finally {
    registerLoading.value = false;
  }
};
</script>

<template>
  <el-tabs v-if="registerEnabled" v-model="activeTab" stretch>
    <el-tab-pane
      label="登录"
      name="login"
      :disabled="loading || registerLoading"
    />
    <el-tab-pane
      label="注册账号"
      name="register"
      :disabled="loading || registerLoading"
    />
  </el-tabs>
  <el-form
    v-if="activeTab === 'login'"
    ref="ruleFormRef"
    :model="ruleForm"
    size="large"
    @submit.prevent="onLogin(ruleFormRef)"
  >
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
        native-type="submit"
      >
        登录
      </el-button>
    </Motion>
  </el-form>
  <el-form
    v-else-if="registerEnabled"
    ref="registerFormRef"
    :model="registerForm"
    :rules="registerRules"
    :disabled="registerLoading"
    size="large"
    @submit.prevent="onRegister"
  >
    <el-form-item prop="username">
      <el-input
        v-model="registerForm.username"
        aria-label="用户名"
        placeholder="用户名（2–64 个字符）"
        autocomplete="username"
        prefix-icon="User"
        clearable
      />
    </el-form-item>
    <el-form-item prop="password">
      <el-input
        v-model="registerForm.password"
        aria-label="密码"
        placeholder="密码（6–72 个字符）"
        autocomplete="new-password"
        type="password"
        prefix-icon="Lock"
        show-password
      />
    </el-form-item>
    <el-form-item prop="confirmPassword">
      <el-input
        v-model="registerForm.confirmPassword"
        aria-label="确认密码"
        placeholder="确认密码"
        autocomplete="new-password"
        type="password"
        prefix-icon="Lock"
        show-password
      />
    </el-form-item>
    <el-form-item prop="nickName">
      <el-input
        v-model="registerForm.nickName"
        aria-label="昵称"
        placeholder="昵称（可选）"
        prefix-icon="User"
        clearable
      />
    </el-form-item>
    <el-button
      class="w-full mt-4!"
      size="default"
      type="primary"
      native-type="submit"
      :loading="registerLoading"
      >注册账号</el-button
    >
  </el-form>
</template>
