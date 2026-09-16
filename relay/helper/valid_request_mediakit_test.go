package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMediaKitValidate 走一遍 MediaKit 的请求校验入口，返回它给出的错误。
func runMediaKitValidate(t *testing.T, body string) error {
	t.Helper()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/mediakit/sync/generate-product-scene-image",
		strings.NewReader(body),
	)

	_, err := GetAndValidateRequest(c, types.RelayFormatMediaKit)
	return err
}

// TestMediaKitRejectsBatchCount 张数倍率进计费，必须在定价与预扣之前拦下。
// 上游真实字段是 batch_count（MediaKit 没有任何工具用 n），且官方对
// generate-product-scene-image 按生成张数计费、回包为 result.images[]：
// 放行 batch_count=4 就是上游出 4 张、网关只收 1 张的钱。
func TestMediaKitRejectsBatchCount(t *testing.T) {
	err := runMediaKitValidate(t, `{"image_url":"https://x/i.png","batch_count":4}`)
	require.Error(t, err)

	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode,
		"必填字段错必须是 400：controller.Relay 只在 err 已是 NewAPIError 时才保留该状态码")
	assert.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
	assert.Contains(t, apiErr.Error(), "batch_count")
}

// TestMediaKitAcceptsSingleBatchCount 单张是等价于「一次收费」的合法写法，
// 不能被守卫误伤。
func TestMediaKitAcceptsSingleBatchCount(t *testing.T) {
	require.NoError(t, runMediaKitValidate(t, `{"image_url":"https://x/i.png","batch_count":1}`))
	require.NoError(t, runMediaKitValidate(t, `{"image_url":"https://x/i.png"}`))
}
