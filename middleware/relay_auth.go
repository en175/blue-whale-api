package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// RelayAuth picks the credential scheme for a relay request.
//
// Two exist. An ordinary `sk-` token authenticates a caller who is spending their own balance, and
// is resolved by TokenAuth against the token table. The Blue Whale model gateway instead relays a
// user's Agent traffic under its own service credential, naming the billing subject in a header —
// that one is routed by prefix, because it has no token row to look up.
//
// A third used to live here: the Run Capability (`bwcap_`), an opaque per-Run token carrying its
// own subject and bounds. It was retired on 2026-09-01 — the subject inside it was chosen by the
// party being billed and verified by nobody, and the bounds were partly inert and partly harmful.
// See docs/decisions/2026-09-01-retire-run-capability-for-identity-propagation.md in
// blue-whale-agent (IKCGEF).
func RelayAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		credential := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		if strings.HasPrefix(credential, AgentBillingCredentialPrefix) {
			AgentBillingAuth(c, credential)
			return
		}
		TokenAuth()(c)
	}
}
