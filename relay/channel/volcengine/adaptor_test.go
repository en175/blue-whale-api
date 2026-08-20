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

// TestConvertImageEditRequestMultipart 验证 IK9ZQM：multipart 图生图参考图必须转换为
// ARK images/generations JSON 的顶层 image 字段 base64 data URI（联调实测 content 数组
// 不被 ARK 消费），而不是像修复前那样在出站 JSON 中丢失。
func TestConvertImageEditRequestMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("single image file becomes top-level image data URI", func(t *testing.T) {
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

		prefix := "data:image/png;base64,"
		assert.True(t, bytes.HasPrefix([]byte(volcReq.Image), []byte(prefix)), "image should be data URI: %s", volcReq.Image)
		decoded, err := base64.StdEncoding.DecodeString(volcReq.Image[len(prefix):])
		require.NoError(t, err)
		assert.Equal(t, []byte("fake image"), decoded)

		// 字段保留：model / prompt / size
		assert.Equal(t, "doubao-seedream-4-0-250828", volcReq.Model)
		assert.Equal(t, "edit this", volcReq.Prompt)
		assert.Equal(t, "2048x2048", volcReq.Size)

		// 出站 JSON 契约：顶层 image 字段携带 data URI，且无 content 数组
		jsonData, err := common.Marshal(volcReq)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, common.Unmarshal(jsonData, &out))
		assert.Equal(t, prefix, out["image"].(string)[:len(prefix)])
		assert.NotContains(t, string(jsonData), `"content"`, "ARK 不消费 content 数组，不应出现在出站 JSON")
	})

	t.Run("multiple image[] files keep first image only", func(t *testing.T) {
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

		decoded, err := base64.StdEncoding.DecodeString(volcReq.Image[len("data:image/png;base64,"):])
		require.NoError(t, err)
		assert.Equal(t, []byte("one"), decoded, "Seedream 仅支持单张参考图，应取第一张")
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
		assert.Equal(t, "https://cdn.test/ref.png", volcReq.Image)
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
