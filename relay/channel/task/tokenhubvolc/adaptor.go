package tokenhubvolc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/arknative"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	tasktokenhub "github.com/QuantumNous/new-api/relay/channel/task/tokenhub"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TaskAdaptor 是火山方舟原生协议渠道（type 70）的适配器。上游仍是华鲲
// ai-tokenhub 的 /api/v2，对客请求/响应形状由本包与请求转换中间件共同维护。
//
// 嵌入 tokenhub 的适配器以复用提交地址、鉴权头、轮询与 resultSummary 解析；
// 请求体与两个响应都自己实现——华鲲的 create 返回 {"id":...}，查询结果是
// resultSummary 信封，都不能直接吐给下游。
type TaskAdaptor struct {
	tasktokenhub.TaskAdaptor
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// ValidateRequestAndSetAction 区分三步：
//
//	成片（finalize）：原生入口的引用已由中间件校验，旧接口只解析输出规格参数
//	样片（draft）   ：只允许 2.5 模型，且只允许 480p
//	普通提交        ：完全沿用 /v1/videos 的通用校验
//
// 通用校验读的是中间件改写后的内部请求体（prompt + metadata），因此提示词必填、
// 时长上限等既有约束原样生效。旧接口的请求体由 arknative.ArkRequest 还原成火山形状后，
// 与原生入口走完全相同的前两条判定。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info != nil && info.Action == constant.TaskActionFinalize {
		return arknative.ValidateFinalizeRequest(c, Res1080p)
	}

	// 通用校验先把内部请求体解析并写进上下文；旧接口的火山形状要在它之后才能还原。
	if taskErr := a.TaskAdaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return taskErr
	}

	native, ok := arknative.ArkRequest(c)
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("task metadata cannot be converted to a video request"), "invalid_request", http.StatusBadRequest)
	}

	// 成片只能走 /v1/videos/{id}/finalize：那条路径经 ResolveOriginTask 校验归属、
	// 样片状态与 7 天窗口，并按成片档位计费。旧提交入口收到 draft_task 引用必须
	// 拒绝，否则它会被当作普通 content 透传给上游，绕过全部成片校验与计费。
	if _, isNative := arknative.NativeRequestFrom(c); !isNative && native.HasDraftTask() {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("draft_task references are only accepted on the finalize endpoint"),
			"invalid_content", http.StatusBadRequest)
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	info.HasVideoInput = native.HasVideoInput()
	// 通用校验把 action 固定成 generate，任务日志会把纯文本和参考视频都显示成
	// 「图生视频」。这里按 content[] 的实际输入形态改写，与 doubao 适配器同口径。
	info.Action = arknative.InferAction(native)

	// duration 是计费乘数之一。火山用 -1 表示「编辑/延长」由模型决定时长，
	// 其余必须是正整数且不超过通用上限——原生协议的 duration 不在
	// /v1/videos 的通用校验范围内，必须在这里补上同样的钳制。
	if native.Duration != nil {
		if d := *native.Duration; d != -1 && (d < 1 || d > relaycommon.MaxTaskDurationSeconds) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("duration must be -1 or between 1 and %d", relaycommon.MaxTaskDurationSeconds),
				"invalid_duration", http.StatusBadRequest)
		}
	}

	if !native.IsDraft() {
		return nil
	}

	// 样片模式是 Seedance 2.5 独有的能力；其它模型收到 draft 只会被上游忽略，
	// 客户端却以为拿到了样片，因此在预扣费之前明确拒绝。
	// 渠道 model_mapping 在 RelayTaskSubmit 的第 2 步才生效，这里提前解析一次，
	// 否则把 2.5 映射成不含 "seedance-2-5" 的别名时会被误拒。
	clientModel := req.Model
	if clientModel == "" {
		clientModel = info.OriginModelName
	}
	if !isSeedance25Model(clientModel, info.OriginModelName, taskcommon.MappedModelName(c, clientModel)) {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("draft mode is only supported by Seedance 2.5 models"), "invalid_draft_model", http.StatusBadRequest)
	}
	if resolution := strings.ToLower(strings.TrimSpace(native.Resolution)); resolution != "" && resolution != Res480p {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("draft task only supports resolution %s", Res480p), "invalid_resolution", http.StatusBadRequest)
	}
	info.Draft = true
	return nil
}

// BuildRequestBody 构造发给华鲲的请求体。
//
// 普通与样片请求从下游原始 JSON 出发，只覆盖 model 与样片相关字段——火山新增
// 的顶层参数（tools、priority 等）无需在本包建模也能透传。旧接口的原始 JSON
// 由 arknative.ArkRequest 从 prompt + metadata 合成，其后完全同路。成片请求则从零构造：
// 它的 content 只能是 draft_task 引用，可重设的只有输出规格类参数。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	native, _ := arknative.ArkRequest(c)

	var payload map[string]any
	if info != nil && info.Action == constant.TaskActionFinalize {
		if info.OriginUpstreamTaskID == "" {
			// 正常情况下 ResolveOriginTask 已经拒绝；这里兜底避免发出无 id 的样片引用
			return nil, fmt.Errorf("origin upstream task id is missing")
		}
		payload = map[string]any{
			"content": []any{map[string]any{
				"type":       "draft_task",
				"draft_task": map[string]any{"id": info.OriginUpstreamTaskID},
			}},
			// 上游成片档位只有 1080p：客户端传任何值都被覆盖，不报错。
			"resolution": Res1080p,
		}
		arknative.ApplyFinalizeOptions(payload, c, native)
		info.RequestedResolution = Res1080p
		info.UpstreamResolution = Res1080p
	} else {
		if native == nil {
			return nil, fmt.Errorf("ark native request is missing")
		}
		if native.HasDraftTask() {
			// 校验阶段已拒绝旧接口的 draft_task；这里兜底，避免任何入口把它
			// 当普通 content 透传、绕过成片归属与计费。
			return nil, fmt.Errorf("draft_task references are only accepted on finalize requests")
		}
		payload = make(map[string]any, len(native.Raw)+2)
		for key, value := range native.Raw {
			payload[key] = value
		}
		// group 是网关的分组路由键，不是火山字段
		delete(payload, "group")
		if info.Draft {
			// 无论客户端只传了 draft 还是只传了 resolution，上游都必须同时收到
			// draft=true 与 480p，否则样片会被当成普通 1080p 任务计费。
			payload["draft"] = true
			payload["resolution"] = Res480p
			info.RequestedResolution = Res480p
			info.UpstreamResolution = Res480p
		} else if native.Resolution != "" {
			info.RequestedResolution = native.Resolution
		}
	}

	payload["model"] = info.UpstreamModelName
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// EstimateBilling 返回档位价倍率（video_input）。
// 样片固定按 480p 档；成片固定 1080p，且「是否含视频输入」继承第一步的样片。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if info != nil && info.Action == constant.TaskActionFinalize {
		ratios := taskcommon.EstimateVideoTierBillingForTier(info, Res1080p, info.OriginHasVideoInput)
		taskcommon.WarnMissingTier(c, info, Res1080p, info.OriginHasVideoInput, ratios)
		return ratios
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	if info != nil && info.Draft {
		return taskcommon.EstimateVideoTierBillingForTier(info, Res480p, info.HasVideoInput)
	}
	ratios := taskcommon.EstimateVideoTierBilling(info, &req)
	// 直接提交 1080p / 4k 同样按档位收费：漏配时也是静默按基准价结算。
	// 基准档（480p/720p 及未传分辨率）本来就是基准价，不告警。
	resolution, _ := req.Metadata["resolution"].(string)
	if normalized := strings.ToLower(strings.TrimSpace(resolution)); normalized == Res1080p || normalized == Res4k {
		taskcommon.WarnMissingTier(c, info, normalized, taskcommon.HasVideoInputInMetadata(req.Metadata), ratios)
	}
	return ratios
}

// DoResponse 解析华鲲的 create 响应，按入口回不同形状：原生协议只回
// {"id": <网关公开任务 ID>}，旧视频接口回 OpenAI 视频对象（与 61/66/67 一致）。
// 上游的 cgt- 任务 ID 不在创建响应里暴露，由查询接口的 upstreamTaskId 回给下游。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var upstream createResponse
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if upstream.ID == "" {
		message := upstream.Message
		if message == "" {
			message = "task_id is empty"
		}
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("%s (error=%s)", message, upstream.Error), "upstream_error", http.StatusInternalServerError)
	}

	if c.Request != nil && strings.HasPrefix(c.Request.URL.Path, constant.NativeArkVideoTaskPath) {
		c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
		return upstream.ID, responseBody, nil
	}

	video := dto.NewOpenAIVideo()
	video.ID = info.PublicTaskID
	video.TaskID = info.PublicTaskID
	video.CreatedAt = time.Now().Unix()
	video.Model = info.OriginModelName
	c.JSON(http.StatusOK, video)
	return upstream.ID, responseBody, nil
}

// createResponse 是华鲲 create 接口的最小解析目标：成功只有 id，失败带
// error/message/errorCode。
type createResponse struct {
	ID      string `json:"id"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

// isSeedance25Model 报告模型名是否为 Seedance 2.5 系（样片模式的唯一支持者）。
// 对客名与映射后的上游名都可能是 2.5，调用方两者都传进来。
func isSeedance25Model(names ...string) bool {
	for _, name := range names {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			continue
		}
		if strings.Contains(lower, "seedance-2-5") || strings.Contains(lower, "seedance-2.5") {
			return true
		}
	}
	return false
}
