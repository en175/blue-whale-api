package material_library

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVolcengineProvider_buildsSignedRequest 验证火山 provider 的地址映射与 SigV4 签名：
// URL 走管控面 endpoint，Action/Version 注入 query，签名头（X-Date/X-Content-Sha256/
// Authorization）齐备且 Credential 作用域为 ak/{region}/ark/request。
func TestVolcengineProvider_buildsSignedRequest(t *testing.T) {
	setConfig(t, `{"enabled":true,"provider":"volcengine","volcengine_access_key":"AK_test","volcengine_secret_key":"SK_test","volcengine_region":"cn-beijing","volcengine_base_url":"https://ark-cn-beijing.volcengineapi.com"}`)
	// 配置系统 JSON 合并不重置缺省字段，显式恢复 tokenhub，避免 provider 状态串扰后续测试。
	defer setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y","base_url":""}`)

	req, err := volcengineProvider{}.newRequest(context.Background(), "ListAssets", nil, []byte(`{}`))
	require.NoError(t, err)

	assert.Equal(t, "https://ark-cn-beijing.volcengineapi.com", req.URL.Scheme+"://"+req.URL.Host)
	assert.Contains(t, req.URL.RawQuery, "Action=ListAssets")
	assert.Contains(t, req.URL.RawQuery, "Version=2024-01-01")

	// 管控面 SigV4 签名头齐备（非 Bearer）
	assert.Empty(t, req.Header.Get("Authorization-Bearer"))
	assert.NotEmpty(t, req.Header.Get("X-Date"))
	assert.NotEmpty(t, req.Header.Get("X-Content-Sha256"))
	auth := req.Header.Get("Authorization")
	assert.Contains(t, auth, "HMAC-SHA256")
	assert.Contains(t, auth, "Credential=AK_test/")
	assert.Contains(t, auth, "/cn-beijing/ark/request")
}

// TestMaterialGroups_providerIsolation 验证归属表按 provider 隔离：
// 同一用户可在不同上游持有同名/不同名组，查询与归属校验跨 provider 不串。
func TestMaterialGroups_providerIsolation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(1, "tokenhub", "g-tokenhub", "t", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(1, "volcengine", "g-volc", "v", "AIGC"))

	tokenGroups, err := model.GetMaterialGroups(1, "tokenhub", "")
	require.NoError(t, err)
	require.Len(t, tokenGroups, 1)
	assert.Equal(t, "g-tokenhub", tokenGroups[0].SeedanceAssetGroupId)
	assert.Equal(t, "tokenhub", tokenGroups[0].Provider)

	volcGroups, err := model.GetMaterialGroups(1, "volcengine", "")
	require.NoError(t, err)
	require.Len(t, volcGroups, 1)
	assert.Equal(t, "g-volc", volcGroups[0].SeedanceAssetGroupId)

	// 归属校验跨 provider 隔离：tokenhub 的组不会被 volcengine 校验命中
	owned, err := model.UserMaterialGroupExists(1, "volcengine", "g-tokenhub")
	require.NoError(t, err)
	assert.False(t, owned)
	owned, err = model.UserMaterialGroupExists(1, "volcengine", "g-volc")
	require.NoError(t, err)
	assert.True(t, owned)
}
