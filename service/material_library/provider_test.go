package material_library

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
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

// TestTranslateTokenhubAssetStatuses 验证 ListAssets 的状态过滤翻译：对客按
// 火山文档传 Active/Processing/Failed，上游过滤只认小写内部枚举。
func TestTranslateTokenhubAssetStatuses(t *testing.T) {
	statusesOf := func(t *testing.T, body []byte) []string {
		t.Helper()
		var parsed struct {
			Filter struct {
				Statuses []string `json:"Statuses"`
			} `json:"Filter"`
		}
		require.NoError(t, common.Unmarshal(body, &parsed))
		return parsed.Filter.Statuses
	}

	t.Run("文档枚举翻译成上游枚举", func(t *testing.T) {
		body := []byte(`{"Filter":{"GroupIds":["g"],"Statuses":["Active","Processing","Failed"]},"PageNumber":1,"PageSize":10}`)
		assert.Equal(t, []string{"success", "verifying", "failed"}, statusesOf(t, translateTokenhubAssetStatuses(body)))
	})

	t.Run("大小写不敏感", func(t *testing.T) {
		body := []byte(`{"Filter":{"GroupIds":["g"],"Statuses":["ACTIVE","processing"]},"PageNumber":1,"PageSize":10}`)
		assert.Equal(t, []string{"success", "verifying"}, statusesOf(t, translateTokenhubAssetStatuses(body)))
	})

	t.Run("未知值原样保留", func(t *testing.T) {
		body := []byte(`{"Filter":{"GroupIds":["g"],"Statuses":["Active","Custom"]},"PageNumber":1,"PageSize":10}`)
		assert.Equal(t, []string{"success", "Custom"}, statusesOf(t, translateTokenhubAssetStatuses(body)))
	})

	t.Run("已是上游枚举时不改请求体", func(t *testing.T) {
		body := []byte(`{"Filter":{"GroupIds":["g"],"Statuses":["success"]},"PageNumber":1,"PageSize":10}`)
		assert.Equal(t, body, translateTokenhubAssetStatuses(body))
	})

	t.Run("没有 Statuses 或形状不符时原样返回", func(t *testing.T) {
		bodies := [][]byte{
			[]byte(`{"Filter":{"GroupIds":["g"]},"PageNumber":1,"PageSize":10}`),
			[]byte(`{"GroupIds":["g"]}`),
			[]byte(`{"Filter":{"GroupIds":["g"],"Statuses":"Active"}}`),
			[]byte(`not-json`),
		}
		for _, body := range bodies {
			assert.Equal(t, body, translateTokenhubAssetStatuses(body))
		}
	})
}

// TestTokenhubProvider_translatesListAssetsStatuses 验证翻译只发生在 ListAssets
// 出站请求上，其它 Action 的请求体保持原样。
func TestTokenhubProvider_translatesListAssetsStatuses(t *testing.T) {
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))
	defer setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y","base_url":""}`)

	body := []byte(`{"Filter":{"GroupIds":["g"],"Statuses":["Active"]},"PageNumber":1,"PageSize":10}`)
	_, err := tokenhubProvider{}.DoAction(context.Background(), "ListAssets", nil, body)
	require.NoError(t, err)
	var sent struct {
		Filter struct {
			Statuses []string `json:"Statuses"`
		} `json:"Filter"`
	}
	require.NoError(t, common.Unmarshal(gotBody, &sent))
	assert.Equal(t, []string{"success"}, sent.Filter.Statuses)

	_, err = tokenhubProvider{}.DoAction(context.Background(), "GetAsset", nil, body)
	require.NoError(t, err)
	assert.Equal(t, body, gotBody)
}
