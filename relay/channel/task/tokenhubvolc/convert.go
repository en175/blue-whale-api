package tokenhubvolc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// arkTaskStatus 把网关任务状态翻译成火山原生视频任务的状态词。客户端按官方
// 文档判断 queued / running / succeeded / failed，不能直接看到内部枚举。
var arkTaskStatus = map[model.TaskStatus]string{
	model.TaskStatusNotStart:   "queued",
	model.TaskStatusSubmitted:  "queued",
	model.TaskStatusQueued:     "queued",
	model.TaskStatusInProgress: "running",
	model.TaskStatusSuccess:    "succeeded",
	model.TaskStatusFailure:    "failed",
}

// nativeTaskResponse 是查询接口回给下游的火山形状。拿不到值的字段一律省略，
// 不返回零值占位。
type nativeTaskResponse struct {
	ID     string `json:"id"`
	Model  string `json:"model,omitempty"`
	Status string `json:"status"`
	// UpstreamTaskID 是华鲲透出的火山任务号（cgt-...）。华鲲的创建与查询响应
	// 都带这个字段，网关此前只留了华鲲 UUID 做轮询键，下游拿不到 cgt 号，
	// 无法直接到火山侧排障。
	UpstreamTaskID string            `json:"upstreamTaskId,omitempty"`
	Content        *nativeTaskResult `json:"content,omitempty"`
	Usage          *nativeTaskUsage  `json:"usage,omitempty"`
	// Tools 是模型本次实际调用的工具（如 web_search）。上游只在确实调用过时
	// 才返回，因此按原样透出，未返回时不出现。
	Tools      []nativeTool     `json:"tools,omitempty"`
	CreatedAt  int64            `json:"created_at"`
	UpdatedAt  int64            `json:"updated_at"`
	Resolution string           `json:"resolution,omitempty"`
	Duration   *int             `json:"duration,omitempty"`
	Error      *nativeTaskError `json:"error,omitempty"`
}

type nativeTaskUsage struct {
	CompletionTokens int              `json:"completion_tokens"`
	TotalTokens      int              `json:"total_tokens"`
	ToolUsage        *nativeToolUsage `json:"tool_usage,omitempty"`
}

type nativeToolUsage struct {
	WebSearch int `json:"web_search"`
}

type nativeTool struct {
	Type string `json:"type"`
}

type nativeTaskResult struct {
	VideoURL string `json:"video_url,omitempty"`
	// LastFrameURL 是上游在 return_last_frame=true 时签发的尾帧图片地址（TOS
	// 预签名，24 小时过期）；未请求或上游未返回时省略。
	LastFrameURL string `json:"last_frame_url,omitempty"`
}

type nativeTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// upstreamTaskData 是华鲲查询响应中网关需要的部分：结果包在 resultSummary 里
// （与 /api/v3 的顶层 content 不同）。错误对象在两层都可能出现：任务级的顶层
// error，以及结果信封内的 resultSummary.error。
type upstreamTaskData struct {
	// UpstreamTaskID 在创建响应与查询响应里都是顶层字段，不在 resultSummary 内。
	UpstreamTaskID string `json:"upstreamTaskId"`
	// Tools 是任务实际调用过的工具，未调用时上游不返回该字段。辅助信息按原始
	// 片段接收：形态漂移时只丢工具列表，不让整条查询响应 500。
	Tools         json.RawMessage `json:"tools"`
	ResultSummary struct {
		Content struct {
			VideoURL     string `json:"video_url"`
			LastFrameURL string `json:"last_frame_url"`
		} `json:"content"`
		Resolution string `json:"resolution"`
		Duration   string `json:"duration"`
		Usage      struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
			// ToolUsage 同上：辅助计费信息，解析失败只省略该字段。
			ToolUsage json.RawMessage `json:"tool_usage"`
		} `json:"usage"`
		Error *struct {
			Code    codeValue `json:"code"`
			Message string    `json:"message"`
		} `json:"error"`
	} `json:"resultSummary"`
	Error *struct {
		Code    codeValue `json:"code"`
		Message string    `json:"message"`
	} `json:"error"`
}

// ConvertToNativeArkTask 把落库任务转成火山原生视频对象。
//
// 状态、时间戳、模型名来自任务表；video_url / usage / resolution / duration 来自
// 上游查询结果（task.Data）。失败原因优先用网关已归一化的 FailReason。
func (a *TaskAdaptor) ConvertToNativeArkTask(originTask *model.Task) ([]byte, error) {
	var data upstreamTaskData
	if len(originTask.Data) > 0 {
		if err := common.Unmarshal(originTask.Data, &data); err != nil {
			return nil, fmt.Errorf("unmarshal tokenhub task data failed: %w", err)
		}
	}

	status := arkTaskStatus[originTask.Status]
	if status == "" {
		status = "running"
	}

	response := nativeTaskResponse{
		ID:             originTask.TaskID,
		Model:          originTask.Properties.OriginModelName,
		Status:         status,
		CreatedAt:      originTask.CreatedAt,
		UpdatedAt:      originTask.UpdatedAt,
		UpstreamTaskID: data.UpstreamTaskID,
	}

	if originTask.Status == model.TaskStatusSuccess {
		if content := data.ResultSummary.Content; content.VideoURL != "" || content.LastFrameURL != "" {
			response.Content = &nativeTaskResult{
				VideoURL:     content.VideoURL,
				LastFrameURL: content.LastFrameURL,
			}
		}
	}
	if usage := data.ResultSummary.Usage; usage.TotalTokens > 0 || usage.ToolUsage != nil {
		native := &nativeTaskUsage{
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		}
		// 辅助计量字段解析失败只省略 tool_usage，不影响已完成的视频查询。
		var toolUsage struct {
			WebSearch int `json:"web_search"`
		}
		if len(usage.ToolUsage) > 0 && common.Unmarshal(usage.ToolUsage, &toolUsage) == nil {
			native.ToolUsage = &nativeToolUsage{WebSearch: toolUsage.WebSearch}
		}
		response.Usage = native
	}
	if len(data.Tools) > 0 {
		var tools []nativeTool
		if common.Unmarshal(data.Tools, &tools) == nil {
			for _, tool := range tools {
				if tool.Type != "" {
					response.Tools = append(response.Tools, tool)
				}
			}
		}
	}
	if data.ResultSummary.Resolution != "" {
		response.Resolution = data.ResultSummary.Resolution
	}
	if seconds, err := strconv.Atoi(data.ResultSummary.Duration); err == nil && seconds > 0 {
		response.Duration = &seconds
	}
	if originTask.Status == model.TaskStatusFailure {
		response.Error = &nativeTaskError{
			Code:    failureCode(&data),
			Message: failureMessage(originTask, &data),
		}
	}

	return common.Marshal(response)
}

func failureMessage(task *model.Task, data *upstreamTaskData) string {
	if task.FailReason != "" {
		return task.FailReason
	}
	if data.ResultSummary.Error != nil && data.ResultSummary.Error.Message != "" {
		return data.ResultSummary.Error.Message
	}
	if data.Error != nil && data.Error.Message != "" {
		return data.Error.Message
	}
	return "upstream task failed"
}

// failureCode 透出上游错误码，优先结果信封内的 error.code，其次任务级顶层
// error.code。上游没给码时沿用网关既有的 TaskFailed，保持响应形状不变。
func failureCode(data *upstreamTaskData) string {
	if data.ResultSummary.Error != nil && data.ResultSummary.Error.Code != "" {
		return string(data.ResultSummary.Error.Code)
	}
	if data.Error != nil && data.Error.Code != "" {
		return string(data.Error.Code)
	}
	return "TaskFailed"
}

// codeValue 兼容上游 error.code 的 string / number 形态：华鲲不同接口的码类型
// 并不统一。无法识别的形态保留原始 JSON 片段，避免一个字段阻塞整条响应解析。
type codeValue string

func (c *codeValue) UnmarshalJSON(data []byte) error {
	var s string
	if err := common.Unmarshal(data, &s); err == nil {
		*c = codeValue(s)
		return nil
	}
	var n int64
	if err := common.Unmarshal(data, &n); err == nil {
		*c = codeValue(strconv.FormatInt(n, 10))
		return nil
	}
	*c = codeValue(bytes.TrimSpace(data))
	return nil
}
