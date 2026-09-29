package taskcommon

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestIsDraftRequest(t *testing.T) {
	trueVal := dto.BoolValue(true)
	falseVal := dto.BoolValue(false)

	tests := []struct {
		name string
		req  *relaycommon.TaskSubmitReq
		want bool
	}{
		{name: "nil request", req: nil, want: false},
		{name: "empty request", req: &relaycommon.TaskSubmitReq{}, want: false},
		{name: "top-level draft true", req: &relaycommon.TaskSubmitReq{Draft: &trueVal}, want: true},
		{name: "top-level draft false", req: &relaycommon.TaskSubmitReq{Draft: &falseVal}, want: false},
		{
			name: "metadata draft bool true",
			req:  &relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"draft": true}},
			want: true,
		},
		{
			name: "metadata draft bool false",
			req:  &relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"draft": false}},
			want: false,
		},
		{
			name: "metadata draft string true",
			req:  &relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"draft": "true"}},
			want: true,
		},
		{
			name: "metadata draft string padded mixed case",
			req:  &relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"draft": "  TRUE "}},
			want: true,
		},
		{
			name: "metadata draft string false",
			req:  &relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"draft": "false"}},
			want: false,
		},
		{
			name: "metadata draft other type",
			req:  &relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"draft": 1}},
			want: false,
		},
		{
			name: "top-level wins over metadata",
			req: &relaycommon.TaskSubmitReq{
				Draft:    &trueVal,
				Metadata: map[string]interface{}{"draft": false},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDraftRequest(tt.req))
		})
	}
}
