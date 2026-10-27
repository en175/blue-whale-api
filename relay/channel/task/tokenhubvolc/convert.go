package tokenhubvolc

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
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
	Usage          *dto.TaskUsage    `json:"usage,omitempty"`
	CreatedAt      int64             `json:"created_at"`
	UpdatedAt      int64             `json:"updated_at"`
	Resolution     string            `json:"resolution,omitempty"`
	Duration       *int              `json:"duration,omitempty"`
	Error          *nativeTaskError  `json:"error,omitempty"`
}

type nativeTaskResult struct {
	VideoURL string `json:"video_url"`
}

type nativeTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// upstreamTaskData 是华鲲查询响应中网关需要的部分：结果包在 resultSummary 里
// （与 /api/v3 的顶层 content 不同）。
type upstreamTaskData struct {
	// UpstreamTaskID 在创建响应与查询响应里都是顶层字段，不在 resultSummary 内。
	UpstreamTaskID string `json:"upstreamTaskId"`
	ResultSummary  struct {
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Resolution string `json:"resolution"`
		Duration   string `json:"duration"`
		Usage      struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"resultSummary"`
	Error *struct {
		Message string `json:"message"`
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

	if originTask.Status == model.TaskStatusSuccess && data.ResultSummary.Content.VideoURL != "" {
		response.Content = &nativeTaskResult{VideoURL: data.ResultSummary.Content.VideoURL}
	}
	if data.ResultSummary.Usage.TotalTokens > 0 {
		response.Usage = &dto.TaskUsage{
			CompletionTokens: data.ResultSummary.Usage.CompletionTokens,
			TotalTokens:      data.ResultSummary.Usage.TotalTokens,
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
			Code:    "TaskFailed",
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
