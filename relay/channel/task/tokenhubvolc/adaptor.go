package tokenhubvolc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
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
//	成片（finalize）：中间件已校验过 draft_task 引用，这里不再走通用校验
//	样片（draft）   ：只允许 2.5 模型，且只允许 480p
//	普通提交        ：完全沿用 /v1/videos 的通用校验
//
// 通用校验读的是中间件改写后的内部请求体（prompt + metadata），因此提示词必填、
// 时长上限等既有约束原样生效。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	native, ok := NativeRequestFrom(c)
	if !ok {
		return service.TaskErrorWrapperLocal(fmt.Errorf("ark native request is missing"), "invalid_request", http.StatusBadRequest)
	}

	if info != nil && info.Action == constant.TaskActionFinalize {
		return nil
	}

	if taskErr := a.TaskAdaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	info.HasVideoInput = native.HasVideoInput()
	// 通用校验把 action 固定成 generate，任务日志会把纯文本和参考视频都显示成
	// 「图生视频」。这里按 content[] 的实际输入形态改写，与 doubao 适配器同口径。
	info.Action = inferNativeAction(native)

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
	if !isSeedance25Model(clientModel, info.OriginModelName, mappedModelName(c, clientModel)) {
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
// 的顶层参数（tools、priority 等）无需在本包建模也能透传。成片请求则从零构造：
// 它的 content 只能是 draft_task 引用，可重设的只有输出规格类参数。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	native, ok := NativeRequestFrom(c)
	if !ok {
		return nil, fmt.Errorf("ark native request is missing")
	}

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
		for _, key := range finalizeOptionKeys {
			if value, exists := native.Raw[key]; exists && value != nil {
				payload[key] = value
			}
		}
		info.RequestedResolution = Res1080p
		info.UpstreamResolution = Res1080p
	} else {
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

// finalizeOptionKeys 是成片请求里可以重设的输出规格类参数。content / prompt /
// duration / ratio / seed / generate_audio 刻意不在其中：成片沿用样片的内容与
// 时长，客户端传什么都影响不了成片本身。
var finalizeOptionKeys = []string{
	"output_format",
	"watermark",
	"return_last_frame",
	"service_tier",
	"execution_expires_after",
	"priority",
	"callback_url",
	"safety_identifier",
}

// EstimateBilling 返回档位价倍率（video_input）。
// 样片固定按 480p 档；成片固定 1080p，且「是否含视频输入」继承第一步的样片。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if info != nil && info.Action == constant.TaskActionFinalize {
		ratios := taskcommon.EstimateVideoTierBillingForTier(info, Res1080p, info.OriginHasVideoInput)
		warnMissingTier(c, info, Res1080p, info.OriginHasVideoInput, ratios)
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
		warnMissingTier(c, info, normalized, taskcommon.HasVideoInputInMetadata(req.Metadata), ratios)
	}
	return ratios
}

const tierWarnInterval = 10 * time.Minute

// missingTierWarned 去重档位缺失告警：同一 model + 档位 + 输入维度在
// tierWarnInterval 内只告警一次，避免每次请求刷屏；超过间隔重新告警，
// 管理员修好配置后又配错时无需重启进程。
var missingTierWarned sync.Map

// warnMissingTier 在按档位收费、但该档位没有显式单价时告警，避免静默按基准价
// 结算（PRD 5.6）。只认「漏配」：倍率为 1.0 可能是配置了与基准价相同的单价，
// 用 VideoTierPriceConfigured 区分，不误报。
// 样片固定 480p，与 720p 同属基准键（video_tier_pricing 的 base 覆盖
// 480p/720p），基准价本就是它的正确价格，因此不调用本函数。
func warnMissingTier(c *gin.Context, info *relaycommon.RelayInfo, resolution string, hasVideo bool, ratios map[string]float64) {
	if info == nil || ratios != nil {
		return
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	if taskcommon.VideoTierPriceConfigured(info.UpstreamModelName, resolution, hasVideo) ||
		taskcommon.VideoTierPriceConfigured(info.OriginModelName, resolution, hasVideo) {
		return // 档位有显式单价，只是恰好等于基准价
	}
	key := fmt.Sprintf("%s|%s|%t", model, resolution, hasVideo)
	now := time.Now()
	if last, loaded := missingTierWarned.Load(key); loaded && now.Sub(last.(time.Time)) < tierWarnInterval {
		return
	}
	missingTierWarned.Store(key, now)
	logger.LogWarn(c.Request.Context(), "video tier pricing missing: model="+model+
		" resolution="+resolution+" has_video="+strconv.FormatBool(hasVideo)+
		" action="+string(info.Action)+", billing falls back to base price")
}

// DoResponse 解析华鲲的 create 响应，并只回下游 {"id": <网关公开任务 ID>}。
// 上游的 cgt- 任务 ID 只落库，不暴露给下游。
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

	c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
	return upstream.ID, responseBody, nil
}

// createResponse 是华鲲 create 接口的最小解析目标：成功只有 id，失败带
// error/message/errorCode。
type createResponse struct {
	ID      string `json:"id"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

// inferNativeAction 按 content[] 的实际输入形态写入任务类型。任务日志与筛选
// 依赖 action 区分「文生 / 图生 / 视频生」，与 doubao 适配器保持一致：视频优先，
// 其次图片，纯文本兜底。音频与 draft_task 不单独构成一种类型。
func inferNativeAction(native *NativeRequest) string {
	hasImage := false
	for _, item := range native.Content {
		if item.Type == "video_url" || item.VideoURL != nil {
			return constant.TaskActionVideoGenerate
		}
		if item.Type == "image_url" || item.ImageURL != nil {
			hasImage = true
		}
	}
	if hasImage {
		return constant.TaskActionGenerate
	}
	return constant.TaskActionTextGenerate
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

// mappedModelName 按渠道 model_mapping 解析上游模型名，支持链式重定向。
// 只读上下文里的配置，不改 info；解析失败时退回原名，交正式映射步骤报错。
func mappedModelName(c *gin.Context, modelName string) string {
	raw := c.GetString("model_mapping")
	if raw == "" || raw == "{}" || modelName == "" {
		return modelName
	}
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
		return modelName
	}
	visited := map[string]bool{modelName: true}
	current := modelName
	for {
		next, exists := mapping[current]
		if !exists || next == "" || visited[next] {
			return current
		}
		visited[next] = true
		current = next
	}
}
