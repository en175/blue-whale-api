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

// TestConvertImageEditRequestMultipart 验证 IK9ZQM / IKA5R5：multipart 图生图参考图必须
// 转换为 ARK images/generations JSON 的顶层 image 字段（联调实测 content 数组不被 ARK 消费）。
// 单图保持已验证的字符串 wire 格式（image: "data:..."）；多图（image[]）输出 image 数组
// （Seedream 4.0/5.0 多参考图，prompt 以"图1/图2"指代）。
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
		var imageURI string
		require.NoError(t, common.Unmarshal(volcReq.Image, &imageURI), "single image should stay string wire format")
		assert.True(t, bytes.HasPrefix([]byte(imageURI), []byte(prefix)), "image should be data URI: %s", imageURI)
		decoded, err := base64.StdEncoding.DecodeString(imageURI[len(prefix):])
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

	t.Run("single image[] file keeps string wire format", func(t *testing.T) {
		c := newEditContext(t, map[string][][]byte{
			"image[]": {[]byte("only")},
		}, map[string]string{"model": "m", "prompt": "p"})

		converted, err := convertEdit(c, dto.ImageRequest{Model: "m", Prompt: "p"})
		require.NoError(t, err)
		volcReq, ok := converted.(*volcImageGenerationRequest)
		require.True(t, ok)

		var imageURI string
		require.NoError(t, common.Unmarshal(volcReq.Image, &imageURI), "single reference must stay string, not array")
		decoded, err := base64.StdEncoding.DecodeString(imageURI[len("data:image/png;base64,"):])
		require.NoError(t, err)
		assert.Equal(t, []byte("only"), decoded)
	})

	t.Run("multiple image[] files become image array (multi-reference)", func(t *testing.T) {
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

		// 多图 → 顶层 image 数组（ARK 多参考图协议），按序转 base64 data URI
		var images []string
		require.NoError(t, common.Unmarshal(volcReq.Image, &images))
		require.Len(t, images, 2)
		prefix := "data:image/png;base64,"
		for i, want := range []string{"one", "two"} {
			assert.True(t, bytes.HasPrefix([]byte(images[i]), []byte(prefix)), "image %d should be data URI: %s", i, images[i])
			decoded, err := base64.StdEncoding.DecodeString(images[i][len(prefix):])
			require.NoError(t, err)
			assert.Equal(t, []byte(want), decoded, "image %d content", i)
		}

		// 出站 JSON 契约：image 为数组，无 content
		jsonData, err := common.Marshal(volcReq)
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, common.Unmarshal(jsonData, &out))
		imgArr, ok := out["image"].([]any)
		require.True(t, ok, "image should be array: %s", jsonData)
		assert.Len(t, imgArr, 2)
		assert.NotContains(t, string(jsonData), `"content"`, "ARK 不消费 content 数组，不应出现在出站 JSON")
	})

	t.Run("more than max reference images rejected", func(t *testing.T) {
		imgs := make([][]byte, maxImageEditImages+1)
		for i := range imgs {
			imgs[i] = []byte("x")
		}
		c := newEditContext(t, map[string][][]byte{
			"image[]": imgs,
		}, map[string]string{"model": "m", "prompt": "p"})

		converted, err := convertEdit(c, dto.ImageRequest{Model: "m", Prompt: "p"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many reference images")
		assert.Nil(t, converted)
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

		var imageURL string
		require.NoError(t, common.Unmarshal(volcReq.Image, &imageURL))
		assert.Equal(t, "https://cdn.test/ref.png", imageURL)
	})

	t.Run("text image URL array becomes image array", func(t *testing.T) {
		c := newEditContext(t, nil, map[string]string{"model": "m", "prompt": "p"})
		request := dto.ImageRequest{
			Model:  "m",
			Prompt: "p",
			Image:  json.RawMessage(`["https://cdn.test/a.png","https://cdn.test/b.png"]`),
		}

		converted, err := convertEdit(c, request)
		require.NoError(t, err)
		volcReq, ok := converted.(*volcImageGenerationRequest)
		require.True(t, ok)

		var urls []string
		require.NoError(t, common.Unmarshal(volcReq.Image, &urls))
		assert.Equal(t, []string{"https://cdn.test/a.png", "https://cdn.test/b.png"}, urls)
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
