package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// EnhanceCost records one MediaKit 超分 attempt. Amounts are RMB fen and are
// audit-only: they never enter user quota or channel limits.
type EnhanceCost struct {
	Id                  int    `json:"id" gorm:"primaryKey"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint"`
	TaskId              string `json:"task_id" gorm:"type:varchar(191);index"`
	ChannelId           int    `json:"channel_id" gorm:"index"`
	RequestedResolution string `json:"requested_resolution" gorm:"type:varchar(16)"`
	UpstreamResolution  string `json:"upstream_resolution" gorm:"type:varchar(16)"`
	OutputDurationMs    int64  `json:"output_duration_ms"`
	Fps                 int    `json:"fps"`
	RateTier            string `json:"rate_tier" gorm:"type:varchar(16)"`
	AmountFen           int    `json:"amount_fen"`
	MediaKitJobId       string `json:"mediakit_job_id" gorm:"type:varchar(191);index"`
	Status              string `json:"status" gorm:"type:varchar(32)"`
}

func (EnhanceCost) TableName() string {
	return "enhance_costs"
}

func InsertEnhanceCost(cost *EnhanceCost) error {
	if cost == nil {
		return nil
	}
	if cost.CreatedAt == 0 {
		cost.CreatedAt = time.Now().Unix()
	}
	if DB == nil {
		common.SysError("InsertEnhanceCost skipped: DB is nil")
		return nil
	}
	return DB.Create(cost).Error
}
