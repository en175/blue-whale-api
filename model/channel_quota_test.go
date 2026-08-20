package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
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
