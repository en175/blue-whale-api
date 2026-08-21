package material_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_IsReady(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"凭证齐全且启用", Config{Enabled: true, AK: "pak_a", SK: "psk_b"}, true},
		{"未启用", Config{AK: "pak_a", SK: "psk_b"}, false},
		{"缺 AK", Config{Enabled: true, SK: "psk_b"}, false},
		{"缺 SK", Config{Enabled: true, AK: "pak_a"}, false},
		{"volcengine 凭证齐全且启用", Config{Enabled: true, Provider: ProviderVolcengine, VolcengineAccessKey: "ak", VolcengineSecretKey: "sk"}, true},
		{"volcengine 缺 AccessKey", Config{Enabled: true, Provider: ProviderVolcengine, VolcengineSecretKey: "sk"}, false},
		{"volcengine 缺 SecretKey", Config{Enabled: true, Provider: ProviderVolcengine, VolcengineAccessKey: "ak"}, false},
		{"volcengine 未启用", Config{Provider: ProviderVolcengine, VolcengineAccessKey: "ak", VolcengineSecretKey: "sk"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.IsReady())
		})
	}
}

func TestGetConfig_defaultsBaseURL(t *testing.T) {
	materialLibrary.Config = Config{Enabled: true, AK: "pak_a", SK: "psk_b"}
	assert.Equal(t, DefaultBaseURL, GetConfig().BaseURL)

	materialLibrary.Config = Config{Enabled: true, BaseURL: "https://custom.example.com"}
	assert.Equal(t, "https://custom.example.com", GetConfig().BaseURL)
}

func TestGetConfig_defaultsProviderAndVolcengineDefaults(t *testing.T) {
	materialLibrary.Config = Config{Enabled: true, AK: "pak_a", SK: "psk_b"}
	cfg := GetConfig()
	assert.Equal(t, ProviderTokenhub, cfg.Provider)
	assert.Equal(t, DefaultVolcengineBaseURL, cfg.VolcengineBaseURL)
	assert.Equal(t, DefaultVolcengineRegion, cfg.VolcengineRegion)
}

// TestMaterialLibraryConfig_dbKeyIsSingleJSON 验证配置经全局配置系统收敛为单一
// JSON 键 material_library.config（管理员 PUT /api/option 的存取路径）。
func TestMaterialLibraryConfig_dbKeyIsSingleJSON(t *testing.T) {
	registered := config.GlobalConfig.Get("material_library")
	require.NotNil(t, registered)

	raw := `{"enabled":true,"ak":"pak_a","sk":"psk_b","base_url":"https://example.com"}`
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{"config": raw}))

	cfg := GetConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "pak_a", cfg.AK)
	assert.Equal(t, "psk_b", cfg.SK)
	assert.Equal(t, "https://example.com", cfg.BaseURL)

	exported, err := config.ConfigToMap(registered)
	require.NoError(t, err)
	require.Contains(t, exported, "config")
	require.Len(t, exported, 1)
	assert.JSONEq(t, raw, exported["config"])
}
