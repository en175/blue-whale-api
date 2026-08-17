package newapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// successEnvelope 取自实测 task_rzOL 原生端点响应（脱敏 result_url）：
// 108900 × modelRatio(37.5) = 4,083,750 = 上游 quota，token 计费前提自洽。
const successEnvelope = `{"code":"success","message":"","data":{"task_id":"task_rzOLuUKw2SHCTfg9egpisd9UKcEYWPmo","status":"SUCCESS","usage_completion_tokens":108900,"usage_total_tokens":108900,"usage_reported":true,"result_url":"https://example.volces.com/seedance.mp4","quota":4083750}}`

// 注意：生产 happy path 下 task_polling.go 的 try-1 会把 {code:"success",data} 信封
// 抢先解析进 model.Task 并跳过 ParseTaskResult。try-1 丢弃 model.Task 没有的字段：
// token 走 AdjustBillingOnComplete 从 task.Data 补救（见下方 TestAdjustBillingOnComplete_*），
// result_url 走 ExtractTaskResultURL 钩子取回（见下方 TestExtractTaskResultURL_*），
// 实现直链透传。以下 ParseTaskResult 用例覆盖的是 try-1 失败（如上游返回非信封错误体）
// 时的兜底路径，保持状态映射与 token 提取自洽。

func TestParseTaskResult_Success(t *testing.T) {
	a := &TaskAdaptor{}
	r, err := a.ParseTaskResult([]byte(successEnvelope))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), r.Status)
	assert.Equal(t, 108900, r.TotalTokens)
	assert.Equal(t, "https://example.volces.com/seedance.mp4", r.Url)
}

func TestParseTaskResult_InProgress(t *testing.T) {
	a := &TaskAdaptor{}
	r, err := a.ParseTaskResult([]byte(`{"code":"success","data":{"status":"IN_PROGRESS"}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusInProgress), r.Status)
	assert.Equal(t, 0, r.TotalTokens)
}

func TestParseTaskResult_Failure(t *testing.T) {
	a := &TaskAdaptor{}
	r, err := a.ParseTaskResult([]byte(`{"code":"failed","data":{"status":"FAILURE","fail_reason":"seedance render error"}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusFailure), r.Status)
	assert.Equal(t, "seedance render error", r.Reason)
}

// TestParseTaskResult_StringTokens 防御：上游历史上曾把 token 序列化成字符串（引爆 doubao converter）。
// flexInt 必须吃下字符串形态，且不影响状态解析。
func TestParseTaskResult_StringTokens(t *testing.T) {
	a := &TaskAdaptor{}
	body := []byte(`{"code":"success","data":{"status":"SUCCESS","usage_total_tokens":"108900","usage_completion_tokens":"108900","result_url":"https://x.mp4"}}`)
	r, err := a.ParseTaskResult(body)
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), r.Status, "字符串 token 不应影响状态")
	assert.Equal(t, 108900, r.TotalTokens, "应从字符串解析出 token")
}

// TestFlexInt_Malformed 防御：null/空串/非数字/小数等畸形值不得炸掉整个信封解析，统一兜底 0。
func TestFlexInt_Malformed(t *testing.T) {
	for _, raw := range []string{`null`, `""`, `"abc"`, `"12.5"`} {
		var f flexInt
		require.NoError(t, common.Unmarshal([]byte(raw), &f), "raw=%q 不应返回 error", raw)
		assert.Equal(t, flexInt(0), f, "raw=%q 应兜底为 0", raw)
	}
}

// TestAdjustBillingOnComplete_ExtractsTokens 核心计费衔接（生产 happy path 承重）：
// try-1 跳过 ParseTaskResult、model.Task 丢 token，故 AdjustBillingOnComplete 必须从原始
// task.Data 重新提取 token 写入 taskResult.TotalTokens 并返回 0，交由 RecalculateTaskQuotaByTokens
// 按 tokens×modelRatio×groupRatio 重算。
func TestAdjustBillingOnComplete_ExtractsTokens(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{Data: []byte(successEnvelope)}
	taskResult := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}
	assert.Equal(t, 0, a.AdjustBillingOnComplete(task, taskResult), "返回 0 以交由 RecalculateTaskQuotaByTokens")
	assert.Equal(t, 108900, taskResult.TotalTokens)
}

func TestAdjustBillingOnComplete_PreservesExistingTokens(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{Data: []byte(successEnvelope)}
	taskResult := &relaycommon.TaskInfo{TotalTokens: 555}
	a.AdjustBillingOnComplete(task, taskResult)
	assert.Equal(t, 555, taskResult.TotalTokens, "已有 token 不应被覆盖")
}

// TestAdjustBillingOnComplete_NoTokenKeepsPrecharge 缺 token 时返回 0 且不写 TotalTokens，
// 保持预扣额度（不触发误重算）。
func TestAdjustBillingOnComplete_NoTokenKeepsPrecharge(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{Data: []byte(`{"code":"success","data":{"status":"SUCCESS"}}`)}
	taskResult := &relaycommon.TaskInfo{}
	assert.Equal(t, 0, a.AdjustBillingOnComplete(task, taskResult))
	assert.Equal(t, 0, taskResult.TotalTokens, "无 token 不应触发重算")
}

// nestedEnvelope 取自实测 task_JQABEhn（脱敏 result_url）：上游 new-api 二开再接 doubao，
// 把下游任务数据再套一层，token 藏在 data.data.usage.total_tokens（非平铺 usage_total_tokens）。
// 平铺结构取不到 token 会导致 AdjustBillingOnComplete 返回 0、差额结算被跳过（不退款）。
const nestedEnvelope = `{"code":"success","message":"","data":{"task_id":"task_nest","status":"SUCCESS","result_url":"https://example.volces.com/seedance.mp4","data":{"status":"succeeded","usage":{"total_tokens":245025,"completion_tokens":245025}}}}`

// TestAdjustBillingOnComplete_NestedTokens 生产承重：嵌套信封下 token 必须从内层
// data.data.usage.total_tokens 提取，否则按量计费模型跳过重算、预扣额不退。
func TestAdjustBillingOnComplete_NestedTokens(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{Data: []byte(nestedEnvelope)}
	taskResult := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}
	assert.Equal(t, 0, a.AdjustBillingOnComplete(task, taskResult), "返回 0 以交由 RecalculateTaskQuotaByTokens")
	assert.Equal(t, 245025, taskResult.TotalTokens)
}

// TestParseTaskResult_NestedTokens 兜底路径同样兼容嵌套 token（try-1 失败时走此方法）。
func TestParseTaskResult_NestedTokens(t *testing.T) {
	a := &TaskAdaptor{}
	r, err := a.ParseTaskResult([]byte(nestedEnvelope))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), r.Status)
	assert.Equal(t, 245025, r.TotalTokens)
}

// TestExtractTaskResultURL_Success 生产 happy path 承重：try-1 丢弃 result_url 后，
// 轮询经此钩子取回上游直链，实现直链透传（不再回退本网关代理地址）。
func TestExtractTaskResultURL_Success(t *testing.T) {
	a := &TaskAdaptor{}
	assert.Equal(t, "https://example.volces.com/seedance.mp4", a.ExtractTaskResultURL([]byte(successEnvelope)))
}

// TestExtractTaskResultURL_NoURL 信封无 result_url 时返回空串，交由现有分支回退代理地址。
func TestExtractTaskResultURL_NoURL(t *testing.T) {
	a := &TaskAdaptor{}
	assert.Equal(t, "", a.ExtractTaskResultURL([]byte(`{"code":"success","data":{"status":"SUCCESS"}}`)))
}

// TestExtractTaskResultURL_Malformed 畸形响应不得 panic，返回空串走兜底。
func TestExtractTaskResultURL_Malformed(t *testing.T) {
	a := &TaskAdaptor{}
	assert.Equal(t, "", a.ExtractTaskResultURL([]byte(`not json`)))
}

// resetVideoTierConfigForEstimate 恢复 video_tier_pricing 为空表，防配置在用例间泄漏。
func resetVideoTierConfigForEstimate(t *testing.T) {
	t.Helper()
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": "{}",
	}))
	t.Cleanup(func() {
		_ = config.GlobalConfig.LoadFromDB(map[string]string{
			"video_tier_pricing.price_table": "{}",
		})
	})
}

// newEstimateCtx 构造带 task_request 的 gin 测试上下文（EstimateBilling 经
// GetTaskRequest 读取，与 ValidateBasicTaskRequest 的存储方式一致）。
func newEstimateCtx(metadata map[string]interface{}) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("task_request", relaycommon.TaskSubmitReq{
		Prompt:   "a cat",
		Metadata: metadata,
	})
	return c
}

// newEstimateInfo 构造带对客名/上游名的 RelayInfo（UpstreamModelName 在嵌入的
// *ChannelMeta 上，必须显式初始化，否则空指针解引用）。
func newEstimateInfo(originName, upstreamName string) *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{OriginModelName: originName}
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: upstreamName}
	return info
}

func videoInputMetadata(resolution string) map[string]interface{} {
	return map[string]interface{}{
		"resolution": resolution,
		"content": []interface{}{
			map[string]interface{}{
				"type":      "video_url",
				"video_url": map[string]interface{}{"url": "https://example.com/input.mp4"},
			},
		},
	}
}

// TestEstimateBilling_BuiltinModelWithVideo 内置豆包模型 + 含视频输入：
// 命中内置价格表，返回 video_input 倍率（28/46）。
func TestEstimateBilling_BuiltinModelWithVideo(t *testing.T) {
	resetVideoTierConfigForEstimate(t)
	a := &TaskAdaptor{}
	ratios := a.EstimateBilling(newEstimateCtx(videoInputMetadata("720p")), newEstimateInfo("doubao-seedance-2-0-260128", "doubao-seedance-2-0-260128"))
	require.NotNil(t, ratios)
	assert.InDelta(t, 28.0/46.0, ratios["video_input"], 1e-9)
}

// TestEstimateBilling_MappedModelUsesUpstreamName 渠道做了模型映射（对客名不在
// 价格表里）时，按上游模型名命中价格表。
func TestEstimateBilling_MappedModelUsesUpstreamName(t *testing.T) {
	resetVideoTierConfigForEstimate(t)
	a := &TaskAdaptor{}
	ratios := a.EstimateBilling(newEstimateCtx(videoInputMetadata("1080p")), newEstimateInfo("seedance-2.0", "doubao-seedance-2-0-260128"))
	require.NotNil(t, ratios)
	assert.InDelta(t, 31.0/46.0, ratios["video_input"], 1e-9)
}

// TestEstimateBilling_ConfiguredCustomModel 管理员为任意模型配置的档位价格生效
// （newapi 渠道对接非内置豆包模型的核心场景）。
func TestEstimateBilling_ConfiguredCustomModel(t *testing.T) {
	resetVideoTierConfigForEstimate(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"video_tier_pricing.price_table": `{"my-seedance": {"base": 50, "video": 25, "1080p": 75}}`,
	}))
	a := &TaskAdaptor{}

	ratios := a.EstimateBilling(newEstimateCtx(videoInputMetadata("720p")), newEstimateInfo("my-seedance", "my-seedance"))
	require.NotNil(t, ratios)
	assert.InDelta(t, 0.5, ratios["video_input"], 1e-9)

	ratios = a.EstimateBilling(newEstimateCtx(map[string]interface{}{"resolution": "1080p"}), newEstimateInfo("my-seedance", "my-seedance"))
	require.NotNil(t, ratios)
	assert.InDelta(t, 1.5, ratios["video_input"], 1e-9)
}

// TestEstimateBilling_NoTableReturnsNil 无任何价格表时不加倍率。
func TestEstimateBilling_NoTableReturnsNil(t *testing.T) {
	resetVideoTierConfigForEstimate(t)
	a := &TaskAdaptor{}
	assert.Nil(t, a.EstimateBilling(newEstimateCtx(videoInputMetadata("720p")), newEstimateInfo("unknown-model", "unknown-model")))
}

// TestEstimateBilling_BaseTierReturnsNil 基准档（不含视频输入的基础分辨率）倍率为 1，
// 不产生多余的 OtherRatio。
func TestEstimateBilling_BaseTierReturnsNil(t *testing.T) {
	resetVideoTierConfigForEstimate(t)
	a := &TaskAdaptor{}
	assert.Nil(t, a.EstimateBilling(newEstimateCtx(map[string]interface{}{"resolution": "720p"}), newEstimateInfo("doubao-seedance-2-0-260128", "doubao-seedance-2-0-260128")))
}

// newSubmitCtx 构造一个可写响应的 gin 测试上下文（DoResponse 会 c.JSON）。
func newSubmitCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
	return c
}

// newSubmitInfo 构造带 PublicTaskID 的 RelayInfo（PublicTaskID 在嵌入的 *TaskRelayInfo 上，
// 裸 &RelayInfo{} 里该指针为 nil，必须初始化）。
func newSubmitInfo() *relaycommon.RelayInfo {
	info := &relaycommon.RelayInfo{OriginModelName: "seedance-2.0"}
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{PublicTaskID: "task_pub"}
	return info
}

// TestDoResponse_FlatTaskID 上游 new-api 提交通常返回 OpenAIVideo 扁平形 {task_id}。
func TestDoResponse_FlatTaskID(t *testing.T) {
	a := &TaskAdaptor{}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"task_id":"task_abc123","id":"task_abc123"}`))}
	info := newSubmitInfo()
	taskID, _, taskErr := a.DoResponse(newSubmitCtx(), resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "task_abc123", taskID)
}

// TestDoResponse_EnvelopeTaskID 兼容 {code,data:{task_id}} 信封形态。
func TestDoResponse_EnvelopeTaskID(t *testing.T) {
	a := &TaskAdaptor{}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":"success","data":{"task_id":"task_xyz"}}`))}
	info := newSubmitInfo()
	taskID, _, taskErr := a.DoResponse(newSubmitCtx(), resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "task_xyz", taskID)
}

// TestDoResponse_EmptyTaskIDError 两种形态都取不到 task_id 时必须明确报错（而非静默返回空）。
func TestDoResponse_EmptyTaskIDError(t *testing.T) {
	a := &TaskAdaptor{}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":"success","data":{}}`))}
	info := newSubmitInfo()
	_, _, taskErr := a.DoResponse(newSubmitCtx(), resp, info)
	require.NotNil(t, taskErr, "空 task_id 必须返回错误")
}
