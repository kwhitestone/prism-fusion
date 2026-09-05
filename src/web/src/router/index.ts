// import "@/utils/sso";
import Cookies from "js-cookie";
import { getConfig } from "@/config";
import NProgress from "@/utils/progress";
import remainingRouter from "./modules/remaining";
import homeRoute from "./modules/home";
import errorRoute from "./modules/error";
import { HostRoutes, type PluginHostOptions } from "@/plugin/host-routes";
import { message } from "@/utils/message";
import { useMultiTagsStoreHook } from "@/store/modules/multiTags";
import { usePermissionStoreHook } from "@/store/modules/permission";
import {
  isUrl,
  openLink,
  isAllEmpty,
  storageLocal
} from "@pureadmin/utils";
import {
  getTopMenu,
  initRouter,
  isOneOfArray,
  getHistoryMode,
  findRouteByPath,
  handleAliveRoute
} from "./utils";
import {
  type Router,
  type RouteRecordRaw,
  createRouter
} from "vue-router";
import {
  type DataInfo,
  userKey,
  endAuthSessionIfCurrent,
  getToken,
  multipleTabsKey
} from "@/utils/auth";
import { useUserStoreHook } from "@/store/modules/user";

/** Explicit shell routes; all addon routes enter through the plugin runtime. */
export const constantRoutes: RouteRecordRaw[] = [homeRoute, errorRoute];
const hostRoutes = new HostRoutes([...constantRoutes, ...remainingRouter as RouteRecordRaw[]]);
export let constantMenus = hostRoutes.getMenus();

/** @deprecated Routes need plugin ownership. Register a PluginModule instead. */
export function registerExternalRoutes(_routes: RouteRecordRaw[]): never {
  throw new Error("registerExternalRoutes is unsupported; use registerExternalPlugins with a PluginModule");
}

export function configurePluginHost(options: PluginHostOptions): void {
  hostRoutes.configure(options);
}

/** Only the loader calls this after all active addons were installed. */
export function commitPluginRoutes(routes: RouteRecordRaw[]): void {
  hostRoutes.commit(routes);
  // Rebuild from the committed snapshot: session cleanup may have reset the
  // live router while an asynchronous addon setup was still running.
  hostRoutes.restore(router);
  constantMenus = hostRoutes.getMenus();
  usePermissionStoreHook().setStaticMenus(constantMenus);
}

export function clearPluginRoutes(): void {
  hostRoutes.clear();
  hostRoutes.restore(router);
  constantMenus = hostRoutes.getMenus();
  usePermissionStoreHook().setStaticMenus(constantMenus);
}

/** 不参与菜单的路由 */
export const remainingPaths = Object.keys(remainingRouter).map(v => {
  return remainingRouter[v].path;
});

/** 创建路由实例 */
export const router: Router = createRouter({
  history: getHistoryMode(import.meta.env.VITE_ROUTER_HISTORY),
  routes: constantRoutes.concat(...(remainingRouter as any)),
  strict: true,
  scrollBehavior(to, from, savedPosition) {
    return new Promise(resolve => {
      if (savedPosition) {
        return savedPosition;
      } else {
        if (from.meta.saveSrollTop) {
          const top: number =
            document.documentElement.scrollTop || document.body.scrollTop;
          resolve({ left: 0, top });
        }
      }
    });
  }
});

/** 记录已经加载的页面路径 */
const loadedPaths = new Set<string>();

/** 重置已加载页面记录 */
export function resetLoadedPaths() {
  loadedPaths.clear();
}

/** 重置路由 */
export function resetRouter() {
  hostRoutes.restore(router);
  usePermissionStoreHook().clearAllCachePage();
  resetLoadedPaths();
}

/** 路由白名单 */
const whiteList = ["/login", "/login/callback"];

const { VITE_HIDE_HOME } = import.meta.env;

router.beforeEach((to: ToRouteType, _from, next) => {
  to.meta.loaded = loadedPaths.has(to.path);

  if (!to.meta.loaded) {
    NProgress.start();
  }

  if (to.meta?.keepAlive) {
    handleAliveRoute(to, "add");
    // 页面整体刷新和点击标签页刷新
    if (_from.name === undefined || _from.name === "Redirect") {
      handleAliveRoute(to);
    }
  }
  const userInfo = storageLocal().getItem<DataInfo<number>>(userKey);
  const externalLink = isUrl(to?.name as string);
  if (!externalLink) {
    to.matched.some(item => {
      if (!item.meta.title) return "";
      const Title = getConfig().Title;
      if (Title) document.title = `${item.meta.title} | ${Title}`;
      else document.title = item.meta.title as string;
    });
  }
  /** 如果已经登录并存在登录信息后不能跳转到路由白名单，而是继续保持在当前页面 */
  function toCorrectRoute() {
    whiteList.includes(to.fullPath) ? next(_from.fullPath) : next();
  }
  if (Cookies.get(multipleTabsKey) && userInfo) {
    // 无权限跳转403页面（early return，避免后续再次调用 next）
    if (to.meta?.roles && !isOneOfArray(to.meta?.roles, userInfo?.roles)) {
      return next({ path: "/error/403" });
    }
    // 开启隐藏首页后在浏览器地址栏手动输入首页welcome路由则跳转到404页面
    if (VITE_HIDE_HOME === "true" && to.fullPath === "/welcome") {
      return next({ path: "/error/404" });
    }
    if (_from?.name) {
      // name为超链接
      if (externalLink) {
        openLink(to?.name as string);
        NProgress.done();
      } else {
        toCorrectRoute();
      }
    } else {
      // 刷新
      if (
        usePermissionStoreHook().wholeMenus.length === 0 &&
        to.path !== "/login"
      ) {
        // 从后端刷新用户信息（头像、角色等）
        useUserStoreHook().fetchUserInfo();
        initRouter().then((router: Router) => {
          if (!useMultiTagsStoreHook().getMultiTagsCache) {
            const { path } = to;
            const route = findRouteByPath(
              path,
              [...router.options.routes]
            );
            getTopMenu(true);
            // query、params模式路由传参数的标签页不在此处处理
            if (route && route.meta?.title) {
              if (isAllEmpty(route.parentId) && route.meta?.backstage) {
                // 此处为动态顶级路由（目录）
                const { path, name, meta } = route.children[0];
                useMultiTagsStoreHook().handleTags("push", {
                  path,
                  name,
                  meta
                });
              } else {
                const { path, name, meta } = route;
                useMultiTagsStoreHook().handleTags("push", {
                  path,
                  name,
                  meta
                });
              }
            }
          }
          // 确保动态路由完全加入路由列表并且不影响静态路由（注意：动态路由刷新时router.beforeEach可能会触发两次，第一次触发动态路由还未完全添加，第二次动态路由才完全添加到路由列表，如果需要在router.beforeEach做一些判断可以在to.name存在的条件下去判断，这样就只会触发一次）
          if (isAllEmpty(to.name)) void router.replace(to.fullPath);
        }).catch(error => {
          console.warn("[Router] Navigation initialization failed", error);
          message("菜单加载失败，请刷新页面重试", { type: "error" });
          NProgress.done();
        });
      }
      toCorrectRoute();
    }
  } else {
    if (to.path !== "/login") {
      if (whiteList.indexOf(to.path) !== -1) {
        next();
      } else {
        void endAuthSessionIfCurrent(getToken()?.sessionId);
        next({ path: "/login" });
      }
    } else {
      next();
    }
  }
});

router.afterEach(to => {
  loadedPaths.add(to.path);
  NProgress.done();
});

export default router;
