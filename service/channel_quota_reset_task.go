package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// IK9W58: 渠道周期配额重置任务
// 主节点跑，1 分钟 tick，批量处理 + 行锁

const (
	channelQuotaResetTickInterval = 1 * time.Minute
	channelQuotaResetBatchSize    = 300
)

var (
	channelQuotaResetOnce    sync.Once
	channelQuotaResetRunning atomic.Bool
)

// StartChannelQuotaResetTask 启动渠道周期配额重置任务
// 仅在 master 节点运行
func StartChannelQuotaResetTask() {
	channelQuotaResetOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("channel quota reset task started: tick=%s", channelQuotaResetTickInterval))
			ticker := time.NewTicker(channelQuotaResetTickInterval)
			defer ticker.Stop()

			runChannelQuotaResetOnce()
			for range ticker.C {
				runChannelQuotaResetOnce()
			}
		})
	})
}

func runChannelQuotaResetOnce() {
	if !channelQuotaResetRunning.CompareAndSwap(false, true) {
		return
	}
	defer channelQuotaResetRunning.Store(false)

	ctx := context.Background()
	totalReset := 0
	for {
		n, err := resetDueChannelPeriodQuota(channelQuotaResetBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("channel quota reset task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		totalReset += n
		if n < channelQuotaResetBatchSize {
			break
		}
	}
	if common.DebugEnabled && totalReset > 0 {
		logger.LogDebug(ctx, "channel quota reset: count=%d", totalReset)
	}
}

// resetDueChannelPeriodQuota 重置到期的渠道周期配额
func resetDueChannelPeriodQuota(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := common.GetTimestamp()

	// 查找需要重置的渠道：周期配额 > 0 且下次重置时间已到
	var channels []model.Channel
	if err := model.DB.Where("period_quota_limit > 0 AND period_reset_time > 0 AND period_reset_time <= ?", now).
		Order("period_reset_time asc").
		Limit(limit).
		Find(&channels).Error; err != nil {
		return 0, err
	}

	if len(channels) == 0 {
		return 0, nil
	}

	resetCount := 0
	for _, ch := range channels {
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			// 行锁，防止并发重置
			var locked model.Channel
			if err := tx.Where("id = ? AND period_reset_time > 0 AND period_reset_time <= ?", ch.Id, now).
				First(&locked).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return nil // 已被其他节点重置
				}
				return err
			}

			// 计算下次重置时间
			nextResetTime := now + int64(locked.PeriodHours)*3600

			// 清零周期已用 + 推进重置时间
			if err := tx.Model(&model.Channel{}).Where("id = ?", locked.Id).
				Updates(map[string]interface{}{
					"period_used_quota":  0,
					"period_reset_time": nextResetTime,
				}).Error; err != nil {
				return err
			}

			// 删除旧周期的 Redis key（兜底，新周期 key 不同）
			if common.RedisEnabled {
				if err := model.ResetChannelPeriodQuota(locked.Id, locked.PeriodResetTime); err != nil {
					logger.LogWarn(context.Background(), fmt.Sprintf("failed to reset Redis key for channel %d: %v", locked.Id, err))
				}
			}

			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
	}

	return resetCount, nil
}