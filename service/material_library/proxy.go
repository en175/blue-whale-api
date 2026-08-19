package material_library

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/gin-gonic/gin"
)

// mountPrefix 是网关侧挂载点，代理把它映射成上游 ARK 兼容接口的 Action。
// 客户端路径 /api/material/{Action} 即上游 /ark 的 Action 操作名。
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

// Proxy 构造指向 ai-tokenhub 素材库（火山 ARK 兼容接口）的透明反向代理：
// 仅把路径段映射为 Action 并注入凭证头，其余（query / body / 响应）原样透传，
// 上游状态码与错误信封直接回传。ReverseProxy 无请求间状态，可作包级单例；
// 凭证在 Director 请求期实时读取。响应观察层静默记录"用户↔素材组"归属。
func Proxy() *httputil.ReverseProxy {
	return proxy
}

var proxy = &httputil.ReverseProxy{
	Director:       director,
	ModifyResponse: observeResponse,
	ErrorHandler:   errorHandler,
	FlushInterval:  -1,
}

// director 在转发前改写目标：/api/material/{Action} -> {base_url}/ark?Action={Action}&Version=...，
// 并注入 AK/SK 凭证头。租户隐含在凭证中，接口内不传租户 ID。
func director(req *http.Request) {
	cfg := material_setting.GetConfig()

	action := strings.TrimPrefix(req.URL.Path, mountPrefix)
	action = strings.TrimPrefix(action, "/")

	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		// base_url 非法：让 ReverseProxy 拨号失败走 ErrorHandler，返回 502。
		req.URL.Host = ""
		req.URL.Scheme = ""
		return
	}
	req.URL.Scheme = base.Scheme
	req.URL.Host = base.Host
	req.Host = base.Host
	req.URL.Path = "/ark"

	query := req.URL.Query()
	query.Set("Action", action)
	query.Set("Version", arkVersion)
	req.URL.RawQuery = query.Encode()

	req.Header.Set("Authorization", "Bearer "+cfg.AK+":"+cfg.SK)
}

func errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	// 不把 Go 内部错误细节返回给客户端，细节进带 trace_id 的后端日志。
	logger.LogError(r.Context(), "material library upstream error: "+err.Error())
	http.Error(w, "material library upstream error", http.StatusBadGateway)
}

// actionFromPath 从 /api/material/{Action} 中取出 Action 操作名。
func actionFromPath(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, mountPrefix), "/")
}

// RecordContext 从登录请求中提取归属记录所需信息并写入 context，供 observeResponse 使用。
// 需要读请求体（建组名、删组 id）时读取后原样装回，不影响代理转发；其余路径直接透传。
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

// observeResponse 观察上游响应：仅在创建/删除素材组且成功时更新归属表。
// 只有标记了记录操作的请求才读取响应体（读取后原样装回，不改动任何字段）。
func observeResponse(resp *http.Response) error {
	reqCtx := resp.Request.Context()
	userID, _ := reqCtx.Value(ctxUserID).(int)
	groupType, isCreate := reqCtx.Value(ctxGroupType).(string)
	groupID, isDelete := reqCtx.Value(ctxGroupID).(string)
	if userID <= 0 || (!isCreate && !isDelete) {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	if isCreate {
		// ARK 信封：素材组 id 在 Result.Id（火山格式 group-...）。
		var out struct {
			Result struct {
				Id string `json:"Id"`
			} `json:"Result"`
		}
		if err := common.Unmarshal(body, &out); err != nil || out.Result.Id == "" {
			return nil
		}
		name, _ := reqCtx.Value(ctxGroupName).(string)
		if err := model.AddUserMaterialGroup(userID, out.Result.Id, name, groupType); err != nil {
			logger.LogError(reqCtx, "material library record group failed: "+err.Error())
		}
		return nil
	}

	if err := model.DeleteUserMaterialGroup(groupID); err != nil {
		logger.LogError(reqCtx, "material library delete group record failed: "+err.Error())
	}
	return nil
}

// readJSONBodyFields 读取请求体 JSON 的指定字段，并把 body 原样装回供代理转发（只读一次）。
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
