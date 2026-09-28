package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// MediaKitCost 记录一个火山 AI MediaKit 任务的结算快照，供后期按工具 / 渠道 / 时段
// 与上游账单对账。
//
// 只记网关自己确知的数：哪个工具、上游任务号、预估与实际时长、实扣额度、终态。
// **不记上游成本**——官方计价是「工具 × 输出档（按短边） × 帧率档」的矩阵，且三种
// 计费模式的量纲不同（元/千次、元/分钟、元/GiB），尚未逐条核实。把猜测的费率硬编进
// 代码会把错误固化进已迁移的库，事后修正要写数据订正；所以成本留给对账方用
// 「实际时长 × 官方费率」在 SQL 外算，本表只提供归属与实扣。
//
// 与 enhance_costs（超分专用）一样，本表仅审计：不进用户 quota，不进渠道限额。
type MediaKitCost struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	TaskId    string `json:"task_id" gorm:"type:varchar(191);index"`
	ChannelId int    `json:"channel_id" gorm:"index"`
	// Tool 是上游 slug（如 enhance-video-generative），非对客模型名。
	Tool          string `json:"tool" gorm:"type:varchar(64);index"`
	BillingMode   string `json:"billing_mode" gorm:"type:varchar(16)"`
	UpstreamJobId string `json:"upstream_job_id" gorm:"type:varchar(191);index"`
	// Status 是网关终态：success / failure。超时被清扫的任务同样记 failure。
	Status string `json:"status" gorm:"type:varchar(32)"`
	// EstimatedMinutes 是提交预扣用的预估分钟（时长类工具才有，其余为 0）。
	EstimatedMinutes float64 `json:"estimated_minutes"`
	// ActualSeconds 是上游回包的真实处理时长（秒）。取不到时为 0——
	// 意味着该行无法参与成本估算，而不是时长为零。
	ActualSeconds float64 `json:"actual_seconds"`
	// ChargedQuota 是退款与差额结算之后的最终实扣额度，非预扣值。
	// 失败退款的记 0（上游按官方规则失败不计费）。
	ChargedQuota int `json:"charged_quota"`
}

func (MediaKitCost) TableName() string {
	return "mediakit_costs"
}

func InsertMediaKitCost(cost *MediaKitCost) error {
	if cost == nil {
		return nil
	}
	if cost.CreatedAt == 0 {
		cost.CreatedAt = time.Now().Unix()
	}
	if DB == nil {
		common.SysError("InsertMediaKitCost skipped: DB is nil")
		return nil
	}
	return DB.Create(cost).Error
}
