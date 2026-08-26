package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kwhitestone/prism-fusion/addons/auth/model"
	"github.com/kwhitestone/prism-fusion/global"

	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

// UserService 用户服务
type UserService struct{}

// LoginRateSubject resolves existing usernames through the same database
// collation used by Login, then rate-limits by stable user ID. This prevents
// case/accent/Unicode aliases accepted by MySQL from creating separate buckets.
func (s *UserService) LoginRateSubject(username string) (string, error) {
	if global.PRISM_DB == nil {
		return "", errors.New("database is unavailable")
	}
	var identity struct{ ID uint }
	err := global.PRISM_DB.Model(&model.User{}).
		Select("id").Where("username = ?", username).Take(&identity).Error
	if err == nil {
		return fmt.Sprintf("user-id:%d", identity.ID), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	canonical := cases.Fold().String(norm.NFKC.String(strings.TrimSpace(username)))
	return "unknown:" + canonical, nil
}

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

// BootstrapAdminFromEnvironment creates an administrator only when the
// operator explicitly supplies one-time credentials. It never logs or embeds
// the password, and it never promotes an existing non-admin account.
func (s *UserService) BootstrapAdminFromEnvironment() error {
	username := strings.TrimSpace(os.Getenv("AUTH_BOOTSTRAP_ADMIN_USERNAME"))
	password := os.Getenv("AUTH_BOOTSTRAP_ADMIN_PASSWORD")
	credentialsProvided := username != "" || password != ""
	if username == "" || password == "" {
		if credentialsProvided {
			return errors.New("both AUTH_BOOTSTRAP_ADMIN_USERNAME and AUTH_BOOTSTRAP_ADMIN_PASSWORD are required")
		}
	}
	if credentialsProvided &&
		(len(username) < 2 || len(username) > 64 || len(password) < 12 || len(password) > 128) {
		return errors.New("bootstrap admin username or password does not meet length requirements")
	}
	if global.PRISM_DB == nil {
		return errors.New("database is unavailable for administrator bootstrap")
	}
	var legacy model.User
	legacyErr := global.PRISM_DB.
		Where("username = ? AND role_id = ?", "admin", 999).
		First(&legacy).Error
	if legacyErr == nil && legacy.CheckPassword("admin123") {
		if credentialsProvided && username == "admin" {
			rotated := legacy
			if err := rotated.SetPassword(password); err != nil {
				return errors.New("failed to hash migrated administrator password")
			}
			return global.PRISM_DB.Transaction(func(tx *gorm.DB) error {
				now := time.Now()
				if err := tx.Model(&model.RefreshSession{}).
					Where("user_id = ? AND revoked_at IS NULL", legacy.ID).
					Update("revoked_at", now).Error; err != nil {
					return err
				}
				return tx.Model(&model.User{}).Where("id = ?", legacy.ID).Updates(map[string]any{
					"password": rotated.Password,
					"enable":   1,
				}).Error
			})
		}
		freezeErr := global.PRISM_DB.Transaction(func(tx *gorm.DB) error {
			now := time.Now()
			if err := tx.Model(&model.RefreshSession{}).
				Where("user_id = ? AND revoked_at IS NULL", legacy.ID).
				Update("revoked_at", now).Error; err != nil {
				return err
			}
			return tx.Model(&model.User{}).Where("id = ?", legacy.ID).Update("enable", 2).Error
		})
		if freezeErr != nil {
			return freezeErr
		}
		return errors.New("legacy default administrator was disabled; set explicit bootstrap credentials for username admin to rotate it")
	}
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return legacyErr
	}
	if !credentialsProvided {
		return nil
	}

	var existing model.User
	err := global.PRISM_DB.Where("username = ?", username).First(&existing).Error
	if err == nil {
		if existing.RoleID == 999 {
			return nil
		}
		return fmt.Errorf("bootstrap username %q already belongs to a non-admin account", username)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	admin := &model.User{
		UUID:     uuid.New().String(),
		Username: username,
		NickName: "Administrator",
		RoleID:   999,
		Enable:   1,
	}
	if err := admin.SetPassword(password); err != nil {
		return errors.New("failed to hash bootstrap administrator password")
	}
	return global.PRISM_DB.Create(admin).Error
}
