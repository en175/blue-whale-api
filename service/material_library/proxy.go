package material_library

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/gin-gonic/gin"
)

// mountPrefix 是网关侧挂载点，客户端路径 /api/material/{Action} 即上游 Action 操作名。
const mountPrefix = "/api/material"

// arkVersion 是上游 ARK 兼容接口的固定版本参数。
const arkVersion = "2024-01-01"

// recordKey 是归属记录信息在请求 context 中的键类型，避免与业务 context 键冲突。
type recordKey int

const (
	ctxUserID recordKey = iota
	ctxGroupName
	ctxGroupType
	ctxGroupID
)

// Proxy 返回 /api/material/{Action} 的处理器：把路径段映射为 Action，经当前 provider 执行，
// 上游状态码/头/体原样回写（含错误信封）。不再用 ReverseProxy——火山签名无法透传，
// 改由 provider 统一分发；客户端看到的 API 面（Action 契约 + 上游错误格式）不变。
// 响应观察层静默记录"用户↔素材组"归属。
func Proxy() http.Handler {
	return http.HandlerFunc(proxyHandler)
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	action := actionFromPath(r.URL.Path)
	if action == "" {
		http.Error(w, "missing action", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.LogError(r.Context(), "material library read request body failed: "+err.Error())
		http.Error(w, "material library read request body failed", http.StatusBadRequest)
		return
	}
	resp, err := currentProvider().DoAction(r.Context(), action, r.URL.Query(), body)
	if err != nil {
		// 不把 Go 内部错误细节返回给客户端，细节进带 trace_id 的后端日志。
		logger.LogError(r.Context(), "material library upstream error: "+err.Error())
		http.Error(w, "material library upstream error", http.StatusBadGateway)
		return
	}
	observeOwnership(r.Context(), resp)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

// observeOwnership 观察上游响应：仅在创建/删除素材组且成功时更新归属表。
// 只有标记了记录操作的请求才携带记录信息（RecordContext 已提取）。
func observeOwnership(ctx context.Context, resp *Response) {
	userID, _ := ctx.Value(ctxUserID).(int)
	groupType, isCreate := ctx.Value(ctxGroupType).(string)
	groupID, isDelete := ctx.Value(ctxGroupID).(string)
	if userID <= 0 || (!isCreate && !isDelete) {
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	if isCreate {
		// ARK 信封：素材组 id 在 Result.Id（火山格式 group-...）。
		var out struct {
			Result struct {
				Id string `json:"Id"`
			} `json:"Result"`
		}
		if err := common.Unmarshal(resp.Body, &out); err != nil || out.Result.Id == "" {
			return
		}
		name, _ := ctx.Value(ctxGroupName).(string)
		if err := model.AddUserMaterialGroup(userID, material_setting.GetConfig().Provider, out.Result.Id, name, groupType); err != nil {
			logger.LogError(ctx, "material library record group failed: "+err.Error())
		}
		return
	}

	if err := model.DeleteUserMaterialGroup(material_setting.GetConfig().Provider, groupID); err != nil {
		logger.LogError(ctx, "material library delete group record failed: "+err.Error())
	}
}

// actionFromPath 从 /api/material/{Action} 中取出 Action 操作名。
func actionFromPath(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, mountPrefix), "/")
}

// RecordContext 从登录请求中提取归属记录所需信息并写入 context，供 observeOwnership 使用。
// 需要读请求体（建组名、删组 id）时读取后原样装回，不影响分发；其余路径直接透传。
func RecordContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	userID := c.GetInt("id")
	if userID <= 0 {
		return ctx
	}
	ctx = context.WithValue(ctx, ctxUserID, userID)

	switch actionFromPath(c.Request.URL.Path) {
	case "CreateAssetGroup":
		// 组类型取请求体（上游 CreateAssetGroup 支持 GroupType，当前仅 AIGC），
		// 不硬编码，避免未来出现 LivenessFace 时被错记。
		fields := readJSONBodyFields(c, "GroupType", "Name")
		ctx = context.WithValue(ctx, ctxGroupType, fields["GroupType"])
		ctx = context.WithValue(ctx, ctxGroupName, fields["Name"])
	case "DeleteAssetGroup":
		ctx = context.WithValue(ctx, ctxGroupID, readJSONBodyField(c, "Id"))
	}
	return ctx
}

// readJSONBodyFields 读取请求体 JSON 的指定字段，并把 body 原样装回供分发（只读一次）。
func readJSONBodyFields(c *gin.Context, fields ...string) map[string]string {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return nil
	}
	result := make(map[string]string, len(fields))
	for _, field := range fields {
		if v, ok := m[field].(string); ok {
			result[field] = v
		}
	}
	return result
}

// readJSONBodyField 读取请求体 JSON 的单个字段，body 原样装回。
func readJSONBodyField(c *gin.Context, field string) string {
	return readJSONBodyFields(c, field)[field]
}
