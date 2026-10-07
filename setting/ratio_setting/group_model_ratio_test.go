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
