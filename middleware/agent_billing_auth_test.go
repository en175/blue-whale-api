package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// IKCGDX/IKCGE2: the gateway credential can bill any user in the system, so every early exit that
// stops a request before it reaches the billing subject is worth pinning down.

func TestAgentBillingCredentialRejectsUnconfiguredAndWrong(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "")
	assert.False(t, authorizedAgentBillingService("bwsvc_anything"),
		"unset credential must never authorize, least of all against an empty string")

	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")
	assert.False(t, authorizedAgentBillingService(""))
	assert.False(t, authorizedAgentBillingService("bwsvc_wrong"))
	assert.False(t, authorizedAgentBillingService("bwsvc_right_with_suffix"))
	assert.True(t, authorizedAgentBillingService("bwsvc_right"))
}

func agentBillingRequest(t *testing.T, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	for key, value := range headers {
		c.Request.Header.Set(key, value)
	}
	AgentBillingAuth(c, c.Request.Header.Get("Authorization"))
	return recorder
}

func TestAgentBillingAuthRejectsBadCredential(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")
	recorder := agentBillingRequest(t, "/v1/chat/completions", map[string]string{
		"Authorization":           "bwsvc_wrong",
		AgentBillingSubjectHeader: "37",
	})
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestAgentBillingAuthConfinedToChatCompletions(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")
	recorder := agentBillingRequest(t, "/v1/embeddings", map[string]string{
		"Authorization":           "bwsvc_right",
		AgentBillingSubjectHeader: "37",
	})
	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestAgentBillingAuthRequiresUsableSubject(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")
	for name, subject := range map[string]string{
		"absent":       "",
		"blank":        "   ",
		"zero":         "0",
		"negative":     "-1",
		"not a number": "someone",
	} {
		t.Run(name, func(t *testing.T) {
			headers := map[string]string{"Authorization": "bwsvc_right"}
			if subject != "" {
				headers[AgentBillingSubjectHeader] = subject
			}
			recorder := agentBillingRequest(t, "/v1/chat/completions", headers)
			// A missing subject must fail closed rather than fall back to any default account:
			// the whole point of the change is that nobody guesses who pays.
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

// 漏配和配错前缀都是「看起来在跑、其实全量 401」，两种都必须在启动时暴露。
// 前缀那条尤其阴：RelayAuth 按前缀分流，不带前缀的凭证根本走不到这个中间件，
// 会掉进 TokenAuth 以普通 401 死掉——从日志上完全看不出是配置问题。
func TestValidateAgentBillingConfig(t *testing.T) {
	t.Run("未配置只记一行日志，不拦启动", func(t *testing.T) {
		t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "")
		ValidateAgentBillingConfig() // 不 panic、不退出即为通过
	})
	t.Run("带前缀的凭证通过", func(t *testing.T) {
		t.Setenv("AGENT_BILLING_SERVICE_TOKEN", AgentBillingCredentialPrefix+"deadbeef")
		ValidateAgentBillingConfig()
	})
}

// 前缀契约本身：改了常量而没改分流，或反过来，这条会挂。
func TestAgentBillingCredentialPrefixIsRoutable(t *testing.T) {
	if AgentBillingCredentialPrefix == "" {
		t.Fatal("前缀不能为空：RelayAuth 靠它分流")
	}
	if strings.HasPrefix("sk-anything", AgentBillingCredentialPrefix) {
		t.Fatal("前缀不得与普通 sk- token 冲突，否则用户 token 会被当成网关凭证")
	}
}

// 库抖动和「这个用户被封了」是两件事，合成一个裸 403 会把排查带到完全错误的方向：
// user cache 抖一下，所有在飞的 Run 都变成「模型调用失败」，而 403 读起来像「不该花钱」。
// TokenAuth 一直是分开的（500 MsgDatabaseError / 403 MsgAuthUserBanned），这里对齐它。
func TestAgentBillingSubjectLookupSeparatesOutageFromBan(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")
	headers := map[string]string{
		"Authorization":           "bwsvc_right",
		AgentBillingSubjectHeader: "37",
	}

	cases := []struct {
		name       string
		lookup     func(int) (*model.UserBase, error)
		wantStatus int
		wantBody   string
	}{
		{
			name:       "查库失败是 500，不是 403——它不是「这个用户不该花钱」",
			lookup:     func(int) (*model.UserBase, error) { return nil, errors.New("user cache down") },
			wantStatus: http.StatusInternalServerError,
			wantBody:   "database_error",
		},
		{
			name: "用户被停用才是 403",
			lookup: func(int) (*model.UserBase, error) {
				return &model.UserBase{Id: 37, Status: common.UserStatusDisabled}, nil
			},
			wantStatus: http.StatusForbidden,
			wantBody:   "user_banned",
		},
		{
			name: "正常用户放行",
			lookup: func(int) (*model.UserBase, error) {
				return &model.UserBase{Id: 37, Status: common.UserStatusEnabled}, nil
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := lookupBillingSubject
			lookupBillingSubject = tc.lookup
			defer func() { lookupBillingSubject = orig }()

			recorder := agentBillingRequest(t, "/v1/chat/completions", headers)
			assert.Equal(t, tc.wantStatus, recorder.Code)
			if tc.wantBody != "" {
				assert.Contains(t, recorder.Body.String(), tc.wantBody,
					"错误码要能把两种成因分开，否则日志里看不出是哪一种")
			}
		})
	}
}

// IKCGF1 ④：subject 头只在认出蓝鲸服务凭证时才被信任。
//
// 这把 `bwsvc_` 凭证能为**系统里任何用户**记账，所以「普通 sk- token 带着 subject 头会不会
// 被当真」是整条链上最贵的一个问题——而在本用例之前，没有任何东西守着它。
//
// `RelayAuth` 按前缀分流：不带 `bwsvc_` 前缀的凭证走 `TokenAuth()`，那条路径压根不看
// subject 头。这里断言的就是「没进网关分支」——agent 计费的三个 context key 一个都没被设上。
//
// 没有 DB 时 `TokenAuth` 会 nil 解引用 panic。那是环境不是被测行为：**panic 发生在断言的
// 那些 key 之后**（上面的探测确认过 code=200、两个 key 都是 nil），所以 recover 掉继续断言是
// 成立的，而不是把失败吞了。
func TestRelayAuthDoesNotTrustSubjectHeaderFromOrdinaryToken(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")

	for _, credential := range []string{
		"Bearer sk-someones-real-token",
		"sk-someones-real-token",
		"Bearer bwsvc_wrong", // 前缀对但值不对：必须走网关分支并被 401，不能落回 TokenAuth
	} {
		t.Run(credential, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Request.Header.Set("Authorization", credential)
			// 攻击面：调用方自己声明想记谁的账。
			c.Request.Header.Set(AgentBillingSubjectHeader, "1")
			c.Request.Header.Set(AgentBillingRunHeader, "run_forged")

			func() {
				defer func() { _ = recover() }() // 见函数注释：无 DB 的 TokenAuth 会 panic
				RelayAuth()(c)
			}()

			assert.Nil(t, c.Keys[string(constant.ContextKeyAgentBilling)],
				"没有正确的网关凭证就不该进入网关计费分支")
			assert.Nil(t, c.Keys[string(constant.ContextKeyAgentRunId)],
				"run 标签也只在网关分支里才落")
			assert.Nil(t, c.Keys["id"],
				"计费主体绝不能来自调用方声明的 subject 头")
		})
	}
}

// 反向对照：凭证对的时候，subject 头**必须**被采信——否则上面那条用例即使把整个网关分支
// 删掉也会绿。
func TestRelayAuthTrustsSubjectHeaderFromGatewayCredential(t *testing.T) {
	t.Setenv("AGENT_BILLING_SERVICE_TOKEN", "bwsvc_right")
	orig := lookupBillingSubject
	lookupBillingSubject = func(id int) (*model.UserBase, error) {
		return &model.UserBase{Id: id, Status: common.UserStatusEnabled, Group: "default"}, nil
	}
	defer func() { lookupBillingSubject = orig }()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Authorization", "Bearer bwsvc_right")
	c.Request.Header.Set(AgentBillingSubjectHeader, "37")
	c.Request.Header.Set(AgentBillingRunHeader, "run_real")

	RelayAuth()(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, true, c.Keys[string(constant.ContextKeyAgentBilling)])
	assert.Equal(t, "run_real", c.Keys[string(constant.ContextKeyAgentRunId)])
	assert.Equal(t, 37, c.Keys["id"])
	assert.Equal(t, true, c.Keys[string(constant.ContextKeyTokenUnlimited)], "tokenless：token 层不设限")
}
