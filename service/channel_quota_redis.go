package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

// IK9W58: 渠道周期配额 Redis 计数封装
// Key 格式: channel:{id}:period:{periodKey}
// periodKey 用于区分不同周期（避免周期重置后 key 冲突）

const channelQuotaRedisPrefix = "channel:%d:period:%s"

// getChannelPeriodKey 生成周期 key
// 使用 PeriodResetTime 作为周期标识，确保周期重置后 key 不同
func getChannelPeriodKey(channelId int, periodResetTime int64) string {
	if periodResetTime <= 0 {
		// 未配置周期或首次使用，使用固定 key
		return fmt.Sprintf(channelQuotaRedisPrefix, channelId, "default")
	}
	return fmt.Sprintf(channelQuotaRedisPrefix, channelId, fmt.Sprintf("%d", periodResetTime))
}

// IncrChannelPeriodQuota 增加渠道周期已用额度（Redis 原子递增）
// 返回递增后的值，如果 Redis 不可用则返回 0, nil
func IncrChannelPeriodQuota(channelId int, periodResetTime int64, periodHours int, delta int64) (int64, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, nil // Redis 不可用，降级到 DB
	}

	key := getChannelPeriodKey(channelId, periodResetTime)
	ctx := context.Background()

	// 原子递增
	result, err := common.RDB.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return 0, fmt.Errorf("Redis INCR failed: %w", err)
	}

	// 如果是新建 key（递增前不存在），设置 TTL
	if result == delta {
		ttl := time.Duration(periodHours) * time.Hour
		if err := common.RDB.Expire(ctx, key, ttl).Err(); err != nil {
			// TTL 设置失败不阻塞，记录日志
			common.SysLog(fmt.Sprintf("failed to set TTL for channel quota key: %v", err))
		}
	}

	return result, nil
}

// GetChannelPeriodQuota 获取渠道周期已用额度（Redis 读）
// 返回 0 如果 key 不存在或 Redis 不可用
func GetChannelPeriodQuota(channelId int, periodResetTime int64) (int64, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, nil // Redis 不可用，降级到 DB 内存
	}

	key := getChannelPeriodKey(channelId, periodResetTime)
	ctx := context.Background()

	val, err := common.RDB.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil // key 不存在，周期内未使用
	}
	if err != nil {
		return 0, fmt.Errorf("Redis GET failed: %w", err)
	}

	return val, nil
}

// ResetChannelPeriodQuota 重置渠道周期已用额度（删除 Redis key）
func ResetChannelPeriodQuota(channelId int, periodResetTime int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil // Redis 不可用，无需操作
	}

	key := getChannelPeriodKey(channelId, periodResetTime)
	ctx := context.Background()

	if err := common.RDB.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("Redis DEL failed: %w", err)
	}

	return nil
}