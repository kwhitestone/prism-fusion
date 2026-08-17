package router

import (
	"context"
	"net/http"

	"github.com/kwhitestone/prism-fusion/addons/auth/service"

	"github.com/danielgtaylor/huma/v2"
)

var (
	jwtService  = &service.JwtService{}
	userService = &service.UserService{}
)

// ---- Named response data types (avoid Huma "duplicate name: DataStruct") ----

// LoginUserInfo 登录响应中的用户信息
type LoginUserInfo struct {
	ID        uint   `json:"id" doc:"用户ID"`
	Username  string `json:"username" doc:"用户名"`
	NickName  string `json:"nickName" doc:"昵称"`
	HeaderImg string `json:"headerImg" doc:"头像"`
	RoleID    uint   `json:"roleId" doc:"角色ID"`
}

// LoginData 登录/刷新 Token 响应数据
type LoginData struct {
	AccessToken  string         `json:"accessToken" doc:"访问令牌"`
	RefreshToken string         `json:"refreshToken" doc:"刷新令牌"`
	ExpiresIn    string         `json:"expiresIn" doc:"过期时间"`
	User         *LoginUserInfo `json:"user" doc:"用户信息"`
}

// UserInfoData 用户信息响应数据
type UserInfoData struct {
	ID        uint     `json:"id" doc:"用户ID"`
	Username  string   `json:"username" doc:"用户名"`
	NickName  string   `json:"nickName" doc:"昵称"`
	HeaderImg string   `json:"headerImg" doc:"头像"`
	RoleID    uint     `json:"roleId" doc:"角色ID"`
	Roles     []string `json:"roles" doc:"角色列表"`
}

// LoginInput 登录请求体
type LoginInput struct {
	Body struct {
		Username string `json:"username" required:"true" minLength:"1" doc:"用户名"`
		Password string `json:"password" required:"true" minLength:"1" doc:"密码"`
	}
}

// LoginOutput 登录响应体
type LoginOutput struct {
	Body struct {
		Code    int        `json:"code" example:"0" doc:"状态码"`
		Message string     `json:"message" example:"success" doc:"响应消息"`
		Data    *LoginData `json:"data" doc:"登录数据"`
	}
}

// RegisterInput 注册请求体
type RegisterInput struct {
	Body struct {
		Username string `json:"username" required:"true" minLength:"2" doc:"用户名"`
		Password string `json:"password" required:"true" minLength:"6" doc:"密码"`
		NickName string `json:"nickName" doc:"昵称"`
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
	Body struct {
		RefreshToken string `json:"refreshToken" required:"true" doc:"刷新令牌"`
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
		user, err := userService.Login(input.Body.Username, input.Body.Password)
		if err != nil {
			return nil, huma.NewError(http.StatusUnauthorized, err.Error())
		}

		token, err := jwtService.GenerateToken(user.ID, user.Username, user.RoleID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "Token 生成失败")
		}

		resp := &LoginOutput{}
		resp.Body.Code = 0
		resp.Body.Message = "登录成功"
		resp.Body.Data = &LoginData{
			AccessToken:  token,
			RefreshToken: token, // 简化实现，refresh token 与 access token 相同
			ExpiresIn:    "7d",
			User: &LoginUserInfo{
				ID:        user.ID,
				Username:  user.Username,
				NickName:  user.NickName,
				HeaderImg: user.HeaderImg,
				RoleID:    user.RoleID,
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
		if err != nil {
			return nil, huma.NewError(http.StatusBadRequest, err.Error())
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
		claims, err := jwtService.ParseToken(input.Body.RefreshToken)
		if err != nil {
			return nil, huma.NewError(http.StatusUnauthorized, "Token 无效或已过期")
		}

		newToken, err := jwtService.GenerateToken(claims.UserID, claims.Username, claims.RoleID)
		if err != nil {
			return nil, huma.NewError(http.StatusInternalServerError, "Token 生成失败")
		}
		resp := &LoginOutput{}

		resp.Body.Code = 0
		resp.Body.Message = "刷新成功"
		resp.Body.Data = &LoginData{
			AccessToken:  newToken,
			RefreshToken: newToken,
			ExpiresIn:    "7d",
		}
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

		claims, err := jwtService.ParseToken(tokenStr)
		if err != nil {
			return nil, huma.NewError(http.StatusUnauthorized, "Token 无效或已过期")
		}

		user, err := userService.GetUserByID(claims.UserID)
		if err != nil {
			return nil, huma.NewError(http.StatusNotFound, "用户不存在")
		}

		resp := &UserInfoOutput{}

		// 根据 RoleID 映射角色名
		roles := []string{"user"}
		if user.RoleID == 999 {
			roles = []string{"admin"}
		}

		resp.Body.Code = 0
		resp.Body.Message = "success"
		resp.Body.Data = &UserInfoData{
			ID:        user.ID,
			Username:  user.Username,
			NickName:  user.NickName,
			HeaderImg: user.HeaderImg,
			RoleID:    user.RoleID,
			Roles:     roles,
		}
		return resp, nil
	})
}
