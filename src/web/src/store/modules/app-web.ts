import { defineStore } from "pinia";
import { ref, reactive } from "vue";
import type { Ref } from "vue";
import type {
  AppConfig,
  ThemeType,
  ComponentSize,
  Language,
  LayoutMode
} from "@/types";

export const useAppStore = defineStore("app", () => {
  const config = reactive<AppConfig>({
    theme: "light",
    size: "default",
    language: "zh-cn",
    layout: "default",
    sidebarCollapse: false,
    enableTabs: true,
    showBreadcrumb: true,
    enableProgress: true,
    enableKeepAlive: true,
    enableWatermark: false,
    primaryColor: "#409EFF",
    grayMode: false,
    colorWeakness: false
  });

  const isCollapse: Ref<boolean> = ref(false);

  const toggleSidebar = (): void => {
    isCollapse.value = !isCollapse.value;
    config.sidebarCollapse = isCollapse.value;
  };

  const setConfig = <K extends keyof AppConfig>(
    key: K,
    value: AppConfig[K]
  ): void => {
    if (key in config) {
      config[key] = value;
    }
  };

  const setTheme = (theme: ThemeType): void => {
    config.theme = theme;
    document.documentElement.setAttribute("data-theme", theme);
  };

  const setSize = (size: ComponentSize): void => {
    config.size = size;
  };

  const setLanguage = (language: Language): void => {
    config.language = language;
  };

  const setLayout = (layout: LayoutMode): void => {
    config.layout = layout;
  };

  const setPrimaryColor = (color: string): void => {
    config.primaryColor = color;
    document.documentElement.style.setProperty("--el-color-primary", color);
  };

  const setGrayMode = (enabled: boolean): void => {
    config.grayMode = enabled;
    if (enabled) {
      document.documentElement.classList.add("gray-mode");
    } else {
      document.documentElement.classList.remove("gray-mode");
    }
  };

  const setColorWeakness = (enabled: boolean): void => {
    config.colorWeakness = enabled;
    if (enabled) {
      document.documentElement.classList.add("color-weakness");
    } else {
      document.documentElement.classList.remove("color-weakness");
    }
  };

  // 重置配置为默认值
  const resetConfig = (): void => {
    Object.assign(config, {
      theme: "light",
      size: "default",
      language: "zh-cn",
      layout: "default",
      sidebarCollapse: false,
      enableTabs: true,
      showBreadcrumb: true,
      enableProgress: true,
      enableKeepAlive: true,
      enableWatermark: false,
      primaryColor: "#409EFF",
      grayMode: false,
      colorWeakness: false
    });
  };

  return {
    config,
    isCollapse,
    toggleSidebar,
    setConfig,
    setTheme,
    setSize,
    setLanguage,
    setLayout,
    setPrimaryColor,
    setGrayMode,
    setColorWeakness,
    resetConfig
  };
});
