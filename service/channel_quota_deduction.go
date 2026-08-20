package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
)

// IK9W58: DeductChannelQuota 扣减渠道配额（累计 + 周期 + Redis）
// 在所有需要扣减渠道配额的地方调用此函数。
// groupRatio 为用户分组倍率，渠道限额按上游原始成本（不含折扣）计算：
//
//	originalQuota = quota / groupRatio
//
// 当 groupRatio <= 0 时退化为 quota（避免除零）。
func DeductChannelQuota(channelId int, quota int, groupRatio float64) {
	// 计算上游原始成本（不含用户分组折扣）
	originalQuota := quota
	if groupRatio > 0 {
		originalQuota = int(decimal.NewFromInt(int64(quota)).
			Div(decimal.NewFromFloat(groupRatio)).
			Round(0).
			IntPart())
		if originalQuota < 0 {
			originalQuota = 0
		}
	}

	// 1. 累计配额（硬上限计数源）——按原始成本
	model.UpdateChannelUsedQuota(channelId, originalQuota)

	// 2. 周期配额——按原始成本
	model.UpdateChannelPeriodUsedQuota(channelId, originalQuota)

	// 3. Redis 实时计数（如果可用）
	if common.RedisEnabled {
		if channel, err := model.CacheGetChannel(channelId); err == nil && channel != nil {
			if channel.PeriodQuotaLimit > 0 && channel.PeriodHours > 0 {
				if _, err := IncrChannelPeriodQuota(channelId, channel.PeriodResetTime, channel.PeriodHours, int64(originalQuota)); err != nil {
					common.SysLog("failed to incr channel period quota in Redis: " + err.Error())
				}
			}
		}
	}
}