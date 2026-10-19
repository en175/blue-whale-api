package novitagemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURL(t *testing.T) {
	adaptor := &Adaptor{}

	tests := []struct {
		name        string
		baseURL     string
		modelName   string
		expectedURL string
		expectError bool
	}{
		{
			name:        "base url without trailing slash",
			baseURL:     "https://api.novita.ai/gemini",
			modelName:   "gemini-3-pro-image",
			expectedURL: "https://api.novita.ai/gemini/v1/models/gemini-3-pro-image:generateContent",
		},
		{
			name:        "base url with trailing slash",
			baseURL:     "https://api.novita.ai/gemini/",
			modelName:   "gemini-nano-banana-2.1",
			expectedURL: "https://api.novita.ai/gemini/v1/models/gemini-nano-banana-2.1:generateContent",
		},
		{
			name:        "empty model name is rejected",
			baseURL:     "https://api.novita.ai/gemini",
			modelName:   "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl:    tc.baseURL,
					UpstreamModelName: tc.modelName,
				},
			}
			url, err := adaptor.GetRequestURL(info)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedURL, url)
		})
	}
}

func TestConvertImageRequestGenerations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{Prompt: "a cute cat"})
	require.NoError(t, err)

	body, ok := converted.(dto.GeminiChatRequest)
	require.True(t, ok, "converted request should be a GeminiChatRequest")
	require.Len(t, body.Contents, 1)
	assert.Equal(t, "user", body.Contents[0].Role)
	require.Len(t, body.Contents[0].Parts, 1)
	assert.Equal(t, "a cute cat", body.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"IMAGE"}, body.GenerationConfig.ResponseModalities)
	assert.Empty(t, body.GenerationConfig.ImageConfig, "size must not be mapped into imageConfig")
}

func TestConvertImageRequestRejections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	n := uint(2)
	stream := true

	tests := []struct {
		name    string
		request dto.ImageRequest
	}{
		{name: "n greater than one", request: dto.ImageRequest{Prompt: "a cat", N: &n}},
		{name: "streaming", request: dto.ImageRequest{Prompt: "a cat", Stream: &stream}},
		{name: "url response format", request: dto.ImageRequest{Prompt: "a cat", ResponseFormat: "url"}},
		{name: "empty prompt", request: dto.ImageRequest{Prompt: "   "}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adaptor.ConvertImageRequest(c, info, tc.request)
			require.Error(t, err)

			// ImageHelper 会用 types.NewError 再包一层；校验失败必须仍然是 400，
			// 不能被降级成默认的 500。
			wrapped := types.NewError(err, types.ErrorCodeConvertRequestFailed)
			assert.Equal(t, http.StatusBadRequest, wrapped.StatusCode)
		})
	}
}

func TestConvertImageRequestEditFromJSONDataURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	request := dto.ImageRequest{
		Prompt: "Add a party hat to this cat",
		Images: json.RawMessage(`[{"image_url":"data:image/png;base64,QUJD"}]`),
	}
	converted, err := adaptor.ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	body, ok := converted.(dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, body.Contents[0].Parts, 2)
	assert.Equal(t, "Add a party hat to this cat", body.Contents[0].Parts[0].Text)
	require.NotNil(t, body.Contents[0].Parts[1].InlineData)
	assert.Equal(t, "image/png", body.Contents[0].Parts[1].InlineData.MimeType)
	assert.Equal(t, "QUJD", body.Contents[0].Parts[1].InlineData.Data)
	assert.Equal(t, []string{"TEXT", "IMAGE"}, body.GenerationConfig.ResponseModalities)
}

func TestConvertImageRequestEditRejectsRemoteURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	request := dto.ImageRequest{
		Prompt: "edit",
		Images: json.RawMessage(`[{"image_url":"https://example.com/cat.png"}]`),
	}
	_, err := adaptor.ConvertImageRequest(c, info, request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data URL")
}

func TestConvertImageRequestEditFromMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	pngBytes := append([]byte("\x89PNG\r\n\x1a\n"), []byte("fake-image-body")...)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("prompt", "Add a party hat"))
	filePart, err := writer.CreateFormFile("image[]", "cat.png")
	require.NoError(t, err)
	_, err = filePart.Write(pngBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(32<<20))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{Prompt: "Add a party hat"})
	require.NoError(t, err)

	body, ok := converted.(dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, body.Contents[0].Parts, 2)
	require.NotNil(t, body.Contents[0].Parts[1].InlineData)
	assert.Equal(t, "image/png", body.Contents[0].Parts[1].InlineData.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngBytes), body.Contents[0].Parts[1].InlineData.Data)
}

func TestBuildImageUsage(t *testing.T) {
	t.Run("usage metadata tokens are billed as-is", func(t *testing.T) {
		response := &dto.GeminiChatResponse{
			UsageMetadata: dto.GeminiUsageMetadata{
				PromptTokenCount:     100,
				CandidatesTokenCount: 200,
				ThoughtsTokenCount:   50,
				TotalTokenCount:      350,
			},
		}
		usage := buildImageUsage(response, 1)
		assert.Equal(t, 100, usage.PromptTokens)
		assert.Equal(t, 250, usage.CompletionTokens)
		assert.Equal(t, 350, usage.TotalTokens)
		require.NotNil(t, usage.BillingUsage)
	})

	t.Run("zero completion tokens fall back to per-image estimate", func(t *testing.T) {
		response := &dto.GeminiChatResponse{
			UsageMetadata: dto.GeminiUsageMetadata{
				PromptTokenCount: 100,
				TotalTokenCount:  100,
			},
		}
		usage := buildImageUsage(response, 1)
		assert.Equal(t, 100, usage.PromptTokens)
		assert.Equal(t, 1400, usage.CompletionTokens)
		assert.Equal(t, 1500, usage.TotalTokens)
		require.NotNil(t, usage.BillingUsage)
	})

	t.Run("missing metadata falls back to per-image estimate", func(t *testing.T) {
		usage := buildImageUsage(&dto.GeminiChatResponse{}, 2)
		assert.Zero(t, usage.PromptTokens)
		assert.Equal(t, 2800, usage.CompletionTokens)
		assert.Equal(t, 2800, usage.TotalTokens)
		require.NotNil(t, usage.BillingUsage)
	})
}

func TestParseImageDataURL(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		mimeType string
		data     string
		ok       bool
	}{
		{name: "png data url", value: "data:image/png;base64,QUJD", mimeType: "image/png", data: "QUJD", ok: true},
		{name: "remote url", value: "https://example.com/a.png", ok: false},
		{name: "non base64 data url", value: "data:image/png,QUJD", ok: false},
		{name: "non image mime", value: "data:text/plain;base64,QUJD", ok: false},
		{name: "empty payload", value: "data:image/png;base64,", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mimeType, data, ok := parseImageDataURL(tc.value)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.mimeType, mimeType)
				assert.Equal(t, tc.data, data)
			}
		})
	}
}

func TestModelListExcludesRejectedModel(t *testing.T) {
	assert.NotContains(t, ModelList, "gemini-3.1-flash-lite-image")
	assert.Contains(t, ModelList, "gemini-3.1-flash-lite-image-as")
}

func TestConvertImageRequestComputedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Prompt: "a cat",
		Size:   "1024x1024",
		Extra:  map[string]json.RawMessage{"ignored": json.RawMessage(`true`)},
	})
	require.NoError(t, err)

	raw, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"contents": [{"role": "user", "parts": [{"text": "a cat"}]}],
		"generationConfig": {"responseModalities": ["IMAGE"]}
	}`, string(raw))
}
