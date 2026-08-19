package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// UserMaterialGroup 用户与素材组（上游 seedance asset group）的归属关联。
// 素材组内的具体素材不落库，需要时透传上游查询。为后续用户隔离与素材管理界面提供数据基础。
// 上游 ARK 接口的素材组无状态字段，故本地不镜像状态。
type UserMaterialGroup struct {
	Id                   int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId               int    `json:"user_id" gorm:"index;not null"`
	SeedanceAssetGroupId string `json:"seedance_asset_group_id" gorm:"type:varchar(128);uniqueIndex;not null"`
	Name                 string `json:"name" gorm:"type:varchar(255)"`
	GroupType            string `json:"group_type" gorm:"type:varchar(32)"` // AIGC（当前上游仅支持虚拟组）
	CreatedAt            int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint"`
}

func (UserMaterialGroup) TableName() string {
	return "user_material_groups"
}

// AddUserMaterialGroup upsert 用户↔素材组归属。
// 已存在的组只更新元数据（Name/GroupType），**不覆盖既有归属（UserId）**，
// 避免同一素材组 id 被另一个用户重复创建时发生归属转手。
func AddUserMaterialGroup(userId int, groupId, name, groupType string) error {
	now := time.Now().Unix()
	record := UserMaterialGroup{
		UserId:               userId,
		SeedanceAssetGroupId: groupId,
		Name:                 name,
		GroupType:            groupType,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	// 显式 find→create/update，保持"归属不可转移"这一不变量清晰可见。
	var existing UserMaterialGroup
	err := DB.Where("seedance_asset_group_id = ?", groupId).First(&existing).Error
	if err == nil {
		existing.Name = name
		existing.GroupType = groupType
		existing.UpdatedAt = now
		return DB.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return DB.Create(&record).Error
}

// DeleteUserMaterialGroup 删除素材组归属（上游素材组被删除时同步清理）。
func DeleteUserMaterialGroup(groupId string) error {
	return DB.Where("seedance_asset_group_id = ?", groupId).Delete(&UserMaterialGroup{}).Error
}

// GetMaterialGroups 按用户/类型查询归属列表，供管理界面使用。
// userId<=0 表示不过滤用户（admin 查全部）；groupType 为空表示不过滤类型。
func GetMaterialGroups(userId int, groupType string) ([]*UserMaterialGroup, error) {
	query := DB.Model(&UserMaterialGroup{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if groupType != "" {
		query = query.Where("group_type = ?", groupType)
	}
	var groups []*UserMaterialGroup
	err := query.Order("id DESC").Find(&groups).Error
	return groups, err
}

// GetUserMaterialGroups 返回用户归属的素材组列表（管理界面的"我的素材组"查询）。
func GetUserMaterialGroups(userId int) ([]*UserMaterialGroup, error) {
	return GetMaterialGroups(userId, "")
}

// UserMaterialGroupExists 校验指定用户是否拥有该素材组（管理界面写操作的数据隔离守卫）。
func UserMaterialGroupExists(userId int, groupId string) (bool, error) {
	var count int64
	err := DB.Model(&UserMaterialGroup{}).
		Where("user_id = ? AND seedance_asset_group_id = ?", userId, groupId).
		Count(&count).Error
	return count > 0, err
}
