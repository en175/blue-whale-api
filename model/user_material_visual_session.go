package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// 真人素材认证会话状态。
const (
	VisualSessionProcessing = "processing" // 认证中（H5 未完成或未开始轮询）
	VisualSessionPolling    = "polling"    // 已在回调中轮询拿 GroupId（防并发/重放重复轮询）
	VisualSessionActive     = "active"     // 已拿到 GroupId，真人组可用
	VisualSessionFailed     = "failed"     // 认证失败或 Token 过期
)

// UserMaterialVisualSession 真人素材认证会话：记录用户发起 CreateVisualValidateSession
// 的上下文，供 H5 活体认证完成后回调反查用户、轮询 GetVisualValidateResult 拿 GroupId。
// byted_token 是上游会话标识（回调 query 携带），h5_link 是发给待验证者的页面，
// callback_url 是发起会话时填的回调地址（H5 完成后浏览器跳回）。
type UserMaterialVisualSession struct {
	Id             int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	Provider       string `json:"provider" gorm:"type:varchar(32);not null"` // tokenhub/volcengine
	Name           string `json:"name" gorm:"type:varchar(255)"`             // 用户起的真人组名，认证完成后用作归属组名
	BytedToken     string `json:"byted_token" gorm:"type:varchar(255);uniqueIndex;not null"`
	H5Link         string `json:"h5_link" gorm:"type:varchar(1024)"`
	CallbackURL    string `json:"callback_url" gorm:"type:varchar(1024)"`    // 发给上游的回调地址（网关自身）
	CustomCallback string `json:"custom_callback" gorm:"type:varchar(1024)"` // 客户传入的原回调地址（对外 API，认证后 302 到它；管理端为空）
	ProjectName    string `json:"project_name" gorm:"type:varchar(64)"`
	GroupId        string `json:"group_id" gorm:"type:varchar(128)"`
	Status         string `json:"status" gorm:"type:varchar(16);not null;default:'processing'"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

func (UserMaterialVisualSession) TableName() string {
	return "user_material_visual_sessions"
}

// AddVisualValidateSession 记录用户发起的真人认证会话（观察层在 CreateVisualValidateSession
// 成功后调用）。name 是用户起的真人组名（认证完成后用作归属组名），无则传空串。
// callbackURL 是发给上游的回调地址（网关自身）；customCallback 是客户传入的原回调地址
// （对外 API 场景，认证后 302 到它），无则传空串。
func AddVisualValidateSession(userId int, provider, name, bytedToken, h5Link, callbackURL, projectName, customCallback string) error {
	now := time.Now().Unix()
	return DB.Create(&UserMaterialVisualSession{
		UserId:         userId,
		Provider:       provider,
		Name:           name,
		BytedToken:     bytedToken,
		H5Link:         h5Link,
		CallbackURL:    callbackURL,
		CustomCallback: customCallback,
		ProjectName:    projectName,
		Status:         VisualSessionProcessing,
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error
}

// GetVisualValidateSessionByToken 按 byted_token 反查会话（回调处理用；token 是上游唯一标识）。
func GetVisualValidateSessionByToken(bytedToken string) (*UserMaterialVisualSession, error) {
	var s UserMaterialVisualSession
	if err := DB.Where("byted_token = ?", bytedToken).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// CompleteVisualValidateSession 认证成功拿到 GroupId 后回填，并标记会话状态。
// 记录不存在时返回 ErrRecordNotFound（调用方按需处理）。
func CompleteVisualValidateSession(bytedToken, groupId, status string) error {
	var s UserMaterialVisualSession
	if err := DB.Where("byted_token = ?", bytedToken).First(&s).Error; err != nil {
		return err
	}
	if groupId != "" {
		s.GroupId = groupId
	}
	s.Status = status
	s.UpdatedAt = time.Now().Unix()
	return DB.Save(&s).Error
}

// ClaimVisualValidateSessionPolling 抢注会话的"轮询中"状态：仅 processing 能转 polling。
// 用于回调端点的幂等/去重守卫——同一 bytedToken 只允许一个在途轮询，重放或并发回调不会重复开轮询。
// 返回 true 表示抢注成功（调用方应开始轮询）；false 表示已被其他请求抢注或已终态。
func ClaimVisualValidateSessionPolling(bytedToken string) bool {
	res := DB.Model(&UserMaterialVisualSession{}).
		Where("byted_token = ? AND status = ?", bytedToken, VisualSessionProcessing).
		Updates(map[string]any{"status": VisualSessionPolling, "updated_at": time.Now().Unix()})
	return res.RowsAffected == 1
}

// ErrVisualSessionNotFound 是会话不存在的统一哨兵错误。
var ErrVisualSessionNotFound = errors.New("visual validate session not found")

// IsVisualSessionNotFound 判断错误是否为会话不存在。
func IsVisualSessionNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrVisualSessionNotFound)
}
