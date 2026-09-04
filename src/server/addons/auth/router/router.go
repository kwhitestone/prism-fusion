package router

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/kwhitestone/prism-fusion/addons/auth/service"
)

var (
	jwtService            = &service.JwtService{}
	userService           = &service.UserService{}
	refreshSessionService = &service.RefreshSessionService{}
	authRateLimitService  = &service.AuthRateLimitService{}
)

// ---- Named response data types (avoid Huma "duplicate name: DataStruct") ----

// LoginUserInfo 登录响应中的用户信息
type LoginUserInfo struct {
	ID          uint     `json:"id" doc:"用户ID"`
	Username    string   `json:"username" doc:"用户名"`
	NickName    string   `json:"nickName" doc:"昵称"`
	HeaderImg   string   `json:"headerImg" doc:"头像"`
	RoleID      uint     `json:"roleId" doc:"兼容旧客户端的主角色ID"`
	Roles       []string `json:"roles" doc:"已启用角色编码"`
	Permissions []string `json:"permissions" doc:"当前有效权限"`
}

// LoginData 登录/刷新 Token 响应数据
type LoginData struct {
	AccessToken      string         `json:"accessToken" doc:"访问令牌"`
	RefreshToken     string         `json:"refreshToken,omitempty" doc:"兼容客户端使用的刷新令牌；cookie-only 客户端不返回"`
	ExpiresIn        string         `json:"expiresIn" doc:"访问令牌有效期"`
	RefreshExpiresIn string         `json:"refreshExpiresIn" doc:"刷新会话有效期"`
	User             *LoginUserInfo `json:"user" doc:"用户信息"`
}

// UserInfoData 用户信息响应数据
type UserInfoData struct {
	ID          uint     `json:"id" doc:"用户ID"`
	Username    string   `json:"username" doc:"用户名"`
	NickName    string   `json:"nickName" doc:"昵称"`
	HeaderImg   string   `json:"headerImg" doc:"头像"`
	RoleID      uint     `json:"roleId" doc:"兼容旧客户端的主角色ID"`
	Roles       []string `json:"roles" doc:"已启用角色编码"`
	Permissions []string `json:"permissions" doc:"当前有效权限"`
}

// LoginInput 登录请求体
type LoginInput struct {
	CookieOnly string `header:"X-Refresh-Cookie-Only" doc:"使用 HttpOnly cookie 保存刷新凭据"`
	Body       struct {
		Username string `json:"username" required:"true" minLength:"1" maxLength:"64" doc:"用户名"`
		Password string `json:"password" required:"true" minLength:"1" maxLength:"72" doc:"密码"`
	}
}

// LoginOutput 登录响应体
type LoginOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		Code    int        `json:"code" example:"0" doc:"状态码"`
		Message string     `json:"message" example:"success" doc:"响应消息"`
		Data    *LoginData `json:"data" doc:"登录数据"`
	}
}

// RegisterInput 注册请求体
type RegisterInput struct {
	Body struct {
		Username string `json:"username" required:"true" minLength:"2" maxLength:"64" doc:"用户名"`
		Password string `json:"password" required:"true" minLength:"6" maxLength:"72" doc:"密码"`
		NickName string `json:"nickName" maxLength:"64" doc:"昵称"`
	}
}

// RegisterOutput 注册响应体
type RegisterOutput struct {
	Body struct {
		Code    int    `json:"code" example:"0" doc:"状态码"`
		Message string `json:"message" example:"success" doc:"响应消息"`
	}
}

// RefreshTokenInput 刷新 Token 请求体
type RefreshTokenInput struct {
	CookieOnly    string      `header:"X-Refresh-Cookie-Only"`
	RequestID     string      `header:"X-Refresh-Request-ID" maxLength:"128"`
	RefreshCookie http.Cookie `cookie:"nucleagent_refresh"`
	Body          struct {
		RefreshToken string `json:"refreshToken,omitempty" maxLength:"512" doc:"兼容客户端使用的刷新令牌"`
	}
}

type LogoutInput struct {
	CookieOnly    string      `header:"X-Refresh-Cookie-Only"`
	RefreshCookie http.Cookie `cookie:"nucleagent_refresh"`
	Body          struct {
		RefreshToken string `json:"refreshToken,omitempty" maxLength:"512" doc:"兼容客户端使用的刷新令牌"`
	}
}

type LogoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
}

// UserInfoOutput 用户信息响应
type UserInfoOutput struct {
	Body struct {
		Code    int           `json:"code" example:"0" doc:"状态码"`
		Message string        `json:"message" example:"success" doc:"响应消息"`
		Data    *UserInfoData `json:"data" doc:"用户信息"`
	}
}

// RegisterRoutes 注册 Auth 路由到 Huma
func RegisterRoutes(api huma.API) {
	// 登录
	huma.Register(api, huma.Operation{
		OperationID: "authLogin",
		Method:      http.MethodPost,
		Path:        "/api/v1/addons/auth/login",
		Summary:     "用户登录",
		Description: "使用用户名密码登录，返回 JWT Token",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
		rateSubject, err := userService.LoginRateSubject(input.Body.Username)
		if err != nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "认证服务暂时不可用")
		}
		allowed, err := authRateLimitService.Allow(
			"login-account",
			rateSubject,
			10,
		)
		if err != nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "认证服务暂时不可用")
		}
		if !allowed {
			return nil, huma.NewError(http.StatusTooManyRequests, "登录尝试过于频繁，请稍后重试")
		}
		user, err := userService.Login(input.Body.Username, input.Body.Password)
		if errors.Is(err, service.ErrInvalidCredentials) {
			return nil, huma.NewError(http.StatusUnauthorized, "用户名或密码错误")
		} else if err != nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "认证服务暂时不可用")
		}
		access, err := service.ResolveAuthorization(ctx, user.ID, user.RoleID)
		if err != nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "权限服务暂时不可用")
		}

		refreshToken, familyID, refreshExpiresAt, err := refreshSessionService.IssueWithFamily(user.ID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "刷新会话创建失败")
		}
		token, err := jwtService.GenerateSessionToken(
			user.ID,
			user.Username,
			user.RoleID,
			familyID,
		)
		if err != nil {
			_ = refreshSessionService.Revoke(refreshToken)
			return nil, huma.NewError(http.StatusInternalServerError, "Token 生成失败")
		}

		resp := &LoginOutput{}
		resp.SetCookie = newRefreshCookie(refreshToken, refreshExpiresAt)
		resp.Body.Code = 0
		resp.Body.Message = "登录成功"
		resp.Body.Data = &LoginData{
			AccessToken:      token,
			RefreshToken:     exposedRefreshToken(input.CookieOnly, refreshToken),
			ExpiresIn:        jwtService.AccessExpiresIn(),
			RefreshExpiresIn: refreshSessionService.RefreshExpiresIn(),
			User: &LoginUserInfo{
				ID:          user.ID,
				Username:    user.Username,
				NickName:    user.NickName,
				HeaderImg:   user.HeaderImg,
				RoleID:      user.RoleID,
				Roles:       access.Roles,
				Permissions: access.Permissions,
			},
		}
		return resp, nil
	})

	// 注册
	huma.Register(api, huma.Operation{
		OperationID: "authRegister",
		Method:      http.MethodPost,
		Path:        "/api/v1/addons/auth/register",
		Summary:     "用户注册",
		Description: "注册新用户",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, input *RegisterInput) (*RegisterOutput, error) {
		nickName := input.Body.NickName
		if nickName == "" {
			nickName = input.Body.Username
		}
		_, err := userService.Register(input.Body.Username, input.Body.Password, nickName, 1)
		if errors.Is(err, service.ErrInvalidUserInput) {
			return nil, huma.NewError(http.StatusBadRequest, "注册信息无效")
		} else if errors.Is(err, service.ErrUsernameExists) {
			return nil, huma.NewError(http.StatusConflict, "用户名已存在")
		} else if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "注册服务暂时不可用")
		}
		resp := &RegisterOutput{}
		resp.Body.Code = 0
		resp.Body.Message = "注册成功"
		return resp, nil
	})

	// 刷新 Token
	huma.Register(api, huma.Operation{
		OperationID: "authRefreshToken",
		Method:      http.MethodPost,
		Path:        "/api/v1/addons/auth/refresh-token",
		Summary:     "刷新Token",
		Description: "使用 refresh token 获取新的 access token",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, input *RefreshTokenInput) (*LoginOutput, error) {
		credential := refreshCredential(
			input.CookieOnly,
			input.Body.RefreshToken,
			input.RefreshCookie,
		)
		if !validRefreshRequestID(input.CookieOnly, input.RequestID) {
			return nil, huma.NewError(http.StatusBadRequest, "刷新请求标识无效")
		}
		user, familyID, newRefreshToken, refreshExpiresAt, err := refreshSessionService.RotateForActiveUser(
			credential,
			input.RequestID,
		)
		if errors.Is(err, service.ErrInvalidRefreshToken) ||
			errors.Is(err, service.ErrExpiredRefreshToken) ||
			errors.Is(err, service.ErrRefreshTokenReused) ||
			errors.Is(err, service.ErrRefreshUserUnavailable) {
			return nil, huma.NewError(http.StatusUnauthorized, "刷新令牌无效或已过期")
		} else if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "刷新会话暂时不可用")
		}
		access, err := service.ResolveAuthorization(ctx, user.ID, user.RoleID)
		if err != nil {
			_ = refreshSessionService.Revoke(newRefreshToken)
			return nil, huma.NewError(http.StatusServiceUnavailable, "权限服务暂时不可用")
		}
		newToken, err := jwtService.GenerateSessionToken(
			user.ID,
			user.Username,
			user.RoleID,
			familyID,
		)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "Token 生成失败")
		}
		resp := &LoginOutput{}
		resp.SetCookie = newRefreshCookie(newRefreshToken, refreshExpiresAt)

		resp.Body.Code = 0
		resp.Body.Message = "刷新成功"
		resp.Body.Data = &LoginData{
			AccessToken:      newToken,
			RefreshToken:     exposedRefreshToken(input.CookieOnly, newRefreshToken),
			ExpiresIn:        jwtService.AccessExpiresIn(),
			RefreshExpiresIn: refreshSessionService.RefreshExpiresIn(),
			User: &LoginUserInfo{
				ID:          user.ID,
				Username:    user.Username,
				NickName:    user.NickName,
				HeaderImg:   user.HeaderImg,
				RoleID:      user.RoleID,
				Roles:       access.Roles,
				Permissions: access.Permissions,
			},
		}
		return resp, nil
	})

	// 注销并吊销整个 refresh token family。无论 token 是否存在都返回成功，
	// 避免通过响应差异探测会话。
	huma.Register(api, huma.Operation{
		OperationID: "authLogout",
		Method:      http.MethodPost,
		Path:        "/api/v1/addons/auth/logout",
		Summary:     "注销会话",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, input *LogoutInput) (*LogoutOutput, error) {
		credential := refreshCredential(
			input.CookieOnly,
			input.Body.RefreshToken,
			input.RefreshCookie,
		)
		if err := refreshSessionService.Revoke(credential); err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "注销会话暂时不可用")
		}
		resp := &LogoutOutput{}
		resp.SetCookie = expiredRefreshCookie()
		resp.Body.Code = 0
		resp.Body.Message = "注销成功"
		return resp, nil
	})

	// 获取当前用户信息
	huma.Register(api, huma.Operation{
		OperationID: "authGetUserInfo",
		Method:      http.MethodGet,
		Path:        "/api/v1/addons/auth/user-info",
		Summary:     "获取当前用户信息",
		Description: "根据 Token 获取当前登录用户的信息",
		Tags:        []string{"Auth"},
		Security: []map[string][]string{
			{"AuthTokenAuth": {}},
		},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization" doc:"JWT Token"`
	}) (*UserInfoOutput, error) {
		if input.Authorization == "" {
			return nil, huma.NewError(http.StatusUnauthorized, "未提供 Token")
		}

		// 去除可能的 "Bearer " 前缀
		tokenStr := input.Authorization
		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}

		claims, err := jwtService.ParseAccessToken(tokenStr)
		if err != nil {
			return nil, huma.NewError(http.StatusUnauthorized, "Token 无效或已过期")
		}

		user, err := userService.GetUserByID(claims.UserID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "用户不存在")
		}

		access, err := service.ResolveAuthorization(ctx, user.ID, user.RoleID)
		if err != nil {
			return nil, huma.NewError(http.StatusServiceUnavailable, "权限服务暂时不可用")
		}

		resp := &UserInfoOutput{}
		resp.Body.Code = 0
		resp.Body.Message = "success"
		resp.Body.Data = &UserInfoData{
			ID:          user.ID,
			Username:    user.Username,
			NickName:    user.NickName,
			HeaderImg:   user.HeaderImg,
			RoleID:      user.RoleID,
			Roles:       access.Roles,
			Permissions: access.Permissions,
		}
		return resp, nil
	})
}
