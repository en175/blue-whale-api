package tokenhub25

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	tasktokenhub "github.com/QuantumNous/new-api/relay/channel/task/tokenhub"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TaskAdaptor 是华鲲（ai-tokenhub）上 Seedance 2.5 的专用渠道（type 67）适配器。
//
// 它嵌入 tokenhub 的适配器以复用华鲲 /api/v2 的提交/查询/解析逻辑，只覆盖与
// 样片模式相关的五个方法。
//
// 嵌的是 tokenhub 而不是 doubao（volcengine25 走的那条）：华鲲把结果包在
// resultSummary 信封里（resultSummary.content.video_url + resultSummary.usage），
// 而 doubao 读的是顶层 content.video_url / status。嵌 doubao 会让 status 永远解析
// 为空，任务卡在 IN_PROGRESS 直到 24h 超时扫尾，video_url 也永远取不到。
type TaskAdaptor struct {
	tasktokenhub.TaskAdaptor
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// ValidateRequestAndSetAction 解析请求并区分三步：
//
//	成片（finalize）：走白名单校验，绝不落入 tokenhub 的 action 覆盖
//	样片（draft）   ：在 tokenhub 的解析之上补 480p 守卫
//	普通提交        ：完全沿用 tokenhub 行为
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 必须在 tokenhub 之前判 finalize：它的 ValidateBasicTaskRequest 末尾会用
	// storeTaskRequest 把 info.Action 覆盖成 generate（既有 remix 丢 action 的
	// 同一个坑）。成片请求体里也没有 prompt，走 tokenhub 的 validatePrompt 必然 400。
	if info != nil && info.Action == constant.TaskActionFinalize {
		return validateFinalizeSubmit(c, info)
	}

	if taskErr := a.TaskAdaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	// 样片只允许 480p。这里明确拒绝而不是静默改写：静默改写会让客户端以为
	// 自己拿到了 1080p，而拒绝能在预扣费之前就返回。
	if taskcommon.IsDraftRequest(&req) {
		if resolution := requestedResolution(&req); resolution != "" && resolution != Res480p {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("draft task only supports resolution %s", Res480p), "invalid_resolution", http.StatusBadRequest)
		}
		info.Draft = true
	}
	// 该字段此前被静默丢弃，客户端无从得知自己的任务类型意图没有生效。类型
	// 不对时必须明确拒绝，不能沿用丢弃。取值本身（auto / reference / edit /
	// extend 及各自的参数约束）由上游权威判定，网关不复制那套会变的规则。
	if raw, exists := req.Metadata[OmniReferenceTaskTypeKey]; exists {
		if _, ok := raw.(string); !ok {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("%s must be a string", OmniReferenceTaskTypeKey),
				"invalid_omni_reference_task_type", http.StatusBadRequest)
		}
	}
	info.HasVideoInput = taskcommon.HasVideoInputInMetadata(req.Metadata)
	return nil
}

// BuildRequestBody 构造上游请求体。成片与样片都要改写 tokenhub 生成的 payload。
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if info != nil && info.Action == constant.TaskActionFinalize {
		info.RequestedResolution = Res1080p
		info.UpstreamResolution = Res1080p
		if info.OriginUpstreamTaskID == "" {
			// 正常情况下 ResolveOriginTask 已经拒绝；这里兜底避免发出无 id 的样片引用
			return nil, fmt.Errorf("origin upstream task id is missing")
		}
		payload, err := buildFinalizePayload(info, finalizeParamsFromContext(c))
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(payload), nil
	}

	body, err := a.TaskAdaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, err
	}

	// omni_reference_task_type 不在 tokenhub 的共用白名单里，在这里补写。客户端
	// 没传时不改写 payload——普通提交与样片路径都与改动前逐位一致。
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	if v, ok := req.Metadata[OmniReferenceTaskTypeKey].(string); ok {
		body, err = rewritePayloadFields(body, map[string]any{OmniReferenceTaskTypeKey: v})
		if err != nil {
			return nil, err
		}
	}

	if !info.Draft {
		return body, nil
	}

	// 样片：无论客户端从顶层 draft 还是 metadata.draft 进入，都保证上游同时
	// 收到 draft=true 与 resolution=480p。
	body, err = rewritePayloadFields(body, map[string]any{
		"draft":      true,
		"resolution": Res480p,
	})
	if err != nil {
		return nil, err
	}
	info.RequestedResolution = Res480p
	info.UpstreamResolution = Res480p
	return body, nil
}

// EstimateBilling 返回档位价倍率。
//
// 成片请求体里没有 content，因此「是否含视频输入」这一维度必须继承第一步的
// 样片任务；分辨率则固定取成片档 1080p。这两点合起来决定 1080p 档的真实单价。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	if info != nil && info.Action == constant.TaskActionFinalize {
		return taskcommon.EstimateVideoTierBillingForTier(info, Res1080p, info.OriginHasVideoInput)
	}
	return a.TaskAdaptor.EstimateBilling(c, info)
}
