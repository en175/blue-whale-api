package newapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Response structures
// ============================

// taskResultEnvelope 是 new-api 上游原生端点 GET /v1/video/generations/{id} 返回的信封：
// {code, message, data:{task_id, status, usage_completion_tokens, usage_total_tokens, result_url, ...}}。
// data 平铺 status 与 usage_*_tokens（非 doubao 的 usage:{...} 嵌套）。
type taskResultEnvelope struct {
	Code string `json:"code"`
	Msg  string `json:"message"`
	Data struct {
		TaskID                string  `json:"task_id"`
		Status                string  `json:"status"`
		UsageCompletionTokens flexInt `json:"usage_completion_tokens"`
		UsageTotalTokens      flexInt `json:"usage_total_tokens"`
		ResultURL             string  `json:"result_url"`
		FailReason            string  `json:"fail_reason"`
	} `json:"data"`
}

// submitResponse 兼容两种提交响应取 upstream task id：
//   - new-api 自家视频提交通常返回 OpenAIVideo 扁平形 {id, task_id, ...}；
//   - 也可能返回 {code, data:{task_id}} 信封。
//
// 只取字符串 task_id（task_xxx），用于后续原生端点轮询；忽略可能为数值的 DB 行 id。
type submitResponse struct {
	Code string `json:"code"`
	Data struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
	TaskID string `json:"task_id"`
}

// flexInt 兼容 JSON 中 int 或字符串形态的数值。上游历史上曾把 usage.completion_tokens
// 序列化成字符串（直接引爆 doubao converter 的 int 字段反序列化），此处对字符串/缺省/非法值
// 一律兜底为 0，避免单个字段类型不符导致整个信封解析失败（连状态一起丢）。
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	if b[0] == '"' {
		n, err := strconv.Atoi(strings.Trim(string(b), `"`))
		if err != nil {
			*f = 0
			return nil
		}
		*f = flexInt(n)
		return nil
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// ============================
// Adaptor implementation
// ============================

// TaskAdaptor 对接 new-api 系上游厂商（如 neovixai）的视频渠道。
// 与 Sora/OpenAI 渠道的核心差异：submit/fetch 均走原生 /v1/video/generations 端点
// （Sora 端点 /v1/videos 对 doubao/seedance 任务坏，且上游不可改），计费回 doubao 式 token。
type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction 接受 POST /v1/video/generations 的 JSON 请求。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// EstimateBilling 检测透传请求是否含视频输入与输出分辨率，返回档位价相对基准价
// 的计费 OtherRatio。本项目与上游同为 new-api 二开，入参契约一致（metadata.content
// 的 video_url 条目 + metadata.resolution），与豆包直连渠道同源同规则；
// 管理员通过 video_tier_pricing.price_table 为任意模型配置各档位价格。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	return taskcommon.EstimateVideoTierBilling(info, &req)
}

// BuildRequestURL submit 走原生端点（核心修复：不再打 /v1/videos）。
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/video/generations", a.baseURL), nil
}

// BuildRequestHeader 设置鉴权与 JSON 头。
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// BuildRequestBody 原样透传客户端 JSON（上游同为 new-api，请求格式一致），仅覆盖 model。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}

	var bodyMap map[string]interface{}
	if unmarshalErr := common.Unmarshal(cachedBody, &bodyMap); unmarshalErr == nil {
		bodyMap["model"] = info.UpstreamModelName
		if newBody, marshalErr := common.Marshal(bodyMap); marshalErr == nil {
			return bytes.NewReader(newBody), nil
		} else {
			// model 覆盖丢失会让上游收到客户端原始 model，可能导致 404/计费错配，必须可观测。
			common.SysError(fmt.Sprintf("newapi BuildRequestBody: marshal 失败 (%v)，回退原始 body，model 覆盖丢失", marshalErr))
		}
	} else {
		common.SysError(fmt.Sprintf("newapi BuildRequestBody: unmarshal 失败 (%v)，回退原始 body，model 覆盖丢失", unmarshalErr))
	}
	return bytes.NewReader(cachedBody), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 解析上游提交响应，提取 upstream task id 供轮询；向 blueapi 客户端回 OpenAIVideo。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var dResp submitResponse
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := dResp.TaskID
	if upstreamID == "" {
		upstreamID = dResp.Data.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return upstreamID, responseBody, nil
}

// FetchTask 走原生端点 GET /v1/video/generations/{id}（核心修复：不再打 /v1/videos）。
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/v1/video/generations/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult 解析原生端点轮询信封。
// 注意：service/task_polling.go 的 try-1 会把 {code:"success",data} 信封抢先解析进
// model.Task 并成功，从而跳过本方法（model.Task 无 usage_*_token / result_url 字段，
// 二者在 try-1 被丢弃：token 由 AdjustBillingOnComplete 从 task.Data 补救，
// result_url 由 ExtractTaskResultURL 钩子取回）。
// 本方法作为 try-1 失败时的兜底（如上游返回非信封错误体），并保持状态映射自洽。
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var env taskResultEnvelope
	if err := common.Unmarshal(respBody, &env); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := &relaycommon.TaskInfo{Code: 0}

	switch strings.ToUpper(strings.TrimSpace(env.Data.Status)) {
	case "", "NOT_START":
		taskResult.Status = string(model.TaskStatusNotStart)
	case "SUBMITTED":
		taskResult.Status = model.TaskStatusSubmitted
	case "QUEUED", "PENDING":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "IN_PROGRESS", "PROCESSING", "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "SUCCESS", "SUCCEEDED", "COMPLETED":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = env.Data.ResultURL
		taskResult.TotalTokens = int(env.Data.UsageTotalTokens)
		if taskResult.TotalTokens == 0 {
			taskResult.TotalTokens = int(env.Data.UsageCompletionTokens)
		}
	case "FAILURE", "FAILED", "CANCELLED", "CANCELED":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = env.Data.FailReason
		if taskResult.Reason == "" {
			taskResult.Reason = "task failed"
		}
	default:
		// 未识别状态留空，交由调用方空状态分支处理
	}

	return taskResult, nil
}

// ExtractTaskResultURL 实现 service.TaskResultURLExtractor：try-1 把信封解析进
// model.Task 时 result_url 被丢弃（model.Task 无此字段），此处从原始信封取回
// 上游真实结果 URL，使轮询 SUCCESS 分支存储上游直链（直链透传），而非回退
// 本网关代理地址。解析失败或无 result_url 时返回空串，交由现有分支处理。
func (a *TaskAdaptor) ExtractTaskResultURL(respBody []byte) string {
	var env taskResultEnvelope
	if err := common.Unmarshal(respBody, &env); err != nil {
		return ""
	}
	return env.Data.ResultURL
}

// AdjustBillingOnComplete 从轮询阶段已存入 task.Data 的完整 new-api 信封提取真实 token，
// 写入 taskResult.TotalTokens 并返回 0，交由 settleTaskBillingOnComplete 路径 #2
// （RecalculateTaskQuotaByTokens）按 tokens × modelRatio × groupRatio × otherMultiplier 重算，
// 与 doubao 直连渠道同公式同量纲。
//
// 必要性：try-1 把信封解析进 model.Task 时丢弃了 usage_*_tokens（model.Task 无此字段），
// 且跳过了 ParseTaskResult，故 token 计费只能在此从原始 task.Data 重新提取。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if taskResult.TotalTokens > 0 {
		return 0
	}
	var env taskResultEnvelope
	if err := common.Unmarshal(task.Data, &env); err != nil {
		return 0
	}
	tokens := int(env.Data.UsageTotalTokens)
	if tokens <= 0 {
		tokens = int(env.Data.UsageCompletionTokens)
	}
	if tokens > 0 {
		taskResult.TotalTokens = tokens
	}
	return 0
}

// ConvertToOpenAIVideo 把存储的任务转换为客户端 OpenAI 视频响应，从信封提取直链 mp4。
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var env taskResultEnvelope
	_ = common.Unmarshal(originTask.Data, &env)

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	if env.Data.ResultURL != "" {
		openAIVideo.SetMetadata("url", env.Data.ResultURL)
	}
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	return common.Marshal(openAIVideo)
}
