package model

import (
	"context"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IK9W58: ChannelIsQuotaExceeded 回归测试。
// 判定在内存快照基础上叠加尚未批量落库的增量，堵住 BATCH_UPDATE_INTERVAL
// 窗口内的继续放行。这里直接操作 batchUpdateStores 构造「未落库」状态。
func TestChannelIsQuotaExceeded(t *testing.T) {
	const (
		hardChannelID   = 90001
		periodChannelID = 90002
	)

	// setPending 在锁内写入「未落库」增量，避免与批量落库并发时产生数据竞争。
	setPending := func(type_, id, value int) {
		batchUpdateLocks[type_].Lock()
		batchUpdateStores[type_][id] = value
		batchUpdateLocks[type_].Unlock()
	}

	t.Cleanup(func() {
		for _, kv := range [...]struct{ typ, id int }{
			{BatchUpdateTypeChannelUsedQuota, hardChannelID},
			{BatchUpdateTypeChannelPeriodUsedQuota, periodChannelID},
		} {
			batchUpdateLocks[kv.typ].Lock()
			delete(batchUpdateStores[kv.typ], kv.id)
			batchUpdateLocks[kv.typ].Unlock()
		}
	})

	tests := []struct {
		name        string
		channel     *Channel
		hardDelta   int // 未落库的累计增量（硬上限）
		periodDelta int // 未落库的周期增量
		want        bool
	}{
		{
			name: "nil channel never exceeded",
			want: false,
		},
		{
			name: "no limits configured ignores pending",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      100,
				HardQuotaLimit: 0,
			},
			hardDelta: 10,
			want:      false,
		},
		{
			name: "hard below limit without pending",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      90,
				HardQuotaLimit: 100,
			},
			want: false,
		},
		{
			name: "hard at limit without pending",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      100,
				HardQuotaLimit: 100,
			},
			want: true,
		},
		{
			name: "hard at limit with pending stays exceeded",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      100,
				HardQuotaLimit: 100,
			},
			hardDelta: 5,
			want:      true,
		},
		{
			name: "hard below limit pushed over by pending",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      90,
				HardQuotaLimit: 100,
			},
			hardDelta: 15,
			want:      true,
		},
		{
			name: "hard plus pending still below limit",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      80,
				HardQuotaLimit: 100,
			},
			hardDelta: 10,
			want:      false,
		},
		{
			// 极限值：used 距上限一步之遥，加上 pending 溢出 int64。
			// 应判定超限（fail-closed），不允许溢出回绕导致漏拦。
			name: "hard used plus pending int64 overflow is exceeded",
			channel: &Channel{
				Id:             hardChannelID,
				UsedQuota:      math.MaxInt64 - 5,
				HardQuotaLimit: math.MaxInt64,
			},
			hardDelta: 10,
			want:      true,
		},
		{
			name: "period below limit without pending",
			channel: &Channel{
				Id:               periodChannelID,
				PeriodUsedQuota:  40,
				PeriodQuotaLimit: 50,
			},
			want: false,
		},
		{
			name: "period at limit without pending",
			channel: &Channel{
				Id:               periodChannelID,
				PeriodUsedQuota:  50,
				PeriodQuotaLimit: 50,
			},
			want: true,
		},
		{
			name: "period below limit pushed over by pending",
			channel: &Channel{
				Id:               periodChannelID,
				PeriodUsedQuota:  40,
				PeriodQuotaLimit: 50,
			},
			periodDelta: 20,
			want:        true,
		},
		{
			name: "period plus pending still below limit",
			channel: &Channel{
				Id:               periodChannelID,
				PeriodUsedQuota:  30,
				PeriodQuotaLimit: 50,
			},
			periodDelta: 10,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setPending(BatchUpdateTypeChannelUsedQuota, hardChannelID, tt.hardDelta)
			setPending(BatchUpdateTypeChannelPeriodUsedQuota, periodChannelID, tt.periodDelta)

			got := ChannelIsQuotaExceeded(tt.channel)
			assert.Equal(t, tt.want, got)
		})
	}
}

// IK9W58: ChannelIsQuotaExceeded Redis 优先判定回归测试。
// 周期限额判定应优先采用 Redis 实时计数（近实时、含未批量落库增量）；
// Redis 未启用 / 读失败 / key 不存在时回退内存快照 + 未批量落库增量。
func TestChannelIsQuotaExceededRedisPriority(t *testing.T) {
	mr := miniredis.RunT(t)

	oldRDB, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	t.Cleanup(func() {
		common.RDB.Close()
		common.RDB = oldRDB
		common.RedisEnabled = oldEnabled
	})

	ctx := context.Background()
	setRedis := func(id int, periodResetTime int64, used int64) {
		_, err := common.RDB.IncrBy(ctx, getChannelPeriodKey(id, periodResetTime), used).Result()
		require.NoError(t, err)
	}
	setPending := func(type_, id, value int) {
		batchUpdateLocks[type_].Lock()
		batchUpdateStores[type_][id] = value
		batchUpdateLocks[type_].Unlock()
	}

	const redisOnlyID = 92001        // 只验证 Redis 优先级，不设 pending
	const redisPlusPendingID = 92002 // 验证采用 Redis 时 pending 不被重复叠加

	t.Cleanup(func() {
		batchUpdateLocks[BatchUpdateTypeChannelPeriodUsedQuota].Lock()
		delete(batchUpdateStores[BatchUpdateTypeChannelPeriodUsedQuota], redisPlusPendingID)
		batchUpdateLocks[BatchUpdateTypeChannelPeriodUsedQuota].Unlock()
	})

	tests := []struct {
		name        string
		channel     *Channel
		redisUsed   int64 // 0 = 不写 Redis key（模拟未启用 / 空 key）
		periodDelta int   // 未落库的周期增量
		want        bool
	}{
		{
			// Redis 实时计数已到限，内存快照还差一截：判定超限（堵住 60s 同步窗口）
			name: "redis at limit wins over stale memory",
			channel: &Channel{
				Id:               redisOnlyID,
				PeriodUsedQuota:  40,
				PeriodQuotaLimit: 50,
				PeriodResetTime:  111,
			},
			redisUsed: 50,
			want:      true,
		},
		{
			// 一致态：Redis 55 ≥ 内存 40 + pending 10 = 50，应采用 Redis（55）而非
			// 叠加 pending（55+10=65）。limit 60 下 55 未超限，证明不重复叠加。
			name: "redis adopted in consistent state without double counting",
			channel: &Channel{
				Id:               redisPlusPendingID,
				PeriodUsedQuota:  40,
				PeriodQuotaLimit: 60,
				PeriodResetTime:  222,
			},
			redisUsed:   55,
			periodDelta: 10,
			want:        false,
		},
		{
			// 不一致态：Redis 计数低于内存快照（如某次 INCR 写失败后失步，
			// 或重启丢 key 后被一笔小额扣费重建）。采用 Redis 会漏拦已超限渠道，
			// 必须回退内存判定（fail-closed）。
			name: "stale low redis falls back to memory",
			channel: &Channel{
				Id:               92006,
				PeriodUsedQuota:  50,
				PeriodQuotaLimit: 50,
				PeriodResetTime:  666,
			},
			redisUsed: 45,
			want:      true,
		},
		{
			// key 不存在（如 Redis 重启丢 key）：回退内存快照，不 fail-open 误放行
			name: "missing key falls back to stale memory",
			channel: &Channel{
				Id:               92003,
				PeriodUsedQuota:  60,
				PeriodQuotaLimit: 50,
				PeriodResetTime:  333,
			},
			want: true,
		},
		{
			// key 不存在且内存快照未用：未超限
			name: "missing key with zero memory is not exceeded",
			channel: &Channel{
				Id:               92004,
				PeriodUsedQuota:  0,
				PeriodQuotaLimit: 50,
				PeriodResetTime:  444,
			},
			want: false,
		},
		{
			// 周期限额为 0（不限）：即使 Redis 有计数也不判定超限
			name: "no period limit ignores redis",
			channel: &Channel{
				Id:               92005,
				PeriodUsedQuota:  0,
				PeriodQuotaLimit: 0,
				PeriodResetTime:  555,
			},
			redisUsed: 500,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.redisUsed > 0 {
				setRedis(tt.channel.Id, tt.channel.PeriodResetTime, tt.redisUsed)
			}
			if tt.periodDelta > 0 {
				setPending(BatchUpdateTypeChannelPeriodUsedQuota, tt.channel.Id, tt.periodDelta)
			}

			got := ChannelIsQuotaExceeded(tt.channel)
			assert.Equal(t, tt.want, got)
		})
	}
}
