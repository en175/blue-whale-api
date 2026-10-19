package novitagemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{name: "mask is rejected", request: dto.ImageRequest{Prompt: "a cat", Mask: json.RawMessage(`"data:image/png;base64,QUJD"`)}},
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
		name        string
		value       string
		mimeType    string
		data        string
		decodedSize int
		expectError bool
	}{
		{name: "png data url", value: "data:image/png;base64,QUJD", mimeType: "image/png", data: "QUJD", decodedSize: 3},
		{name: "remote url", value: "https://example.com/a.png", expectError: true},
		{name: "non base64 data url", value: "data:image/png,QUJD", expectError: true},
		{name: "non image mime", value: "data:text/plain;base64,QUJD", expectError: true},
		{name: "empty payload", value: "data:image/png;base64,", expectError: true},
		{name: "invalid base64 payload", value: "data:image/png;base64,!!!", expectError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mimeType, data, decodedSize, err := parseImageDataURL(tc.value)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.mimeType, mimeType)
			assert.Equal(t, tc.data, data)
			assert.Equal(t, tc.decodedSize, decodedSize)
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

func TestConvertImageRequestEditRejectsInvalidDataURLPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	request := dto.ImageRequest{
		Prompt: "edit",
		Images: json.RawMessage(`[{"image_url":"data:image/png;base64,!!!"}]`),
	}
	_, err := adaptor.ConvertImageRequest(c, info, request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base64 payload is invalid")
}

func TestConvertImageRequestEditFromMixedMultipartFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	pngBytes := append([]byte("\x89PNG\r\n\x1a\n"), []byte("fake-image-body")...)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("prompt", "combine these"))
	for _, field := range []string{"image", "image[]", "image[1]", "image[2]"} {
		part, err := writer.CreateFormFile(field, field+".png")
		require.NoError(t, err)
		_, err = part.Write(pngBytes)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(32<<20))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{Prompt: "combine these"})
	require.NoError(t, err)

	body, ok := converted.(dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, body.Contents[0].Parts, 5, "prompt plus four image parts")
	for i, part := range body.Contents[0].Parts[1:] {
		require.NotNil(t, part.InlineData, "part %d should be an inline image", i)
		assert.Equal(t, "image/png", part.InlineData.MimeType)
	}
}

func TestConvertImageRequestEditRejectsTooManyMultipartImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("prompt", "too many"))
	for i := 0; i <= maxInputImages; i++ {
		part, err := writer.CreateFormFile(fmt.Sprintf("image[%d]", i), "a.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(32<<20))

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	_, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{Prompt: "too many"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many input images")
}

func TestDoRequestRejectsPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
		},
	}

	_, err := adaptor.DoRequest(c, info, strings.NewReader(`{"prompt":"a cat"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support request body passthrough")
	assert.Equal(t, http.StatusBadRequest, types.NewError(err, types.ErrorCodeConvertRequestFailed).StatusCode)
}

func TestNovitaGeminiImageHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newResponse := func(body string) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}
	newContext := func() (*gin.Context, *httptest.ResponseRecorder) {
		writer := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(writer)
		return c, writer
	}

	t.Run("collects every inline image and skips empty parts", func(t *testing.T) {
		c, writer := newContext()
		body := `{"candidates":[
			{"content":{"parts":[{"text":"here"},{"inlineData":{"mimeType":"image/png","data":"QUJD"}},{"inlineData":{"mimeType":"image/png","data":""}}]}},
			{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"REVG"}}]}}
		]}`
		usage, apiErr := novitaGeminiImageHandler(c, newResponse(body), &relaycommon.RelayInfo{})
		require.Nil(t, apiErr)
		require.NotNil(t, usage)

		var imageResponse dto.ImageResponse
		require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &imageResponse))
		require.Len(t, imageResponse.Data, 2)
		assert.Equal(t, "QUJD", imageResponse.Data[0].B64Json)
		assert.Equal(t, "REVG", imageResponse.Data[1].B64Json)
	})

	t.Run("content policy finish reason becomes 400 and keeps the reason", func(t *testing.T) {
		c, _ := newContext()
		body := `{"candidates":[{"content":{"role":"model","parts":null},"finishReason":"IMAGE_RECITATION","finishMessage":"Unable to show the generated image."}]}`
		_, apiErr := novitaGeminiImageHandler(c, newResponse(body), &relaycommon.RelayInfo{})
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Contains(t, apiErr.Error(), "IMAGE_RECITATION")
		assert.Contains(t, apiErr.Error(), "Unable to show the generated image.")
	})

	t.Run("prompt feedback block reason becomes 400", func(t *testing.T) {
		c, _ := newContext()
		_, apiErr := novitaGeminiImageHandler(c, newResponse(`{"promptFeedback":{"blockReason":"SAFETY"}}`), &relaycommon.RelayInfo{})
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.Contains(t, apiErr.Error(), "SAFETY")
	})

	t.Run("non-policy finish reason stays 500 with the reason", func(t *testing.T) {
		c, _ := newContext()
		_, apiErr := novitaGeminiImageHandler(c, newResponse(`{"candidates":[{"finishReason":"OTHER"}]}`), &relaycommon.RelayInfo{})
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		assert.Contains(t, apiErr.Error(), "OTHER")
	})

	t.Run("invalid json is rejected", func(t *testing.T) {
		c, _ := newContext()
		_, apiErr := novitaGeminiImageHandler(c, newResponse(`not-json`), &relaycommon.RelayInfo{})
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	})
}
