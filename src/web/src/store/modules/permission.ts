import { defineStore } from "pinia";
import {
  type cacheType,
  store,
  ascending,
  getKeyList,
  filterTree,
  constantMenus,
  filterNoPermissionTree,
  formatFlatteningRoutes
} from "../utils";
import { useMultiTagsStoreHook } from "./multiTags";
import { copyRoutes } from "@/plugin/host-routes";
import { mergeNavigationMetadata } from "@/plugin/navigation";
import type { RouteRecordRaw } from "vue-router";

export const usePermissionStore = defineStore("pure-permission", {
  state: () => ({
    // 静态路由生成的菜单
    constantMenus: copyRoutes(constantMenus),
    // 已声明路由生成的菜单，可叠加后端显示元数据
    wholeMenus: [],
    // 整体路由（一维数组格式）
    flatteningRoutes: [],
    // 缓存页面keepAlive
    cachePageList: []
  }),
  actions: {
    setStaticMenus(routes: RouteRecordRaw[]) {
      this.constantMenus = copyRoutes(routes);
      this.wholeMenus = [];
      this.flatteningRoutes = [];
    },
    /** 组装整体路由生成的菜单 */
    handleWholeMenus(routes: unknown) {
      const menus = ascending(mergeNavigationMetadata(this.constantMenus as RouteRecordRaw[], routes));
      this.wholeMenus = filterNoPermissionTree(
        filterTree(menus)
      );
      this.flatteningRoutes = formatFlatteningRoutes(
        menus
      );
    },
    /** 监听缓存页面是否存在于标签页，不存在则删除 */
    clearCache() {
      let cacheLength = this.cachePageList.length;
      const nameList = getKeyList(useMultiTagsStoreHook().multiTags, "name");
      while (cacheLength > 0) {
        nameList.findIndex(v => v === this.cachePageList[cacheLength - 1]) ===
          -1 &&
          this.cachePageList.splice(
            this.cachePageList.indexOf(this.cachePageList[cacheLength - 1]),
            1
          );
        cacheLength--;
      }
    },
    cacheOperate({ mode, name }: cacheType) {
      const delIndex = this.cachePageList.findIndex(v => v === name);
      switch (mode) {
        case "refresh":
          this.cachePageList = this.cachePageList.filter(v => v !== name);
          this.clearCache();
          break;
        case "add":
          this.cachePageList.push(name);
          break;
        case "delete":
          delIndex !== -1 && this.cachePageList.splice(delIndex, 1);
          this.clearCache();
          break;
      }
    },
    /** 清空缓存页面 */
    clearAllCachePage() {
      this.wholeMenus = [];
      this.cachePageList = [];
    }
  }
});

export function usePermissionStoreHook() {
  return usePermissionStore(store);
}
