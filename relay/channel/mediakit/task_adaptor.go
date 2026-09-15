package mediakit

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// submitResponse 是异步工具的提交响应信封。task_id 是上游任务 ID，
// 只落库到 private_data，对客返回的是网关公开任务 ID。
type submitResponse struct {
	Success bool   `json:"success"`
	TaskID  string `json:"task_id"`
	Message string `json:"message"`
}

// queryResponse 是异步任务的查询响应。result 的键名因工具而异
// （image_url / video_url / audio_url ...），故按 map 解析后按产物类型取。
type queryResponse struct {
	Success bool           `json:"success"`
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Result  map[string]any `json:"result"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {}

// ValidateRequestAndSetAction 校验工具名与请求体。不复用
// relaycommon.ValidateBasicTaskRequest——它要求 prompt 非空，而 MediaKit
// 原生协议没有 prompt 字段（入参是 video_url / image_url 之类）。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	spec, ok := ToolForModel(info.OriginModelName)
	if !ok {
		return service.TaskErrorWrapper(fmt.Errorf("unknown mediakit tool: %s", info.OriginModelName), "unknown_tool", http.StatusBadRequest)
	}
	if spec.Sync {
		return service.TaskErrorWrapper(fmt.Errorf("mediakit tool %s is synchronous, use POST /v1/mediakit/sync/%s", spec.Name, spec.Name), "invalid_tool", http.StatusBadRequest)
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_request_body_failed", http.StatusBadRequest)
	}
	raw, err := storage.Bytes()
	if err != nil {
		return service.TaskErrorWrapper(err, "read_request_body_failed", http.StatusBadRequest)
	}
	var body map[string]any
	if err := common.Unmarshal(raw, &body); err != nil {
		return service.TaskErrorWrapper(fmt.Errorf("request body must be a JSON object: %w", err), "invalid_request_body", http.StatusBadRequest)
	}

	info.Action = spec.Name
	return nil
}

// EstimateBilling 时长类工具按预估分钟预扣。MediaKit 多数工具的入参是 URL，
// 提交时拿不到真实时长，故用渠道配置的默认预估分钟；真实时长在任务完成时
// 由 AdjustBillingOnComplete 结算差额。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	spec, ok := ToolForModel(info.OriginModelName)
	if !ok || spec.Mode != BillingDuration {
		return nil
	}
	minutes := info.ChannelSetting.MediaKitDefaultMinutes
	if minutes <= 0 {
		return nil
	}
	return map[string]float64{"minutes": minutes}
}

// BuildRequestURL 由注册表给出上游路径，客户端工具名不参与拼接。
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	spec, ok := ToolForModel(info.UpstreamModelName)
	if !ok {
		return "", fmt.Errorf("unknown mediakit tool: %s", info.UpstreamModelName)
	}
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return strings.TrimSuffix(baseURL, "/") + spec.Path, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// BuildRequestBody 完全透传：MediaKit 请求体里没有 model 字段，网关一个字都不改写。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body storage: %w", err)
	}
	return common.ReaderOnly(storage), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 上游以 HTTP 200 + success=false 表达业务失败，必须在此拦下：
// relay_task.go 只拦非 200 响应码。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", nil, service.TaskErrorWrapper(readErr, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var submit submitResponse
	if err := common.Unmarshal(responseBody, &submit); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("unmarshal_response_body_failed: %w", err), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if !submit.Success || submit.TaskID == "" {
		message := submit.Message
		if message == "" {
			message = "submit failed"
		}
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("mediakit submit failed: %s", message), "invalid_response", http.StatusBadGateway)
	}

	// 对客沿用 MediaKit 信封，但只暴露网关公开任务 ID。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"task_id": info.PublicTaskID,
		"message": "",
	})
	return submit.TaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	uri := strings.TrimSuffix(baseUrl, "/") + queryPathPrefix + taskID

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var res queryResponse
	if err := common.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("unmarshal task result failed: %w", err)
	}

	taskResult := relaycommon.TaskInfo{Code: 0}
	switch strings.ToLower(res.Status) {
	case "pending", "queued", "submitted":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "completed", "succeeded", "success":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = ResultURL(res.Result)
	case "failed", "error", "canceled", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = res.Message
	default:
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}
	return &taskResult, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return GetModelList()
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}
