package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetUserLogsTypeUnion 保护 IKGCWR 的验收契约：type 多值取并集，total 为命中类型条数之和；
// 空列表不过滤（与历史 type=0 一致），且仍只返回当前用户的流水。
func TestGetUserLogsTypeUnion(t *testing.T) {
	truncateTables(t)

	const (
		owner  = 9001
		others = 9002
	)
	seeds := []Log{
		{UserId: owner, Type: LogTypeTopup, CreatedAt: 100},
		{UserId: owner, Type: LogTypeConsume, CreatedAt: 200},
		{UserId: owner, Type: LogTypeRefund, CreatedAt: 300},
		{UserId: others, Type: LogTypeConsume, CreatedAt: 400},
	}
	for i := range seeds {
		require.NoError(t, LOG_DB.Create(&seeds[i]).Error)
	}

	tests := []struct {
		name     string
		logTypes []int
		expected int64
	}{
		{name: "空列表不过滤", logTypes: nil, expected: 3},
		{name: "单值与历史行为一致", logTypes: []int{LogTypeConsume}, expected: 1},
		{name: "多值取并集", logTypes: []int{LogTypeConsume, LogTypeRefund}, expected: 2},
		{name: "重复值不重复计数", logTypes: []int{LogTypeConsume, LogTypeConsume}, expected: 1},
		{name: "无命中类型返回空", logTypes: []int{LogTypeLogin}, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs, total, err := GetUserLogs(owner, tt.logTypes, 0, 0, "", "", 0, 20, "", "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, total)
			require.Len(t, logs, int(tt.expected))
			for _, entry := range logs {
				assert.Equal(t, owner, entry.UserId, "不得越权返回其他用户的流水")
			}
		})
	}
}

// TestGetUserLogsTypeUnionPaginationIsSingleCursor 保护本单的立项理由：消耗与退款合进一条
// 带 IN 的查询后，翻页由同一个游标推进，两页之间不重叠、不遗漏。
// 断言用 RequestId 而非 Id —— 返回值里的 Id 被 formatUserLogs 覆写成展示序号，不是主键。
func TestGetUserLogsTypeUnionPaginationIsSingleCursor(t *testing.T) {
	truncateTables(t)

	const owner = 9004
	seeds := []Log{
		{UserId: owner, Type: LogTypeConsume, CreatedAt: 100, RequestId: "consume-1"},
		{UserId: owner, Type: LogTypeRefund, CreatedAt: 200, RequestId: "refund-1"},
		{UserId: owner, Type: LogTypeConsume, CreatedAt: 300, RequestId: "consume-2"},
		{UserId: owner, Type: LogTypeRefund, CreatedAt: 400, RequestId: "refund-2"},
		{UserId: owner, Type: LogTypeTopup, CreatedAt: 500, RequestId: "topup-1"},
	}
	for i := range seeds {
		require.NoError(t, LOG_DB.Create(&seeds[i]).Error)
	}

	seen := make([]string, 0, 4)
	for page := 0; page < 2; page++ {
		logs, total, err := GetUserLogs(owner, []int{LogTypeConsume, LogTypeRefund}, 0, 0, "", "", page*2, 2, "", "", "")
		require.NoError(t, err)
		assert.Equal(t, int64(4), total, "total 应为两类条数之和，与页码无关")
		require.Len(t, logs, 2)
		for _, entry := range logs {
			seen = append(seen, entry.RequestId)
		}
	}

	assert.ElementsMatch(t, []string{"consume-1", "refund-1", "consume-2", "refund-2"}, seen)
}
