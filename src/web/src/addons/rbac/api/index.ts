import service from "@/utils/request";

const RBAC_BASE = "/api/v1/addons/rbac";

/** 获取动态路由 */
export const getAsyncRoutes = () => {
  return service({
    url: `${RBAC_BASE}/async-routes`,
    method: "get",
    donNotShowLoading: true
  });
};

/** 获取角色列表 */
export const getRoleList = () => {
  return service({
    url: `${RBAC_BASE}/roles`,
    method: "get"
  });
};

/** 获取当前用户权限 */
export const getUserPermissions = (roleId?: number) => {
  return service({
    url: `${RBAC_BASE}/permissions`,
    method: "get",
    params: roleId ? { roleId } : {}
  });
};
