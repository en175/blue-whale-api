package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/task/tokenhubvolc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runArkConvert 走一遍请求转换中间件，返回响应与下游处理函数实际读到的内容。
func runArkConvert(t *testing.T, body string) (*httptest.ResponseRecorder, *tokenhubvolc.NativeRequest, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var captured *tokenhubvolc.NativeRequest
	var internal map[string]any

	router := gin.New()
	router.POST(constant.NativeArkVideoTaskPath, ArkNativeRequestConvert(), func(c *gin.Context) {
		captured, _ = tokenhubvolc.NativeRequestFrom(c)
		// 用与业务代码相同的读取方式，验证改写后的 body 真的替换了缓存
		require.NoError(t, common.UnmarshalBodyReusable(c, &internal))
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, constant.NativeArkVideoTaskPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w, captured, internal
}

func TestArkNativeRequestConvert_rewritesInternalBody(t *testing.T) {
	w, native, internal := runArkConvert(t, `{
		"model":"doubao-seedance-2-5-260628",
		"content":[{"type":"text","text":"a cat"},{"type":"image_url","image_url":{"url":"asset://x"},"role":"first_frame"}],
		"resolution":"480p",
		"draft":true
	}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, native)
	assert.Equal(t, "a cat", native.Prompt())
	assert.True(t, native.IsDraft())
	assert.Empty(t, native.DraftTaskID())

	assert.Equal(t, "doubao-seedance-2-5-260628", internal["model"])
	assert.Equal(t, "a cat", internal["prompt"])
	metadata, ok := internal["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "480p", metadata["resolution"])
	assert.Len(t, metadata["content"], 2)
}

func TestArkNativeRequestConvert_draftTaskBecomesOriginTask(t *testing.T) {
	w, native, _ := runArkConvert(t, `{
		"model":"doubao-seedance-2-5-260628",
		"content":[{"type":"draft_task","draft_task":{"id":"task_abc"}}]
	}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, native)
	assert.True(t, native.HasDraftTask())
	assert.Equal(t, "task_abc", native.DraftTaskID())
}

func TestArkNativeRequestConvert_rejectsBadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "缺少 model", body: `{"content":[{"type":"text","text":"x"}]}`, code: "InvalidParameter"},
		{name: "draft_task 缺 id", body: `{"model":"m","content":[{"type":"draft_task","draft_task":{}}]}`, code: "InvalidParameter"},
		{name: "非法 JSON", body: `{"model":`, code: "InvalidParameter"},
		{name: "顶层不是对象", body: `[]`, code: "InvalidParameter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, _, _ := runArkConvert(t, tt.body)

			require.Equal(t, http.StatusBadRequest, w.Code)
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
					Param   string `json:"param"`
					Type    string `json:"type"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &envelope))
			assert.Equal(t, tt.code, envelope.Error.Code)
			assert.Equal(t, "BadRequest", envelope.Error.Type)
			assert.NotEmpty(t, envelope.Error.Message)
		})
	}
}

func TestWriteArkError_mapsStatusToType(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "BadRequest"},
		{http.StatusNotFound, "NotFound"},
		{http.StatusConflict, "Conflict"},
		{http.StatusTooManyRequests, "TooManyRequests"},
		{http.StatusServiceUnavailable, "InternalServerError"},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		WriteArkError(c, tt.status, "SomeCode", "boom")

		var envelope struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &envelope))
		assert.Equal(t, tt.status, w.Code)
		assert.Equal(t, "SomeCode", envelope.Error.Code)
		assert.Equal(t, tt.want, envelope.Error.Type)
	}
}
