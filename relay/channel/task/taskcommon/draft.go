package taskcommon

import (
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// IsDraftRequest reports whether a task submit is a Seedance draft (样片) request.
// Two entry points are supported: Ark's top-level draft parameter, and the
// metadata.draft key used by the new-api task API.
func IsDraftRequest(req *relaycommon.TaskSubmitReq) bool {
	if req == nil {
		return false
	}
	if req.Draft != nil && bool(*req.Draft) {
		return true
	}
	switch v := req.Metadata["draft"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return false
}
