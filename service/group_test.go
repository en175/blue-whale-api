package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

// TestResolveGroupRatioPriority 锁定三层分组倍率的替换优先级：
// 分组模型折扣 → 用户组特价 → 分组基础倍率。
// 三层任一命中即整体替换，绝不叠乘；预扣与结算都必须拿到同一个值。
func TestResolveGroupRatioPriority(t *testing.T) {
	originGroupRatio := ratio_setting.GroupRatio2JSONString()
	originGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	originGroupModelRatio := ratio_setting.GroupModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupModelRatioByJSONString(originGroupModelRatio))
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.8}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"default":0.5}}`))
	require.NoError(t, ratio_setting.UpdateGroupModelRatioByJSONString(`{"default":{"seedance-2.0":0.3}}`))

	tests := []struct {
		name       string
		userGroup  string
		usingGroup string
		modelName  string
		wantRatio  float64
		wantSource types.GroupRatioSource
	}{
		{
			name:       "分组模型折扣压过用户组特价与分组倍率",
			userGroup:  "vip",
			usingGroup: "default",
			modelName:  "seedance-2.0",
			wantRatio:  0.3,
			wantSource: types.GroupRatioSourceGroupModel,
		},
		{
			name:       "同一分组的其他模型回落到用户组特价",
			userGroup:  "vip",
			usingGroup: "default",
			modelName:  "seedance-2.0-fast",
			wantRatio:  0.5,
			wantSource: types.GroupRatioSourceUserGroup,
		},
		{
			name:       "模型名为空时跳过分组模型折扣",
			userGroup:  "vip",
			usingGroup: "default",
			modelName:  "",
			wantRatio:  0.5,
			wantSource: types.GroupRatioSourceUserGroup,
		},
		{
			name:       "两层特价都没配时使用分组基础倍率",
			userGroup:  "default",
			usingGroup: "default",
			modelName:  "seedance-2.0-fast",
			wantRatio:  1,
			wantSource: types.GroupRatioSourceGroup,
		},
		{
			name:       "分组模型折扣按使用组索引，不看用户组",
			userGroup:  "vip",
			usingGroup: "vip",
			modelName:  "seedance-2.0",
			wantRatio:  0.8,
			wantSource: types.GroupRatioSourceGroup,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio, source := ResolveGroupRatio(tt.userGroup, tt.usingGroup, tt.modelName)
			require.Equal(t, tt.wantRatio, ratio)
			require.Equal(t, tt.wantSource, source)
		})
	}
}
