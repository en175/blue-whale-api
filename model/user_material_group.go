package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// UserMaterialGroup 用户与素材组（上游 seedance asset group）的归属关联。
// 素材组内的具体素材不落库，需要时透传上游查询。为后续用户隔离与素材管理界面提供数据基础。
// 上游 ARK 接口的素材组无状态字段，故本地不镜像状态。
// Provider 记录该组属于哪个上游（tokenhub/volcengine），切换上游后按当前 provider 过滤，
// 互不干扰。组 id 由各上游生成，跨上游可能碰撞，故唯一约束含 provider。
type UserMaterialGroup struct {
	Id                   int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId               int    `json:"user_id" gorm:"index;not null;uniqueIndex:uniq_user_provider_group,priority:1"`
	Provider             string `json:"provider" gorm:"type:varchar(32);not null;default:'tokenhub';uniqueIndex:uniq_user_provider_group,priority:2"`
	SeedanceAssetGroupId string `json:"seedance_asset_group_id" gorm:"type:varchar(128);not null;uniqueIndex:uniq_user_provider_group,priority:3"`
	Name                 string `json:"name" gorm:"type:varchar(255)"`
	GroupType            string `json:"group_type" gorm:"type:varchar(32)"` // AIGC（当前上游仅支持虚拟组）
	CreatedAt            int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint"`
}

func (UserMaterialGroup) TableName() string {
	return "user_material_groups"
}

// AddUserMaterialGroup upsert 用户↔素材组归属（按 provider 隔离）。
// 已存在的组只更新元数据（Name/GroupType），**不覆盖既有归属（UserId）**，
// 避免同一素材组 id 被另一个用户重复创建时发生归属转手。
func AddUserMaterialGroup(userId int, provider, groupId, name, groupType string) error {
	now := time.Now().Unix()
	record := UserMaterialGroup{
		UserId:               userId,
		Provider:             provider,
		SeedanceAssetGroupId: groupId,
		Name:                 name,
		GroupType:            groupType,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	// 显式 find→create/update，保持"归属不可转移"这一不变量清晰可见。
	var existing UserMaterialGroup
	err := DB.Where("provider = ? AND seedance_asset_group_id = ?", provider, groupId).First(&existing).Error
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

// DeleteUserMaterialGroup 删除素材组归属（上游素材组被删除时同步清理，按 provider 定位）。
func DeleteUserMaterialGroup(provider, groupId string) error {
	return DB.Where("provider = ? AND seedance_asset_group_id = ?", provider, groupId).Delete(&UserMaterialGroup{}).Error
}

// GetMaterialGroups 按用户/provider/类型查询归属列表，供管理界面使用。
// userId<=0 表示不过滤用户（admin 查全部）；provider/groupType 为空表示不过滤对应维度。
func GetMaterialGroups(userId int, provider, groupType string) ([]*UserMaterialGroup, error) {
	query := DB.Model(&UserMaterialGroup{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if provider != "" {
		query = query.Where("provider = ?", provider)
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
	return GetMaterialGroups(userId, "", "")
}

// UserMaterialGroupExists 校验指定用户是否拥有该素材组（管理界面写操作的数据隔离守卫）。
func UserMaterialGroupExists(userId int, provider, groupId string) (bool, error) {
	var count int64
	err := DB.Model(&UserMaterialGroup{}).
		Where("user_id = ? AND provider = ? AND seedance_asset_group_id = ?", userId, provider, groupId).
		Count(&count).Error
	return count > 0, err
}

// GetMaterialGroupType 返回素材组在当前 provider 下的类型（AIGC/LivenessFace），
// 供管理端列表透传给上游 Filter.GroupType（火山管控面 ListAssets 必填；tokenhub 兼容忽略）。
// 组不存在时返回空串。
func GetMaterialGroupType(provider, groupId string) string {
	var g UserMaterialGroup
	if err := DB.Where("provider = ? AND seedance_asset_group_id = ?", provider, groupId).
		First(&g).Error; err != nil {
		return ""
	}
	return g.GroupType
}

// ensureMaterialGroupProviderIndex 清理历史单列唯一索引。唯一约束升级为
// user_id+provider+group_id 复合索引后，AutoMigrate 只加不删，旧单列唯一索引
// （seedance_asset_group_id）残留会把跨 provider 的相同组 id 误判为冲突。
// 不同 GORM 版本/旧模型生成过两个名字（idx_... 前缀与裸字段名），一并清理；
// 三数据库用 GORM Migrator 幂等删除（HasIndex 判定，索引不存在直接跳过）。
func ensureMaterialGroupProviderIndex() error {
	for _, legacyIdx := range []string{
		"idx_user_material_groups_seedance_asset_group_id",
		"seedance_asset_group_id",
	} {
		if DB.Migrator().HasIndex(&UserMaterialGroup{}, legacyIdx) {
			if err := DB.Migrator().DropIndex(&UserMaterialGroup{}, legacyIdx); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetMaterialGroupsPaged 分页查询归属组（管理端组列表）：按用户/provider/类型过滤，
// name 非空时按组名或组 id 模糊匹配（LIKE），返回总数与当前页。page 从 1 起，pageSize 由调用方限界。
func GetMaterialGroupsPaged(userId int, provider, groupType, name string, page, pageSize int) (int64, []*UserMaterialGroup, error) {
	query := DB.Model(&UserMaterialGroup{})
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if groupType != "" {
		query = query.Where("group_type = ?", groupType)
	}
	if name != "" {
		query = applyMaterialNameLike(query, name)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, nil, err
	}
	var groups []*UserMaterialGroup
	err := query.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&groups).Error
	return total, groups, err
}

// applyMaterialNameLike 追加组名/组 id 的 LIKE 模糊匹配过滤，返回新查询。
// 转义 LIKE 通配符（% _）与 ESCAPE 字符 ! 自身，用户输入按字面匹配。
// 用 ! 而非 \ 作 ESCAPE：MySQL 字符串字面量里反斜杠是转义符，ESCAPE '\' 会把闭合引号
// 转义掉导致 SQL 语法错误（1064）——与 model/token.go 的 sanitizeLikePattern 同源约定（三库兼容）。
func applyMaterialNameLike(query *gorm.DB, name string) *gorm.DB {
	escaped := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`).Replace(name)
	pattern := "%" + escaped + "%"
	return query.Where("(name LIKE ? ESCAPE '!' OR seedance_asset_group_id LIKE ? ESCAPE '!')", pattern, pattern)
}
