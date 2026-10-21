package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDeepSeekToolRequest(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		body    string
		wantErr string
	}{
		{name: "search minimal", tool: DeepSeekToolSearch, body: `{"query":"DeepSeek"}`},
		{name: "search unknown fields pass through", tool: DeepSeekToolSearch, body: `{"query":"q","count":5,"need_content":true,"localization":"cn"}`},
		{name: "search query required", tool: DeepSeekToolSearch, body: `{"count":5}`, wantErr: "query is required"},
		{name: "search query empty", tool: DeepSeekToolSearch, body: `{"query":""}`, wantErr: "query is required"},
		{name: "search query too long", tool: DeepSeekToolSearch, body: `{"query":"` + strings.Repeat("鲸", 129) + `"}`, wantErr: "at most 128"},
		{name: "search count zero", tool: DeepSeekToolSearch, body: `{"query":"q","count":0}`, wantErr: "between 1 and 50"},
		{name: "search count too large", tool: DeepSeekToolSearch, body: `{"query":"q","count":51}`, wantErr: "between 1 and 50"},
		{name: "search count bounds", tool: DeepSeekToolSearch, body: `{"query":"q","count":50}`},
		{name: "fetch https", tool: DeepSeekToolFetch, body: `{"url":"https://example.com/article"}`},
		{name: "fetch http", tool: DeepSeekToolFetch, body: `{"url":"http://example.com/article"}`},
		{name: "fetch url required", tool: DeepSeekToolFetch, body: `{}`, wantErr: "url is required"},
		{name: "fetch unsupported scheme", tool: DeepSeekToolFetch, body: `{"url":"ftp://example.com/a"}`, wantErr: "http or https"},
		{name: "fetch relative url", tool: DeepSeekToolFetch, body: `{"url":"/article"}`, wantErr: "valid absolute URL"},
		{name: "fetch credentials", tool: DeepSeekToolFetch, body: `{"url":"https://user:pass@example.com/a"}`, wantErr: "must not contain credentials"},
		{name: "invalid json", tool: DeepSeekToolSearch, body: `{"query":`, wantErr: "invalid JSON body"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeepSeekToolRequest(tt.tool, []byte(tt.body))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestIsDeepSeekToolBillable(t *testing.T) {
	tests := []struct {
		name string
		tool string
		body string
		want bool
	}{
		{name: "search with results", tool: DeepSeekToolSearch, body: `{"results":[{"title":"a"}]}`, want: true},
		{name: "search empty results", tool: DeepSeekToolSearch, body: `{"results":[]}`},
		{name: "search null results", tool: DeepSeekToolSearch, body: `{"results":null}`},
		{name: "search missing results", tool: DeepSeekToolSearch, body: `{}`},
		{name: "fetch with content", tool: DeepSeekToolFetch, body: `{"title":"t","content":"# body"}`, want: true},
		{name: "fetch empty content", tool: DeepSeekToolFetch, body: `{"title":"t","content":""}`},
		{name: "fetch null content", tool: DeepSeekToolFetch, body: `{"content":null}`},
		{name: "fetch missing content", tool: DeepSeekToolFetch, body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isDeepSeekToolBillable(tt.tool, []byte(tt.body)))
		})
	}
}

func TestDeepSeekToolQuota(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	// 5 美元 / 千次，500000 quota = 1 美元：单次 2500 quota，再乘分组倍率。
	require.Equal(t, 2500, deepSeekToolQuota(ctx, 5, 1, &relaycommon.RelayInfo{}))
	require.Equal(t, 1250, deepSeekToolQuota(ctx, 5, 0.5, &relaycommon.RelayInfo{}))
	require.Equal(t, 0, deepSeekToolQuota(ctx, 5, 0, &relaycommon.RelayInfo{}))
}

func TestDeepSeekToolQuotaRejectsNegativePricing(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	// 负价格/负倍率会在无 BillingSession 的结算回退路径里变成给用户加额度。
	require.Equal(t, 0, deepSeekToolQuota(ctx, -5, 1, &relaycommon.RelayInfo{}))
	require.Equal(t, 0, deepSeekToolQuota(ctx, 5, -1, &relaycommon.RelayInfo{}))
	require.Equal(t, 0, deepSeekToolQuota(ctx, -5, -1, &relaycommon.RelayInfo{}))
}

func TestDeepSeekToolPrices(t *testing.T) {
	require.Equal(t, 5.0, operation_setting.GetToolPrice("deepseek_search"))
	require.Equal(t, 5.0, operation_setting.GetToolPrice("deepseek_fetch"))
	require.Equal(t, 5.0, operation_setting.GetToolPriceForModel("web_search", "deepseek-v4-pro"))
	require.Equal(t, 5.0, operation_setting.GetToolPriceForModel("web_search", "deepseek-flash"))
	require.Equal(t, 10.0, operation_setting.GetToolPriceForModel("web_search", "claude-3-7-sonnet"))
}

func TestResponsesWebSearchBillsPerCallAtDeepSeekPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "deepseek-v4-pro",
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
				dto.BuildInToolWebSearch: {CallCount: 3},
			},
		},
	}
	summary := &textQuotaSummary{ModelName: "deepseek-v4-pro", GroupRatio: 1}

	surcharge := calculateTextToolCallSurcharge(ctx, relayInfo, summary)
	// 3 次 × 5 美元/千次 × 500000 = 7500
	require.Equal(t, "7500", surcharge.String())
	require.Equal(t, 3, summary.WebSearchCallCount)
}

func TestClaudeWebSearchBillsModelAwarePrice(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		model string
		want  string
	}{
		{model: "deepseek-v4-pro", want: "5000"},    // 2 次 × 5/1000 × 500000
		{model: "claude-3-7-sonnet", want: "10000"}, // 2 次 × 10/1000 × 500000
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Set("claude_web_search_requests", 2)

			relayInfo := &relaycommon.RelayInfo{OriginModelName: tt.model}
			summary := &textQuotaSummary{ModelName: tt.model, GroupRatio: 1}

			surcharge := calculateTextToolCallSurcharge(ctx, relayInfo, summary)
			require.Equal(t, tt.want, surcharge.String())
			require.Equal(t, 2, summary.ClaudeWebSearchCallCount)
		})
	}
}

type deepSeekToolTestEnv struct {
	ctx      *gin.Context
	recorder *httptest.ResponseRecorder
	user     *model.User
	token    *model.Token
	channel  *model.Channel
}

// setupDeepSeekToolTest seeds a user/token/channel whose upstream points at an
// httptest server, so the proxy runs through the real pre-consume/settle/refund
// path instead of a mock.
func setupDeepSeekToolTest(t *testing.T, body string, upstream http.HandlerFunc, userQuota int) *deepSeekToolTestEnv {
	t.Helper()
	common.MemoryCacheEnabled = false
	InitHttpClient()

	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)

	require.NoError(t, model.DB.Where("type = ?", constant.ChannelTypeDeepSeek).Delete(&model.Channel{}).Error)

	suffix := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	user := &model.User{
		Username:    "tooltest_" + suffix,
		Password:    "12345678",
		DisplayName: "tool test",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       userQuota,
		AffCode:     fmt.Sprintf("aff%d", time.Now().UnixNano()),
	}
	require.NoError(t, model.DB.Create(user).Error)

	token := &model.Token{
		UserId:         user.Id,
		Key:            fmt.Sprintf("tooltest%d", user.Id),
		Name:           "tool test",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}
	require.NoError(t, model.DB.Create(token).Error)

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeDeepSeek,
		Key:     "upstream-key",
		Status:  common.ChannelStatusEnabled,
		Name:    "tool-test-" + suffix,
		BaseURL: &baseURL,
		Models:  "deepseek-chat",
		Group:   "default",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	t.Cleanup(func() {
		// Unscoped: User/Token use soft deletes, and a soft-deleted row still
		// occupies the unique username/key index across repeated test runs.
		model.DB.Unscoped().Where("id = ?", user.Id).Delete(&model.User{})
		model.DB.Unscoped().Where("id = ?", token.Id).Delete(&model.Token{})
		model.DB.Unscoped().Where("id = ?", channel.Id).Delete(&model.Channel{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.Log{})
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/tools/search", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUserId, user.Id)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(ctx, constant.ContextKeyTokenKey, token.Key)
	common.SetContextKey(ctx, constant.ContextKeyTokenUnlimited, true)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})

	return &deepSeekToolTestEnv{ctx: ctx, recorder: recorder, user: user, token: token, channel: channel}
}

func setTokenModelLimits(t *testing.T, env *deepSeekToolTestEnv, enabled bool, limits string) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", env.token.Id).Updates(map[string]any{
		"model_limits_enabled": enabled,
		"model_limits":         limits,
	}).Error)

	limitMap := make(map[string]bool)
	for _, limit := range strings.Split(limits, ",") {
		if limit != "" {
			limitMap[limit] = true
		}
	}
	common.SetContextKey(env.ctx, constant.ContextKeyTokenModelLimitEnabled, enabled)
	common.SetContextKey(env.ctx, constant.ContextKeyTokenModelLimit, limitMap)
}

func (env *deepSeekToolTestEnv) userQuota(t *testing.T) int {
	t.Helper()
	quota, err := model.GetUserQuota(env.user.Id, true)
	require.NoError(t, err)
	return quota
}

func TestDeepSeekToolProxyBillsSuccessfulSearch(t *testing.T) {
	env := setupDeepSeekToolTest(t, `{"query":"DeepSeek","count":3}`, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/tools/search", r.URL.Path)
		assert.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"ok"}]}`))
	}, 100000)

	apiErr := DeepSeekToolProxy(env.ctx, DeepSeekToolSearch)
	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, env.recorder.Code)
	require.Contains(t, env.recorder.Body.String(), `"results"`)

	require.Equal(t, 97500, env.userQuota(t), "一次成功搜索应扣 2500 quota")

	var log model.Log
	require.NoError(t, model.DB.Where("user_id = ?", env.user.Id).First(&log).Error)
	require.Equal(t, "deepseek-search", log.ModelName)
	require.Equal(t, 2500, log.Quota)
	require.Contains(t, log.Other, `"billable":true`)
}

func TestDeepSeekToolProxyRefundsEmptySearchResult(t *testing.T) {
	env := setupDeepSeekToolTest(t, `{"query":"nothing"}`, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}, 100000)

	apiErr := DeepSeekToolProxy(env.ctx, DeepSeekToolSearch)
	require.Nil(t, apiErr)
	require.Equal(t, http.StatusOK, env.recorder.Code)
	require.Equal(t, 100000, env.userQuota(t), "无结果不得扣费")

	var log model.Log
	require.NoError(t, model.DB.Where("user_id = ?", env.user.Id).First(&log).Error)
	require.Equal(t, 0, log.Quota)
	require.Contains(t, log.Other, `"billable":false`)
}

func TestDeepSeekToolProxyRefundsUpstreamError(t *testing.T) {
	env := setupDeepSeekToolTest(t, `{"query":"boom"}`, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream failed"}`))
	}, 100000)

	apiErr := DeepSeekToolProxy(env.ctx, DeepSeekToolSearch)
	require.Nil(t, apiErr)
	require.Equal(t, http.StatusInternalServerError, env.recorder.Code)
	require.Contains(t, env.recorder.Body.String(), "upstream failed", "上游错误应原样透传")
	require.Eventually(t, func() bool {
		quota, err := model.GetUserQuota(env.user.Id, true)
		return err == nil && quota == 100000
	}, 2*time.Second, 10*time.Millisecond, "上游失败不得扣费（退款异步执行）")

	var count int64
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ?", env.user.Id).Count(&count).Error)
	require.Zero(t, count, "失败请求不写消费流水")
}

func TestDeepSeekToolProxyFailsWhenNoChannel(t *testing.T) {
	env := setupDeepSeekToolTest(t, `{"query":"DeepSeek"}`, func(w http.ResponseWriter, r *http.Request) {}, 100000)
	require.NoError(t, model.DB.Where("id = ?", env.channel.Id).Delete(&model.Channel{}).Error)

	apiErr := DeepSeekToolProxy(env.ctx, DeepSeekToolSearch)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.Equal(t, 100000, env.userQuota(t))
}

func TestDeepSeekToolProxyEnforcesTokenModelLimits(t *testing.T) {
	tests := []struct {
		name       string
		limits     string
		wantStatus int
	}{
		{name: "non deepseek model denied", limits: "gpt-4o", wantStatus: http.StatusForbidden},
		{name: "empty limits denied", limits: "", wantStatus: http.StatusForbidden},
		{name: "deepseek model allowed", limits: "deepseek-chat", wantStatus: http.StatusOK},
		{name: "deepseek search pseudo model allowed", limits: DeepSeekSearchModelName, wantStatus: http.StatusOK},
		{name: "deepseek fetch pseudo model allowed", limits: DeepSeekFetchModelName, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupDeepSeekToolTest(t, `{"query":"DeepSeek"}`, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"results":[{"title":"ok"}]}`))
			}, 100000)
			setTokenModelLimits(t, env, true, tt.limits)

			apiErr := DeepSeekToolProxy(env.ctx, DeepSeekToolSearch)
			if tt.wantStatus == http.StatusOK {
				require.Nil(t, apiErr)
				require.Equal(t, http.StatusOK, env.recorder.Code)
				require.Equal(t, 97500, env.userQuota(t))
				return
			}

			require.NotNil(t, apiErr)
			require.Equal(t, tt.wantStatus, apiErr.StatusCode)
			require.Equal(t, 100000, env.userQuota(t), "令牌无模型权限时不得扣费")

			var count int64
			require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ?", env.user.Id).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
