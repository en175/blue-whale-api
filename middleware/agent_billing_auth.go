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

func agentBillingCredential() string {
	return strings.TrimSpace(os.Getenv("AGENT_BILLING_SERVICE_TOKEN"))
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
	user, err := model.GetUserCache(subjectUserId)
	if err != nil || user.Status != common.UserStatusEnabled {
		abortWithOpenAiMessage(c, http.StatusForbidden, "Agent billing subject unavailable")
		return
	}
	c.Set("id", subjectUserId)
	user.WriteContext(c)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, user.Group)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "")
	common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
	// Reuses the capability flag purely for its tokenless-billing meaning (skip token pre-consume
	// and token quota decrement). The name outlives the capability by one ticket: IKCGEF deletes
	// the capability and renames it to what it has always actually meant.
	common.SetContextKey(c, constant.ContextKeyRunCapabilityBilling, true)
	common.SetContextKey(c, constant.ContextKeyAgentBilling, true)
	if runId := strings.TrimSpace(c.GetHeader(AgentBillingRunHeader)); runId != "" {
		common.SetContextKey(c, constant.ContextKeyAgentRunId, runId)
	}
	c.Next()
}
