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
// 请求体与上游响应（状态码/头/体）原样透传；CRUD 成功响应保留信封（tokenhub 本身信封）。
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

// TestProxy_forwardsUpstreamErrorInTokenhubFormat 验证上游错误归一为 tokenhub 格式
// （状态码 + {"ResponseMetadata":{},"ResponseError":{...}}），CRUD 渠道不影响错误格式。
func TestProxy_forwardsUpstreamErrorInTokenhubFormat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"feature not enabled"}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	rec := httptest.NewRecorder()
	// 用表外 Action 验证上游错误信封原样透传（表内 Action 会先做归属校验）
	req := httptest.NewRequest(http.MethodPost, "/api/material/SomeFutureAction", strings.NewReader(`{"Id":"asset-x"}`))
	Proxy().ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	// tokenhub 错误本身带 ResponseMetadata + ResponseError，归一化按 tokenhub 格式原样透传。
	assert.JSONEq(t, `{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"feature not enabled"}}`, rec.Body.String())
}

// TestProxy_volcengineCRUDKeepsEnvelope 验证火山渠道 CRUD 成功响应保留信封
// （与 tokenhub 一致的对外格式，符合已外发接口文档），并证明确实走了火山渠道
// provider（/ark 路径 + SigV4 签名 Authorization 头）。
func TestProxy_volcengineCRUDKeepsEnvelope(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"req-1"},"Result":{"Id":"group-volc"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"provider":"volcengine","volcengine_access_key":"ak","volcengine_secret_key":"sk","volcengine_base_url":%q}`, upstream.URL))
	defer setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y","base_url":""}`)

	req := httptest.NewRequest(http.MethodPost, "/api/material/CreateAssetGroup", strings.NewReader(`{"Name":"g","GroupType":"AIGC"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{"RequestId":"req-1"},"Result":{"Id":"group-volc"}}`, rec.Body.String())
	// 证明走了火山渠道 provider：基础路径为 /（火山管控面根，tokenhub 才是 /ark）+ SigV4 签名头
	assert.Equal(t, "/", gotPath)
	assert.NotEmpty(t, gotAuth)
}

// TestNormalizeUpstreamResponse 表驱动验证响应归一化（火山渠道兼容 tokenhub 实际格式）：
// CRUD 保留信封、会话类去信封、火山错误转 tokenhub 错误、tokenhub 直接/错误原样透传。
func TestNormalizeUpstreamResponse(t *testing.T) {
	cases := []struct {
		name   string
		action string
		in     string
		want   string
	}{
		{"tokenhub 直接原样", "GetVisualValidateResult", `{"GroupId":"g1"}`, `{"GroupId":"g1"}`},
		{"CRUD 成功保留信封", "CreateAssetGroup", `{"ResponseMetadata":{},"Result":{"Id":"g2"}}`, `{"ResponseMetadata":{},"Result":{"Id":"g2"}}`},
		{"会话类成功去信封", "GetVisualValidateResult", `{"ResponseMetadata":{},"Result":{"GroupId":"g3"}}`, `{"GroupId":"g3"}`},
		{"会话类大整数保精度", "GetVisualValidateResult", `{"ResponseMetadata":{},"Result":{"GroupId":"g","ExpireAt":9007199254740993}}`, `{"GroupId":"g","ExpireAt":9007199254740993}`},
		{"火山错误转 tokenhub 错误", "CreateAssetGroup", `{"ResponseMetadata":{"Error":{"Code":"NotFound","Message":"x"}}}`, `{"ResponseMetadata":{},"ResponseError":{"Code":"NotFound","Message":"x"}}`},
		{"tokenhub 错误原样", "CreateAssetGroup", `{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"f"}}`, `{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"f"}}`},
		{"裸 tokenhub 错误补空信封", "CreateAssetGroup", `{"ResponseError":{"Code":"InternalError","Message":"e"}}`, `{"ResponseMetadata":{},"ResponseError":{"Code":"InternalError","Message":"e"}}`},
		{"非 JSON 原样", "CreateAssetGroup", `not-json`, `not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeUpstreamResponse(tc.action, []byte(tc.in))
			if tc.name == "非 JSON 原样" {
				assert.Equal(t, tc.want, string(got))
			} else {
				assert.JSONEq(t, tc.want, string(got))
			}
		})
	}
}

// TestMirrorName 保护镜像表写入边界：原生入口不做名称过滤（D13），超长名与
// 尖括号会直接到达落库侧。列宽 varchar(255)，超长在 MySQL strict / PG 上插入
// 失败会让归属丢失、该组后续操作全部 404；尖括号则进管理端展示。
func TestMirrorName(t *testing.T) {
	cases := []struct {
		label string
		in    string
		want  string
	}{
		{"正常名不变", "我的素材组", "我的素材组"},
		{"255 边界不变", strings.Repeat("中", 255), strings.Repeat("中", 255)},
		{"超长截断到 255 rune", strings.Repeat("中", 300), strings.Repeat("中", 255)},
		{"移除尖括号", "a<b>c", "abc"},
		{"空名不变", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			assert.Equal(t, tc.want, mirrorName(context.Background(), tc.in))
		})
	}
}

// TestMaterialNameInvalid 表驱动验证名称校验规则：超长（>255 rune）与 HTML 危险字符 < >
// 判非法；正常中文/英文/数字名、255 边界、以及 SQL 注入负载（网关不拦、透传上游）放行。
// 超长拒绝防镜像表列宽溢出（tokenhub 对超长名回 500）；SQL 注入防御在上游后端，网关不代劳。
func TestMaterialNameInvalid(t *testing.T) {
	cases := []struct {
		label string
		in    string
		want  bool
	}{
		{"正常中文名", "我的素材组", false},
		{"正常英文数字", "AssetGroup-2024", false},
		{"255 边界放行", strings.Repeat("中", 255), false},
		{"256 超长拒绝", strings.Repeat("中", 256), true},
		{"HTML < 拒绝", "a<b", true},
		{"HTML > 拒绝", "a>b", true},
		{"SQL 负载放行", `Robert'); DROP TABLE user_material_groups;--`, false},
		{"单引号放行", "O'Brien", false},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := MaterialNameInvalid(tc.in)
			assert.Equal(t, tc.want, got, "MaterialNameInvalid(%q) = %v, want %v", tc.in, got, tc.want)
		})
	}
}
