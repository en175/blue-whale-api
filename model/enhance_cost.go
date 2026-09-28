package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// EnhanceCostSummary is the admin-facing rollup of enhance_costs for one task.
type EnhanceCostSummary struct {
	Status              string
	RequestedResolution string
	UpstreamResolution  string
	AmountFen           int
	AttemptCount        int
}

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

func applyEnhancedOnlyFilter(query *gorm.DB, enabled bool) *gorm.DB {
	if query == nil || !enabled {
		return query
	}
	return query.Where("EXISTS (SELECT 1 FROM enhance_costs WHERE enhance_costs.task_id = tasks.task_id)")
}

// SummarizeEnhanceCostsByTaskIDs rolls up MediaKit attempts for admin task logs.
// A task is enhanced if it has any row; status is succeeded when any row succeeded.
func SummarizeEnhanceCostsByTaskIDs(taskIDs []string) map[string]EnhanceCostSummary {
	out := make(map[string]EnhanceCostSummary)
	if len(taskIDs) == 0 || DB == nil {
		return out
	}
	var rows []EnhanceCost
	if err := DB.Where("task_id IN ?", taskIDs).Order("id asc").Find(&rows).Error; err != nil {
		common.SysError("SummarizeEnhanceCostsByTaskIDs: " + err.Error())
		return out
	}
	for _, row := range rows {
		cur := out[row.TaskId]
		cur.AttemptCount++
		cur.AmountFen += row.AmountFen
		if row.RequestedResolution != "" {
			cur.RequestedResolution = row.RequestedResolution
		}
		if row.UpstreamResolution != "" {
			cur.UpstreamResolution = row.UpstreamResolution
		}
		if row.Status == "succeeded" {
			cur.Status = "succeeded"
		} else if cur.Status == "" {
			cur.Status = "failed"
		}
		out[row.TaskId] = cur
	}
	return out
}
