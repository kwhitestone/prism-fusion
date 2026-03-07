// Package addons 插件导入入口
// 导入所有插件，触发各插件的 init() 函数完成自动注册
// 新增插件只需在此添加 import 即可，无需修改框架其他代码
package addons

import (
	// 认证插件 - 内置 JWT（优先级 10，provider=builtin）
	_ "whitestone.top/prism-fusion/addons/auth"
	// 权限管理插件 - 内置角色管理（优先级 20，provider=builtin）
	_ "whitestone.top/prism-fusion/addons/rbac"
)
