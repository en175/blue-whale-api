package highwayapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

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
			baseURL:     "https://api.highwayapi.ai/v3",
			modelName:   "gpt-image-2-text-to-image",
			expectedURL: "https://api.highwayapi.ai/v3/gpt-image-2-text-to-image",
		},
		{
			name:        "base url with trailing slash",
			baseURL:     "https://api.highwayapi.ai/v3/",
			modelName:   "gpt-image-2-light-text-to-image",
			expectedURL: "https://api.highwayapi.ai/v3/gpt-image-2-light-text-to-image",
		},
		{
			name:        "custom base url",
			baseURL:     "https://proxy.example.com/highway/v3",
			modelName:   "some-future-model",
			expectedURL: "https://proxy.example.com/highway/v3/some-future-model",
		},
		{
			name:        "empty model name is rejected",
			baseURL:     "https://api.highwayapi.ai/v3",
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

func TestConvertImageRequest(t *testing.T) {
	adaptor := &Adaptor{}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	t.Run("known fields are passed through, absent fields omitted", func(t *testing.T) {
		n := uint(3)
		request := dto.ImageRequest{
			Prompt:            "a cat",
			N:                 &n,
			Size:              "1536x1024",
			Quality:           "high",
			Background:        json.RawMessage(`"auto"`),
			Moderation:        json.RawMessage(`"low"`),
			OutputFormat:      json.RawMessage(`"jpeg"`),
			OutputCompression: json.RawMessage(`80`),
		}

		converted, err := adaptor.ConvertImageRequest(c, info, request)
		require.NoError(t, err)

		body, ok := converted.(map[string]json.RawMessage)
		require.True(t, ok, "converted request should be a JSON object map")

		assert.JSONEq(t, `"a cat"`, string(body["prompt"]))
		assert.JSONEq(t, `3`, string(body["n"]))
		assert.JSONEq(t, `"1536x1024"`, string(body["size"]))
		assert.JSONEq(t, `"high"`, string(body["quality"]))
		assert.JSONEq(t, `"auto"`, string(body["background"]))
		assert.JSONEq(t, `"low"`, string(body["moderation"]))
		assert.JSONEq(t, `"jpeg"`, string(body["output_format"]))
		assert.JSONEq(t, `80`, string(body["output_compression"]))

		// model 拼接进请求路径，请求体中不应出现
		_, hasModel := body["model"]
		assert.False(t, hasModel)
	})

	t.Run("absent fields are omitted", func(t *testing.T) {
		request := dto.ImageRequest{Prompt: "a dog"}

		converted, err := adaptor.ConvertImageRequest(c, info, request)
		require.NoError(t, err)
		body, ok := converted.(map[string]json.RawMessage)
		require.True(t, ok, "converted request should be a JSON object map")

		assert.JSONEq(t, `"a dog"`, string(body["prompt"]))
		for _, key := range []string{"n", "size", "quality", "background", "moderation", "output_format", "output_compression"} {
			_, exists := body[key]
			assert.False(t, exists, "key %s should be omitted when unset", key)
		}
	})

	t.Run("unknown client fields are passed through", func(t *testing.T) {
		request := dto.ImageRequest{
			Prompt: "a bird",
			Extra: map[string]json.RawMessage{
				"new_upstream_param": json.RawMessage(`{"flag":true}`),
			},
		}

		converted, err := adaptor.ConvertImageRequest(c, info, request)
		require.NoError(t, err)
		body, ok := converted.(map[string]json.RawMessage)
		require.True(t, ok, "converted request should be a JSON object map")

		assert.JSONEq(t, `{"flag":true}`, string(body["new_upstream_param"]))
	})

	t.Run("extra_fields object is merged into body", func(t *testing.T) {
		request := dto.ImageRequest{
			Prompt:      "a fish",
			ExtraFields: json.RawMessage(`{"seed":42,"guidance":1.5}`),
		}

		converted, err := adaptor.ConvertImageRequest(c, info, request)
		require.NoError(t, err)
		body, ok := converted.(map[string]json.RawMessage)
		require.True(t, ok, "converted request should be a JSON object map")

		assert.JSONEq(t, `42`, string(body["seed"]))
		assert.JSONEq(t, `1.5`, string(body["guidance"]))
	})

	t.Run("b64_json response format is rejected", func(t *testing.T) {
		request := dto.ImageRequest{Prompt: "a cat", ResponseFormat: "b64_json"}
		_, err := adaptor.ConvertImageRequest(c, info, request)
		require.Error(t, err)
	})

	t.Run("image edits are rejected", func(t *testing.T) {
		editInfo := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
		_, err := adaptor.ConvertImageRequest(c, editInfo, dto.ImageRequest{Prompt: "a cat"})
		require.Error(t, err)
	})
}

func TestParseImageResponse(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		expectedData []dto.ImageData
		expectError  bool
	}{
		{
			name: "images field carries urls",
			body: `{"images": ["https://cdn.example.com/a.png", "https://cdn.example.com/b.png"]}`,
			expectedData: []dto.ImageData{
				{Url: "https://cdn.example.com/a.png"},
				{Url: "https://cdn.example.com/b.png"},
			},
		},
		{
			name: "image_urls field is used when images is empty",
			body: `{"images": [], "image_urls": ["https://cdn.example.com/c.png"]}`,
			expectedData: []dto.ImageData{
				{Url: "https://cdn.example.com/c.png"},
			},
		},
		{
			name: "non-http values fall back to b64_json",
			body: `{"images": ["iVBORw0KGgoAAAANSUhEUg=="]}`,
			expectedData: []dto.ImageData{
				{B64Json: "iVBORw0KGgoAAAANSUhEUg=="},
			},
		},
		{
			name: "empty string elements are skipped",
			body: `{"images": ["", "https://cdn.example.com/d.png", ""]}`,
			expectedData: []dto.ImageData{
				{Url: "https://cdn.example.com/d.png"},
			},
		},
		{
			name:        "empty image list is rejected",
			body:        `{"images": []}`,
			expectError: true,
		},
		{
			name:        "all empty elements are rejected",
			body:        `{"images": ["", ""]}`,
			expectError: true,
		},
		{
			name:        "missing image fields are rejected",
			body:        `{"status": "ok"}`,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var payload imageResponsePayload
			require.NoError(t, common.Unmarshal([]byte(tc.body), &payload))

			data, err := parseImageResponse(&payload)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectedData, data)
		})
	}
}

func TestHighwayImageHandlerWritesOpenAIFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	info := &relaycommon.RelayInfo{}
	info.StartTime = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	upstreamBody := `{"images": ["https://cdn.example.com/x.png"]}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}

	usage, apiErr := highwayImageHandler(c, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage, "handler must return non-nil usage for ImageHelper billing")

	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(writer.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "https://cdn.example.com/x.png", imageResponse.Data[0].Url)
	assert.Equal(t, info.StartTime.Unix(), imageResponse.Created)
	assert.Equal(t, http.StatusOK, writer.Code)
	assert.Equal(t, "application/json", writer.Header().Get("Content-Type"))
}
