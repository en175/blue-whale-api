package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 同步链路（文本/音频/Claude/WSS）与 MJ 的日志必须像任务链路一样标出倍率来源。
// 缺了 model_group_ratio 时，命中分组模型折扣的请求在前端只能回落显示成
// "Group Ratio" —— 金额是对的，标签是错的，而标签正是这个字段存在的意义。
func TestOtherInfoReportsModelGroupRatioSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	cases := []struct {
		name      string
		info      types.GroupRatioInfo
		wantRatio float64
		wantField bool
	}{
		{
			name:      "命中分组模型折扣时写出该层倍率",
			info:      types.NewGroupRatioInfo(0.4, types.GroupRatioSourceGroupModel),
			wantRatio: 0.4,
			wantField: true,
		},
		{
			// 0 是合法折扣（该模型在该组下免费）。若判定写成 >0，这笔免费请求
			// 在日志里会伪装成「没有模型折扣」。
			name:      "折扣配 0 仍需写出，不能当成未配置",
			info:      types.NewGroupRatioInfo(0, types.GroupRatioSourceGroupModel),
			wantRatio: 0,
			wantField: true,
		},
		{
			name: "命中用户组特价时不写模型层字段",
			info: types.NewGroupRatioInfo(0.5, types.GroupRatioSourceUserGroup),
		},
		{
			name: "只有分组基础倍率时不写模型层字段",
			info: types.NewGroupRatioInfo(1, types.GroupRatioSourceGroup),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			relayInfo := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{},
				PriceData:   types.PriceData{GroupRatioInfo: tc.info},
			}

			for _, other := range []map[string]interface{}{
				GenerateTextOtherInfo(ctx, relayInfo, 11.5, tc.info.GroupRatio, 1, 0, 0, 11.5, tc.info.GroupSpecialRatio),
				GenerateMjOtherInfo(relayInfo, relayInfo.PriceData),
			} {
				got, ok := other["model_group_ratio"]
				require.Equal(t, tc.wantField, ok, "字段出现与否不对")
				if tc.wantField {
					require.Equal(t, tc.wantRatio, got)
				}
				// 生效倍率本身照旧记在 group_ratio 上，本次改动不改变它的语义
				require.Equal(t, tc.info.GroupRatio, other["group_ratio"])
			}
		})
	}
}
