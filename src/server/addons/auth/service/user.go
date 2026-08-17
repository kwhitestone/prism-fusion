package service

import (
	"errors"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"

	"github.com/google/uuid"
)

// UserService 用户服务
type UserService struct{}

// Login 用户登录
func (s *UserService) Login(username, password string) (*model.User, error) {
	var user model.User
	if err := global.PRISM_DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	if user.Enable == 2 {
		return nil, errors.New("用户已被冻结")
	}

	if !user.CheckPassword(password) {
		return nil, errors.New("密码错误")
	}

	return &user, nil
}

// Register 用户注册
func (s *UserService) Register(username, password, nickName string, roleID uint) (*model.User, error) {
	// 检查用户名是否已存在
	var count int64
	global.PRISM_DB.Model(&model.User{}).Where("username = ?", username).Count(&count)
	if count > 0 {
		return nil, errors.New("用户名已存在")
	}

	user := &model.User{
		UUID:     uuid.New().String(),
		Username: username,
		NickName: nickName,
		RoleID:   roleID,
		Enable:   1,
	}

	if err := user.SetPassword(password); err != nil {
		return nil, errors.New("密码加密失败")
	}

	if err := global.PRISM_DB.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByID 根据 ID 获取用户
func (s *UserService) GetUserByID(id uint) (*model.User, error) {
	var user model.User
	if err := global.PRISM_DB.First(&user, id).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	return &user, nil
}

// GetUserList 获取用户列表
func (s *UserService) GetUserList(page, pageSize int) ([]model.User, int64, error) {
	var users []model.User
	var total int64

	db := global.PRISM_DB.Model(&model.User{})
	db.Count(&total)

	if page > 0 && pageSize > 0 {
		db = db.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	if err := db.Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// ChangePassword 修改密码
func (s *UserService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	var user model.User
	if err := global.PRISM_DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}

	if !user.CheckPassword(oldPassword) {
		return errors.New("原密码错误")
	}

	if err := user.SetPassword(newPassword); err != nil {
		return errors.New("密码加密失败")
	}

	return global.PRISM_DB.Model(&user).Update("password", user.Password).Error
}

// SeedAdminUser 初始化管理员用户（如果不存在）
func (s *UserService) SeedAdminUser() {
	var count int64
	global.PRISM_DB.Model(&model.User{}).Count(&count)
	if count > 0 {
		return
	}

	admin := &model.User{
		UUID:     uuid.New().String(),
		Username: "admin",
		NickName: "超级管理员",
		RoleID:   999,
		Enable:   1,
	}
	_ = admin.SetPassword("admin123")
	global.PRISM_DB.Create(admin)

	global.PRISM_LOG.Info("初始化管理员用户完成: admin / admin123")
}
