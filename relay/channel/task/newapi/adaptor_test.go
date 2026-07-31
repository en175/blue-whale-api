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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// successEnvelope 取自实测 task_rzOL 原生端点响应（脱敏 result_url）：
// 108900 × modelRatio(37.5) = 4,083,750 = 上游 quota，token 计费前提自洽。
const successEnvelope = `{"code":"success","message":"","data":{"task_id":"task_rzOLuUKw2SHCTfg9egpisd9UKcEYWPmo","status":"SUCCESS","usage_completion_tokens":108900,"usage_total_tokens":108900,"usage_reported":true,"result_url":"https://example.volces.com/seedance.mp4","quota":4083750}}`

// 注意：生产 happy path 下 task_polling.go 的 try-1 会把 {code:"success",data} 信封
// 抢先解析进 model.Task 并跳过 ParseTaskResult，故真实 token 提取走 AdjustBillingOnComplete
// （见下方 TestAdjustBillingOnComplete_*）。以下 ParseTaskResult 用例覆盖的是 try-1 失败
// （如上游返回非信封错误体）时的兜底路径，保持状态映射与 token 提取自洽。

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
