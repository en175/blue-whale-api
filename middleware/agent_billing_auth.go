package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AgentBillingCredentialPrefix marks the Blue Whale model gateway's own service credential.
// The prefix exists so the relay entrypoint can route on it without probing the token table,
// exactly as `bwcap_` did for the Run Capability it replaces.
const AgentBillingCredentialPrefix = "bwsvc_"

// AgentBillingSubjectHeader carries the user the call is billed to. It is only ever written by
// the Blue Whale model gateway, which derives it from `project.owner_id` — the Agent runtime
// cannot set it, because the gateway strips any inbound copy before forwarding.
const AgentBillingSubjectHeader = "X-Bw-Subject-User-Id"

// AgentBillingRunHeader is a ledger label, not an authorization input: it lands in the log row so
// spend can be read back per Agent run. Nothing downstream trusts it.
const AgentBillingRunHeader = "X-Bw-Agent-Run-Id"

// 计费主体的查库入口，做成变量只为可测：库错与封号必须分成 500 / 403 两条路，而这两条路
// 在真实实现下都要一个活着的 DB 才能走到。生产路径就是 model.GetUserCache。
var lookupBillingSubject = model.GetUserCache

func agentBillingCredential() string {
	return strings.TrimSpace(os.Getenv("AGENT_BILLING_SERVICE_TOKEN"))
}

// ValidateAgentBillingConfig checks the gateway credential at boot instead of at request time.
//
// Both failure modes were silent before, and both look like a model outage from the browser:
//
//   - **Unset.** Every gateway call 401s. Nothing is logged at startup, so the first sign is users
//     reporting that the Agent stopped answering.
//   - **Set without the `bwsvc_` prefix.** Worse, because it looks configured. `RelayAuth` routes
//     on the prefix, so a prefix-less credential never reaches this middleware at all — it falls
//     through to `TokenAuth`, fails to find a token row, and dies as an ordinary 401.
//
// Fatal on the second: the operator plainly meant to enable agent billing and it would silently
// not work. Only a log line on the first, because a deployment that does not run the Blue Whale
// Agent is legitimate and must still boot.
func ValidateAgentBillingConfig() {
	credential := agentBillingCredential()
	if credential == "" {
		common.SysLog("AGENT_BILLING_SERVICE_TOKEN 未配置：蓝鲸网关代打不可用，/v1/chat/completions 上的网关调用会一律 401")
		return
	}
	if !strings.HasPrefix(credential, AgentBillingCredentialPrefix) {
		common.FatalLog("AGENT_BILLING_SERVICE_TOKEN 必须以 " + AgentBillingCredentialPrefix +
			" 开头：relay 入口按前缀分流，不带前缀的凭证永远走不到网关分支，会以普通 401 静默失败")
		return
	}
	common.SysLog("蓝鲸网关代打已启用（AGENT_BILLING_SERVICE_TOKEN 已配置）")
}

// authorizedAgentBillingService reports whether `supplied` is the configured gateway credential.
// Constant-time: this credential authorizes billing any user in the system, so a timing oracle on
// it is worth more to an attacker than one on an ordinary token.
func authorizedAgentBillingService(supplied string) bool {
	expected := agentBillingCredential()
	if expected == "" || supplied == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(supplied)) == 1
}

// AgentBillingAuth authenticates the Blue Whale model gateway relaying a model call on a user's
// behalf, and installs that user as the billing subject.
//
// This replaces the Run Capability (`bwcap_`) branch. The capability carried its subject inside an
// opaque token, which read as though someone had verified it — nobody had: the Agent runtime put
// `subject_user_id` in the mint request itself. Attribution is now established one hop earlier, by
// the gateway looking the project's owner up in its own database, and asserted here over a
// credential only the gateway holds.
//
// No token row is involved, so the request runs tokenless the same way a capability call did:
// unlimited at the token layer, group taken from the subject user, quota settled straight against
// that user.
func AgentBillingAuth(c *gin.Context, credential string) {
	if !authorizedAgentBillingService(credential) {
		abortWithOpenAiMessage(c, http.StatusUnauthorized, "Agent billing credential invalid")
		return
	}
	// Same scope the capability had. The credential can bill anyone, so it is confined to the one
	// route the Agent runtime actually needs rather than the whole relay surface.
	if c.Request.URL.Path != "/v1/chat/completions" {
		abortWithOpenAiMessage(c, http.StatusForbidden, "Agent billing scope denied")
		return
	}
	subjectHeader := strings.TrimSpace(c.GetHeader(AgentBillingSubjectHeader))
	subjectUserId, err := strconv.Atoi(subjectHeader)
	if err != nil || subjectUserId <= 0 {
		abortWithOpenAiMessage(c, http.StatusBadRequest, "Agent billing subject missing", "billing_subject_required")
		return
	}
	user, err := lookupBillingSubject(subjectUserId)
	if err != nil {
		// 查不到和被封是两件事，不能合成一个裸 403。user cache 抖一下会让所有在飞的 Run 变成
		// 「模型调用失败」，而 403 读起来像「这个用户不该花钱」——排查会往完全错误的方向走。
		// `TokenAuth` 一直是分开的（500 MsgDatabaseError / 403 MsgAuthUserBanned），这里对齐它。
		abortWithOpenAiMessage(c, http.StatusInternalServerError, "Agent billing subject lookup failed", "database_error")
		return
	}
	if user.Status != common.UserStatusEnabled {
		abortWithOpenAiMessage(c, http.StatusForbidden, "Agent billing subject is not enabled", "user_banned")
		return
	}
	c.Set("id", subjectUserId)
	user.WriteContext(c)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "")
	common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
	// Tokenless: no token row exists, so token-level pre-consume and quota decrement are skipped
	// and the user's balance is settled directly. The user-level balance gate in NewBillingSession
	// still runs — a gateway call from an account with no money is refused like any other.
	common.SetContextKey(c, constant.ContextKeyAgentBilling, true)
	if runId := strings.TrimSpace(c.GetHeader(AgentBillingRunHeader)); runId != "" {
		common.SetContextKey(c, constant.ContextKeyAgentRunId, runId)
	}
	c.Next()
}
