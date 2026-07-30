package model

import (
	"github.com/QuantumNous/new-api/common"
)

// RechargeTier 充值档位。
// 用户端只展示有效档位（is_active=1 且在时间区间内）。
type RechargeTier struct {
	Id            string   `json:"id" gorm:"primaryKey;type:varchar(50)"`
	Name          string   `json:"name" gorm:"type:varchar(100);default:''"`
	Amount        int64    `json:"amount"`
	Price         float64  `json:"price"`
	OriginalPrice *float64 `json:"original_price,omitempty" gorm:"default:null"`
	Bonus         int64    `json:"bonus" gorm:"default:0"`
	IsRecommended bool     `json:"is_recommended" gorm:"default:0"`
	IsActive      bool     `json:"is_active" gorm:"default:1"`
	StartTime     *int64   `json:"start_time,omitempty" gorm:"default:null"`
	EndTime       *int64   `json:"end_time,omitempty" gorm:"default:null"`
	SortOrder     int      `json:"sort_order" gorm:"default:0"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
}

// ---------- 用户端查询 ----------

// GetActiveRechargeTiers 返回当前有效的档位（已上架且在时间区间内），按 sort_order 排序。
func GetActiveRechargeTiers() []*RechargeTier {
	var tiers []*RechargeTier
	now := common.GetTimestamp()
	DB.Where("is_active = ?", true).
		Where("start_time IS NULL OR start_time <= ?", now).
		Where("end_time IS NULL OR end_time >= ?", now).
		Order("sort_order ASC, id ASC").
		Find(&tiers)
	return tiers
}

// ---------- 管理端查询 ----------

// GetAllRechargeTiers 返回全部档位（含已下架/过期），按 sort_order 排序。
func GetAllRechargeTiers() []*RechargeTier {
	var tiers []*RechargeTier
	DB.Order("sort_order ASC, id ASC").Find(&tiers)
	return tiers
}

// GetRechargeTierById 按主键 id 查单个档位。
func GetRechargeTierById(id string) *RechargeTier {
	var tier RechargeTier
	err := DB.Where("id = ?", id).First(&tier).Error
	if err != nil {
		return nil
	}
	return &tier
}

// ---------- 写操作 ----------

// InsertRechargeTier 新增档位。
func InsertRechargeTier(tier *RechargeTier) error {
	now := common.GetTimestamp()
	tier.CreatedAt = now
	tier.UpdatedAt = now
	return DB.Create(tier).Error
}

// UpdateRechargeTier 全量保存档位。
func UpdateRechargeTier(tier *RechargeTier) error {
	tier.UpdatedAt = common.GetTimestamp()
	return DB.Save(tier).Error
}

// DeleteRechargeTierById 按主键删除档位。
func DeleteRechargeTierById(id string) error {
	return DB.Where("id = ?", id).Delete(&RechargeTier{}).Error
}