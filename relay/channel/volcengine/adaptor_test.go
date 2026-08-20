package volcengine

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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEditContext 构造带 multipart 图片文件的 /v1/images/edits 请求上下文。
// textFields 模拟表单文本字段；files 的 key 是图片字段名（image / image[]）。
func newEditContext(t *testing.T, files map[string][][]byte, textFields map[string]string) *gin.Context {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range textFields {
		require.NoError(t, writer.WriteField(key, value))
	}
	for fieldName, fieldFiles := range files {
		for _, data := range fieldFiles {
			part, err := writer.CreateFormFile(fieldName, "input.png")
			require.NoError(t, err)
			_, err = part.Write(data)
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return c
}

func convertEdit(c *gin.Context, request dto.ImageRequest) (any, error) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
	return (&Adaptor{}).ConvertImageRequest(c, info, request)
}

// TestConvertImageEditRequestMultipart 验证 IK9ZQM：multipart 图生图图片文件必须
// 转换为 ARK images/generations JSON 的 content[].image_url base64 data URI，
// 而不是像修复前那样在出站 JSON 中丢失。
func TestConvertImageEditRequestMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("single image file becomes base64 data URI in content", func(t *testing.T) {
		c := newEditContext(t, map[string][][]byte{
			"image": {[]byte("fake image")},
		}, map[string]string{"model": "doubao-seedream-4-0-250828", "prompt": "edit this"})
		request := dto.ImageRequest{
			Model:  "doubao-seedream-4-0-250828",
			Prompt: "edit this",
			Size:   "2048x2048",
		}

		converted, err := convertEdit(c, request)
		require.NoError(t, err)
		volcReq, ok := converted.(*volcImageGenerationRequest)
		require.True(t, ok, "converted request should be volcImageGenerationRequest")
		require.Len(t, volcReq.Content, 1)
		assert.Equal(t, "image_url", volcReq.Content[0].Type)
		require.NotNil(t, volcReq.Content[0].ImageURL)

		prefix := "data:image/png;base64,"
		url := volcReq.Content[0].ImageURL.URL
		assert.True(t, bytes.HasPrefix([]byte(url), []byte(prefix)), "url should be data URI: %s", url)
		decoded, err := base64.StdEncoding.DecodeString(url[len(prefix):])
		require.NoError(t, err)
		assert.Equal(t, []byte("fake image"), decoded)

		// 字段保留：model / prompt / size
		assert.Equal(t, "doubao-seedream-4-0-250828", volcReq.Model)
		assert.Equal(t, "edit this", volcReq.Prompt)
		assert.Equal(t, "2048x2048", volcReq.Size)

		// 出站 JSON 契约：content[].image_url.url 携带 data URI
		jsonData, err := common.Marshal(volcReq)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, common.Unmarshal(jsonData, &out))
		content, ok := out["content"].([]any)
		require.True(t, ok)
		first, ok := content[0].(map[string]any)
		require.True(t, ok)
		imageURL := first["image_url"].(map[string]any)
		assert.Equal(t, prefix, imageURL["url"].(string)[:len(prefix)])
	})

	t.Run("multiple image[] files keep order and count", func(t *testing.T) {
		c := newEditContext(t, map[string][][]byte{
			"image[]": {[]byte("one"), []byte("two")},
		}, map[string]string{"model": "doubao-seedream-4-0-250828", "prompt": "combine"})
		request := dto.ImageRequest{
			Model:  "doubao-seedream-4-0-250828",
			Prompt: "combine",
		}

		converted, err := convertEdit(c, request)
		require.NoError(t, err)
		volcReq, ok := converted.(*volcImageGenerationRequest)
		require.True(t, ok)
		require.Len(t, volcReq.Content, 2)

		want := [][]byte{[]byte("one"), []byte("two")}
		for i, item := range volcReq.Content {
			decoded, err := base64.StdEncoding.DecodeString(item.ImageURL.URL[len("data:image/png;base64,"):])
			require.NoError(t, err)
			assert.Equal(t, want[i], decoded, "content[%d] 应保持文件顺序", i)
		}
	})

	t.Run("text image URL field falls back to direct URL", func(t *testing.T) {
		c := newEditContext(t, nil, map[string]string{"model": "m", "prompt": "p"})
		request := dto.ImageRequest{
			Model:  "m",
			Prompt: "p",
			Image:  json.RawMessage(`"https://cdn.test/ref.png"`),
		}

		converted, err := convertEdit(c, request)
		require.NoError(t, err)
		volcReq, ok := converted.(*volcImageGenerationRequest)
		require.True(t, ok)
		require.Len(t, volcReq.Content, 1)
		assert.Equal(t, "https://cdn.test/ref.png", volcReq.Content[0].ImageURL.URL)
	})

	t.Run("missing image errors", func(t *testing.T) {
		c := newEditContext(t, nil, map[string]string{"model": "m", "prompt": "p"})
		request := dto.ImageRequest{Model: "m", Prompt: "p"}

		converted, err := convertEdit(c, request)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "image is required")
		assert.Nil(t, converted)
	})
}
