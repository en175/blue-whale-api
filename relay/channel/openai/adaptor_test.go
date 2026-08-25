package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvertOpenAIRequestStreamOptions verifies that ConvertOpenAIRequest
// preserves StreamOptions when the channel supports it (IKB1FV regression).
//
// The bug: ConvertOpenAIRequest stripped StreamOptions for ALL non-OpenAI
// /non-Azure channels, including Volcengine which supports stream_options.
// ConvertClaudeRequest injects StreamOptions before calling this method,
// so stripping it here prevented upstreams from returning usage data,
// breaking billing for Claude-format clients.
func TestConvertOpenAIRequestStreamOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                string
		channelType         int
		supportStreamOpts   bool
		expectStreamOptions bool
	}{
		{
			name:                "openai_channel_preserves",
			channelType:         constant.ChannelTypeOpenAI,
			supportStreamOpts:   true,
			expectStreamOptions: true,
		},
		{
			name:                "azure_channel_preserves",
			channelType:         constant.ChannelTypeAzure,
			supportStreamOpts:   true,
			expectStreamOptions: true,
		},
		{
			name:                "volcengine_with_support_preserves",
			channelType:         constant.ChannelTypeVolcEngine,
			supportStreamOpts:   true,
			expectStreamOptions: true,
		},
		{
			name:                "volcengine_without_support_strips",
			channelType:         constant.ChannelTypeVolcEngine,
			supportStreamOpts:   false,
			expectStreamOptions: false,
		},
		{
			name:                "unsupported_channel_strips",
			channelType:         999,
			supportStreamOpts:   false,
			expectStreamOptions: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:          tt.channelType,
					SupportStreamOptions: tt.supportStreamOpts,
					UpstreamModelName:    "gpt-test",
				},
			}

			request := &dto.GeneralOpenAIRequest{
				Model:  "gpt-test",
				Stream: boolPtr(true),
				StreamOptions: &dto.StreamOptions{
					IncludeUsage: true,
				},
			}

			result, err := (&Adaptor{}).ConvertOpenAIRequest(c, info, request)
			require.NoError(t, err)

			converted, ok := result.(*dto.GeneralOpenAIRequest)
			require.True(t, ok, "expected *GeneralOpenAIRequest, got %T", result)

			if tt.expectStreamOptions {
				require.NotNil(t, converted.StreamOptions,
					"StreamOptions must be preserved when SupportStreamOptions=true")
				assert.True(t, converted.StreamOptions.IncludeUsage)
			} else {
				assert.Nil(t, converted.StreamOptions,
					"StreamOptions must be stripped when SupportStreamOptions=false")
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }
