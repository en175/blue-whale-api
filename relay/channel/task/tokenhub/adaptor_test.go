package tokenhub

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// succeeded 的 resultSummary 信封是 ai-tokenhub 与 doubao 的关键差异：
// video_url / usage / upstreamStatus 都包在 resultSummary 下，而非顶层字段。
// video_url 用占位直链替换真实带签名的 TOS 地址（凭据不入库）；其余字段为实测值。
const succeededFixture = `{"id":"019ffef2-ac64-7693-8e7d-e727c39795f9","upstreamTaskId":"cgt-20260814142545-pmz7b","status":"succeeded","model":"dreamina-seedance-2-0-mini-260615","resultSummary":{"content":{"video_url":"https://example.volces.com/seedance.mp4"},"duration":"5","resolution":"720p","upstreamStatus":"succeeded","usage":{"completion_tokens":108900,"total_tokens":108900}},"createdAt":"2026-08-14T14:25:44.547682+08:00","updatedAt":"2026-08-14T14:28:39.929951+08:00"}`

func TestParseTaskResult(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantStatus     string
		wantProgress   string
		wantURL        string
		wantCompletion int
		wantTotal      int
		wantReason     string
	}{
		{
			name:         "submitted maps to queued",
			body:         `{"id":"019ffef2-ac64-7693-8e7d-e727c39795f9","upstreamTaskId":"cgt-20260814142545-pmz7b","status":"submitted","model":"dreamina-seedance-2-0-mini-260615","createdAt":"2026-08-14T14:25:44.547682+08:00"}`,
			wantStatus:   string(model.TaskStatusQueued),
			wantProgress: "10%",
		},
		{
			name:         "running maps to in-progress",
			body:         `{"id":"019fff5f-de70-7126-a2be-be87a9801368","upstreamTaskId":"cgt-20260814162501-qtx2g","status":"running","model":"dreamina-seedance-2-0-mini-260615","resultSummary":{"upstreamStatus":"running"},"createdAt":"2026-08-14T16:25:00.783419+08:00","updatedAt":"2026-08-14T16:26:44.70757+08:00"}`,
			wantStatus:   string(model.TaskStatusInProgress),
			wantProgress: "50%",
		},
		{
			name:           "succeeded extracts resultSummary url and tokens",
			body:           succeededFixture,
			wantStatus:     string(model.TaskStatusSuccess),
			wantProgress:   "100%",
			wantURL:        "https://example.volces.com/seedance.mp4",
			wantCompletion: 108900,
			wantTotal:      108900,
		},
		{
			name:         "failed maps to failure",
			body:         `{"id":"x","status":"failed","model":"dreamina-seedance-2-0-mini-260615","resultSummary":{"upstreamStatus":"failed"}}`,
			wantStatus:   string(model.TaskStatusFailure),
			wantProgress: "100%",
			wantReason:   "upstream task failed",
		},
		{
			name:           "upstreamStatus absent falls back to top-level status",
			body:           `{"id":"x","status":"succeeded","model":"dreamina-seedance-2-0-mini-260615","resultSummary":{"content":{"video_url":"https://example.volces.com/seedance.mp4"},"usage":{"completion_tokens":1,"total_tokens":1}}}`,
			wantStatus:     string(model.TaskStatusSuccess),
			wantProgress:   "100%",
			wantURL:        "https://example.volces.com/seedance.mp4",
			wantCompletion: 1,
			wantTotal:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &TaskAdaptor{}
			r, err := a.ParseTaskResult([]byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, r.Status)
			assert.Equal(t, tt.wantProgress, r.Progress)
			assert.Equal(t, tt.wantURL, r.Url)
			assert.Equal(t, tt.wantCompletion, r.CompletionTokens)
			assert.Equal(t, tt.wantTotal, r.TotalTokens)
			assert.Equal(t, tt.wantReason, r.Reason)
		})
	}
}
