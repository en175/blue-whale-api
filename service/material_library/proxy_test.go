package material_library

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestTokenhubProvider_mapsActionToArkAndInjectsCredential(t *testing.T) {
	// base_url 显式置空，避免与其他测试的配置串扰（配置系统 JSON 合并不重置缺省字段）。
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":""}`)

	req, err := tokenhubProvider{}.newRequest(context.Background(), "ListAssetGroups", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "/ark", req.URL.Path)
	assert.Equal(t, "https://ai-tokenhub.ai", req.URL.Scheme+"://"+req.URL.Host)
	assert.Equal(t, "Action=ListAssetGroups&Version=2024-01-01", req.URL.RawQuery)
	assert.Equal(t, "Bearer pak_x:psk_y", req.Header.Get("Authorization"))
}

func TestTokenhubProvider_usesConfiguredBaseURLAndKeepsClientQuery(t *testing.T) {
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"https://upstream.example.com"}`)

	q := url.Values{}
	q.Set("trace", "abc")
	req, err := tokenhubProvider{}.newRequest(context.Background(), "CreateAssetGroup", q, nil)
	require.NoError(t, err)

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
	assert.Equal(t, `{"Id":"group-abc"}`, rec.Body.String())
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
	// 用 GetAsset（素材单操作，尚未做归属校验的透传 Action）验证上游错误信封原样透传
	req := httptest.NewRequest(http.MethodPost, "/api/material/GetAsset", strings.NewReader(`{"Id":"asset-x"}`))
	Proxy().ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"ResponseError":{"Code":"Forbidden","Message":"feature not enabled"}}`, rec.Body.String())
}

// TestNormalizeUpstreamResponse 表驱动验证响应归一化（统一 tokenhub 直接格式）。
func TestNormalizeUpstreamResponse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tokenhub 直接原样", `{"GroupId":"g1"}`, `{"GroupId":"g1"}`},
		{"火山成功去信封", `{"ResponseMetadata":{},"Result":{"Id":"g2"}}`, `{"Id":"g2"}`},
		{"火山错误转 ResponseError", `{"ResponseMetadata":{"Error":{"Code":"NotFound","Message":"x"}}}`, `{"ResponseError":{"Code":"NotFound","Message":"x"}}`},
		{"空 ResponseMetadata + ResponseError 剥信封", `{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"f"}}`, `{"ResponseError":{"Code":"Forbidden","Message":"f"}}`},
		{"非 JSON 原样", `not-json`, `not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeUpstreamResponse([]byte(tc.in))
			if tc.name == "非 JSON 原样" {
				assert.Equal(t, tc.want, string(got))
			} else {
				assert.JSONEq(t, tc.want, string(got))
			}
		})
	}
}
