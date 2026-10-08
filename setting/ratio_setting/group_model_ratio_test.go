package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetGroupModelRatio(t *testing.T) {
	origin := GroupModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupModelRatioByJSONString(origin))
	})

	require.NoError(t, UpdateGroupModelRatioByJSONString(
		`{"vip":{"seedance-2.0":0.8,"seedance-2.0-fast":0.6,"seedance-2.0-free":0}}`))

	tests := []struct {
		name       string
		usingGroup string
		modelName  string
		wantRatio  float64
		wantOk     bool
	}{
		{"命中折扣", "vip", "seedance-2.0", 0.8, true},
		{"同组另一模型", "vip", "seedance-2.0-fast", 0.6, true},
		{"折扣为 0 时算命中，不能与未配置混淆", "vip", "seedance-2.0-free", 0, true},
		{"分组未配置", "default", "seedance-2.0", -1, false},
		{"模型未配置", "vip", "gpt-4o", -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio, ok := GetGroupModelRatio(tt.usingGroup, tt.modelName)
			require.Equal(t, tt.wantOk, ok)
			require.Equal(t, tt.wantRatio, ratio)
		})
	}
}

func TestCheckGroupModelRatio(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{"空表合法", `{}`, false},
		{"0 合法，该模型在该组下免费", `{"vip":{"seedance-2.0":0}}`, false},
		{"负数被拒", `{"vip":{"seedance-2.0":-1}}`, true},
		{"非法 JSON 被拒", `not-json`, true},
		{"空串表示清空配置，不算错误", ``, false},
		{"纯空白同样表示清空配置", `   `, false},
		// encoding/json 把 null 解进非指针标量是 no-op 且不报错，键会以零值 0 落表。
		// 而 0 的含义是「该模型在该组免费」，放行即静默白送。前端的 Infinity 输入
		// （JSON.stringify(Infinity) -> null）也会走到这里，所以这层必须挡住。
		{"模型倍率为 null 被拒，不能静默变成 0", `{"vip":{"seedance-2.0":null}}`, true},
		{"分组表为 null 被拒", `{"vip":null}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckGroupModelRatio(tt.jsonStr)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// 空串是前端清空原始 JSON 框时发出的值。只让校验放行还不够——写入侧也必须归一化，
// 否则 CheckGroupModelRatio 通过之后 UpdateGroupModelRatioByJSONString 仍会报
// unexpected end of JSON input，管理员依然改不掉配置。
func TestUpdateGroupModelRatioByJSONStringEmptyClears(t *testing.T) {
	origin := GroupModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupModelRatioByJSONString(origin))
	})

	require.NoError(t, UpdateGroupModelRatioByJSONString(`{"vip":{"seedance-2.0":0.4}}`))
	_, ok := GetGroupModelRatio("vip", "seedance-2.0")
	require.True(t, ok)

	require.NoError(t, UpdateGroupModelRatioByJSONString(""))

	_, ok = GetGroupModelRatio("vip", "seedance-2.0")
	require.False(t, ok, "空串应当清空折扣表")
	require.Equal(t, `{}`, GroupModelRatio2JSONString())
}
