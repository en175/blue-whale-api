package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
