package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relay/channel/mediakit"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// distributeMediaKitSync 直接以同步路由的 path 与 :tool 参数调用 getModelRequest。
// 不经 gin 路由匹配，因为路由本就会把带 / 的路径挡在外面（返回 404），那样测到的
// 是 gin 而不是本分支的校验逻辑；这里要验的是「无论 tool 取什么值都不拼 URL」。
func distributeMediaKitSync(t *testing.T, tool string) (*ModelRequest, bool, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/mediakit/sync/"+tool, strings.NewReader(`{"image_url":"https://x/a.png"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "tool", Value: tool}}

	return getModelRequest(c)
}

// TestDistributeMediaKitSyncResolvesToolModel 保护对客模型名契约：MediaKit 原生
// 请求体里没有 model 字段，模型只能由路径上的工具名查表得出。若这个分支被放到
// 「读 JSON body 取 model」的兜底分支之后，模型会是空串，请求随后以莫名其妙的
// 「无可用渠道」失败。
func TestDistributeMediaKitSyncResolvesToolModel(t *testing.T) {
	modelRequest, shouldSelect, err := distributeMediaKitSync(t, "enhance-image")
	require.NoError(t, err)
	require.NotNil(t, modelRequest)
	assert.Equal(t, mediakit.ModelNameForTool("enhance-image"), modelRequest.Model)
	assert.True(t, shouldSelect, "同步工具需要走渠道选择")
}

// TestDistributeMediaKitSyncRejectsUnknownTool 是安全边界：客户端传的工具名只用于
// 查表，未登记的工具必须在分销阶段就以 400 拒绝，绝不能带着它继续走到上游拼接 URL。
func TestDistributeMediaKitSyncRejectsUnknownTool(t *testing.T) {
	for _, tool := range []string{"not-a-tool", "../../etc/passwd", "enhance-image/../.."} {
		t.Run(tool, func(t *testing.T) {
			modelRequest, _, err := distributeMediaKitSync(t, tool)
			require.Error(t, err, "未登记的工具必须报错")
			assert.Contains(t, err.Error(), "unknown mediakit tool")
			assert.Nil(t, modelRequest)
		})
	}
}

// TestDistributeMediaKitSyncRejectsAsyncTool 保护同步/异步路由的分离：异步工具
// 走同步路由时必须在分销阶段拒绝。否则要等到 Adaptor.GetRequestURL 才失败，
// 客户端的用法错误会表现为 500。
func TestDistributeMediaKitSyncRejectsAsyncTool(t *testing.T) {
	modelRequest, _, err := distributeMediaKitSync(t, "enhance-video-generative")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is asynchronous")
	assert.Nil(t, modelRequest)
}

// TestPath2RelayModeMediaKit 固定中继模式解析：genBaseRelayInfo 只在 Path2RelayMode
// 返回 Unknown 时才回退读 context，解析错会让请求走错 handler。三个前缀有包含关系，
// 顺序错了（比如把通用前缀提到最前）会把同步工具和查询都吞成异步。
func TestPath2RelayModeMediaKit(t *testing.T) {
	assert.Equal(t, relayconstant.RelayModeMediaKitSync, relayconstant.Path2RelayMode("/v1/mediakit/sync/enhance-image"))
	assert.Equal(t, relayconstant.RelayModeMediaKitAsync, relayconstant.Path2RelayMode("/v1/mediakit/enhance-video-generative"))
	assert.Equal(t, relayconstant.RelayModeMediaKitFetch, relayconstant.Path2RelayMode("/v1/mediakit/tasks/task_abc"))
	assert.Equal(t, relayconstant.RelayModeUnknown, relayconstant.Path2RelayMode("/v1/whatever"))
}

// distributeMediaKitAsync 以异步路由的 path 与 :tool 参数调用 getModelRequest。
func distributeMediaKitAsync(t *testing.T, tool string) (*ModelRequest, bool, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/mediakit/"+tool, strings.NewReader(`{"video_url":"https://x/a.mp4"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "tool", Value: tool}}

	return getModelRequest(c)
}

// TestDistributeMediaKitAsyncResolvesToolModel 与同步分支同理：模型只能由路径上的
// 工具名查表得出，落到 JSON body 兜底分支会拿到空模型名。
func TestDistributeMediaKitAsyncResolvesToolModel(t *testing.T) {
	modelRequest, shouldSelect, err := distributeMediaKitAsync(t, "enhance-video-generative")
	require.NoError(t, err)
	require.NotNil(t, modelRequest)
	assert.Equal(t, mediakit.ModelNameForTool("enhance-video-generative"), modelRequest.Model)
	assert.True(t, shouldSelect, "异步提交需要走渠道选择")
}

// TestDistributeMediaKitAsyncRejectsTool 是同一个安全边界：工具名只用于查表，
// 未登记的拒绝；同步工具走异步路由同样必须在分销阶段挡下，否则会一路走到
// TaskAdaptor.ValidateRequestAndSetAction 才报错。
func TestDistributeMediaKitAsyncRejectsTool(t *testing.T) {
	t.Run("未登记工具", func(t *testing.T) {
		modelRequest, _, err := distributeMediaKitAsync(t, "not-a-tool")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown mediakit tool")
		assert.Nil(t, modelRequest)
	})

	t.Run("同步工具", func(t *testing.T) {
		modelRequest, _, err := distributeMediaKitAsync(t, "enhance-image")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is synchronous")
		assert.Nil(t, modelRequest)
	})
}

// TestDistributeMediaKitFetchSkipsChannelSelection 查询路径用的是任务记录里的渠道，
// 重新选渠道会让查询落到另一条渠道上（甚至另一家供应商），任务永远查不到。
func TestDistributeMediaKitFetchSkipsChannelSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/mediakit/tasks/task_abc", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_abc"}}

	_, shouldSelect, err := getModelRequest(c)
	require.NoError(t, err)
	assert.False(t, shouldSelect, "查询不应重新选择渠道")
	assert.Equal(t, relayconstant.RelayModeMediaKitFetch, c.GetInt("relay_mode"))
}
