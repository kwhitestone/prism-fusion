package model

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 用户表
type User struct {
	ID        uint           `json:"id" gorm:"primarykey;comment:主键ID"`
	CreatedAt *LocalTime     `json:"createdAt" gorm:"column:created_at;comment:创建时间"`
	UpdatedAt *LocalTime     `json:"updatedAt" gorm:"column:updated_at;comment:更新时间"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index;comment:删除时间"`
	UUID      string         `json:"uuid" gorm:"column:uuid;size:50;index;comment:用户UUID"`
	Username  string         `json:"username" gorm:"column:username;size:100;uniqueIndex;comment:用户登录名"`
	Password  string         `json:"-" gorm:"column:password;comment:用户登录密码"`
	NickName  string         `json:"nickName" gorm:"column:nick_name;comment:用户昵称"`
	HeaderImg string         `json:"headerImg" gorm:"column:header_img;comment:用户头像"`
	Phone     string         `json:"phone" gorm:"column:phone;comment:用户手机号"`
	Email     string         `json:"email" gorm:"column:email;comment:用户邮箱"`
	Enable    int            `json:"enable" gorm:"column:enable;default:1;comment:用户是否被冻结 1正常 2冻结"`
	RoleID    uint           `json:"roleId" gorm:"column:role_id;comment:用户角色ID"`
}

// SetPassword 设置密码（bcrypt 哈希）
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

// CheckPassword 验证密码
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}
