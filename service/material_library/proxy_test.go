package material_library

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setConfig 经由配置系统写入素材库配置，模拟管理员 PUT /api/option 的加载路径。
func setConfig(t *testing.T, raw string) {
	t.Helper()
	registered := config.GlobalConfig.Get("material_library")
	require.NotNil(t, registered)
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{"config": raw}))
}

func TestDirector_mapsActionToArkAndInjectsCredential(t *testing.T) {
	// base_url 显式置空，避免与其他测试的配置串扰（配置系统 JSON 合并不重置缺省字段）。
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":""}`)

	req := httptest.NewRequest(http.MethodPost, "/api/material/ListAssetGroups", nil)
	director(req)

	assert.Equal(t, "/ark", req.URL.Path)
	assert.Equal(t, "https://ai-tokenhub.ai", req.URL.Scheme+"://"+req.URL.Host)
	assert.Equal(t, "ai-tokenhub.ai", req.Host)
	assert.Equal(t, "Action=ListAssetGroups&Version=2024-01-01", req.URL.RawQuery)
	assert.Equal(t, "Bearer pak_x:psk_y", req.Header.Get("Authorization"))
}

func TestDirector_usesConfiguredBaseURLAndKeepsClientQuery(t *testing.T) {
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"https://upstream.example.com"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/material/CreateAssetGroup?trace=abc", nil)
	director(req)

	assert.Equal(t, "https://upstream.example.com", req.URL.Scheme+"://"+req.URL.Host)
	assert.Equal(t, "/ark", req.URL.Path)
	// Action/Version 固定注入，客户端 query 保留
	assert.Contains(t, req.URL.RawQuery, "Action=CreateAssetGroup")
	assert.Contains(t, req.URL.RawQuery, "Version=2024-01-01")
	assert.Contains(t, req.URL.RawQuery, "trace=abc")
}

// TestProxy_passesThroughRequestAndResponse 覆盖完整代理链路：Action 映射、凭证注入、
// 请求体与上游响应（状态码/头/体）原样透传。
func TestProxy_passesThroughRequestAndResponse(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"group-abc"}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	body := `{"Name":"test","GroupType":"AIGC"}`
	req := httptest.NewRequest(http.MethodPost, "/api/material/CreateAssetGroup", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	Proxy().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/ark", gotPath)
	assert.Equal(t, "Action=CreateAssetGroup&Version=2024-01-01", gotQuery)
	assert.Equal(t, "Bearer pak_x:psk_y", gotAuth)
	assert.Equal(t, body, gotBody)
	assert.Equal(t, `{"ResponseMetadata":{},"Result":{"Id":"group-abc"}}`, rec.Body.String())
}

// TestProxy_forwardsUpstreamErrorVerbatim 验证上游错误信封（状态码 + 体）原样透传。
func TestProxy_forwardsUpstreamErrorVerbatim(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"feature not enabled"}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/material/ListAssetGroups", strings.NewReader(`{}`))
	Proxy().ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"feature not enabled"}}`, rec.Body.String())
}
