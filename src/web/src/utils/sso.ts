import { setAuthToken, setToken, type DataInfo } from "./auth";
import { subBefore, getQueryMap } from "@pureadmin/utils";
import {
  createAuthRequestID,
  invalidateAuthSession,
  observeAuthSession,
  withAuthSessionLock
} from "@/addons/auth/session";

/**
 * 简版前端单点登录，根据实际业务自行编写，平台启动后本地可以跳后面这个链接进行测试 http://localhost:3388/#/permission/page/index?username=sso&roles=admin&accessToken=eyJhbGciOiJIUzUxMiJ9.admin
 * 划重点：
 * 判断是否为单点登录，不为则直接返回不再进行任何逻辑处理，下面是单点登录后的逻辑处理
 * 1.清空本地旧信息；
 * 2.获取url中的重要参数信息，然后通过 setToken 保存在本地；
 * 3.删除不需要显示在 url 的参数
 * 4.使用 window.location.replace 跳转正确页面
 */
(async function () {
  // 获取 url 中的参数
  const params = getQueryMap(location.href) as DataInfo<Date>;
  const must = ["username", "roles", "accessToken"];
  const mustLength = must.length;
  if (Object.keys(params).length !== mustLength) return;

  // url 参数满足 must 里的全部值，才判定为单点登录，避免非单点登录时刷新页面无限循环
  let sso = [];
  let start = 0;

  while (start < mustLength) {
    if (Object.keys(params).includes(must[start]) && sso.length <= mustLength) {
      sso.push(must[start]);
    } else {
      sso = [];
    }
    start++;
  }

  if (sso.length === mustLength) {
    // 判定为单点登录

    const sessionId = createAuthRequestID();
    const accessToken = String(params.accessToken);
    const tokenData: DataInfo<Date> = {
      ...params,
      accessToken,
      refreshToken: "",
      sessionId,
      refreshRequestId: createAuthRequestID(),
      expires: new Date(Date.now() + 15 * 60 * 1000),
      roles: Array.isArray(params.roles)
        ? [...params.roles]
        : String(params.roles)
            .split(",")
            .map(role => role.trim())
            .filter(Boolean)
    };

    await withAuthSessionLock(async () => {
      invalidateAuthSession();
      setAuthToken(`Bearer ${accessToken}`);
      setToken(tokenData);
      observeAuthSession(sessionId);
    });

    // 删除不需要显示在 url 的参数
    delete params.roles;
    delete params.accessToken;

    const newUrl = `${location.origin}${location.pathname}${subBefore(
      location.hash,
      "?"
    )}?${JSON.stringify(params)
      .replace(/["{}]/g, "")
      .replace(/:/g, "=")
      .replace(/,/g, "&")}`;

    // 替换历史记录项
    window.location.replace(newUrl);
  } else {
    return;
  }
})();
