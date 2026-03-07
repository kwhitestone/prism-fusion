import { defineStore } from "pinia";
import { store } from "../utils";
import { shallowRef, type Component } from "vue";

/**
 * 登录 UI 插件化
 *
 * 认证插件通过 setLoginComponent() 注册自己的登录界面组件：
 * - builtin auth → 用户名密码表单
 * - oauth auth → OAuth 登录按钮 / 自动跳转
 * - 未来的其他认证插件 → 各自的登录 UI
 *
 * 业务项目通过 setFooterComponent() 注册登录页底部页脚（如备案信息）
 *
 * 登录页通过 useLoginUIStore().loginComponent 动态渲染
 */

const _loginComponent = shallowRef<Component | null>(null);
const _footerComponent = shallowRef<Component | null>(null);

/** 设置登录组件（由认证插件调用） */
export function setLoginComponent(component: Component) {
  _loginComponent.value = component;
}

/** 设置登录页底部页脚组件（由业务项目调用，如备案信息） */
export function setFooterComponent(component: Component) {
  _footerComponent.value = component;
}

export const useLoginUIStore = defineStore("login-ui", {
  state: () => ({
    /** 认证插件注册的登录组件 */
    loginComponent: _loginComponent,
    /** 业务项目注册的页脚组件 */
    footerComponent: _footerComponent
  })
});

export function useLoginUIStoreHook() {
  return useLoginUIStore(store);
}
