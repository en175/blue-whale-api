package material_library

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	ctxCallbackURL
	ctxProjectName
	ctxCustomCallback
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
	// 对外真人会话：客户自定义回调 → 替换为网关回调（存原值，认证后 302 到客户），
	// 保证网关能拿到认证结果记录归属。管理端（无自定义回调）不替换。
	if action == "CreateVisualValidateSession" {
		rewritten, customCallback := rewriteVisualCallbackBody(r, body)
		body = rewritten
		if customCallback != "" {
			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxCustomCallback, customCallback)
			ctx = context.WithValue(ctx, ctxCallbackURL, gatewayCallbackURL(r))
			r = r.WithContext(ctx)
		}
	}
	// 对外查询分组列表：查本地归属表（按 API key 用户隔离，避免越权看到他人组），
	// 保持 ARK 信封格式返回，不查询上游。
	if action == "ListAssetGroups" {
		handleLocalAssetGroups(w, r, body)
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

// rewriteVisualCallbackBody 对外真人会话的请求体改写：客户自定义 CallbackURL →
// 替换为网关回调（保证网关拿到认证结果），返回改写后 body 与客户原回调。
// 客户未传或传的已是网关回调时不改写（返回原 body、customCallback 空）。
func rewriteVisualCallbackBody(r *http.Request, body []byte) ([]byte, string) {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body, ""
	}
	clientCB, _ := m["CallbackURL"].(string)
	if clientCB == "" {
		return body, ""
	}
	gatewayCB := gatewayCallbackURL(r)
	if clientCB == gatewayCB {
		return body, ""
	}
	m["CallbackURL"] = gatewayCB
	rewritten, err := common.Marshal(m)
	if err != nil {
		return body, ""
	}
	return rewritten, clientCB
}

// gatewayCallbackURL 网关真人认证回调地址：优先配置 callback_url，未配置拼请求 host。
func gatewayCallbackURL(r *http.Request) string {
	if cb := material_setting.GetConfig().CallbackURL; cb != "" {
		return cb
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/v1/material/h5-callback"
}

// AssetGroupItem 对外 ListAssetGroups 本地查询的组条目（ARK 信封 Items 字段）。
type AssetGroupItem struct {
	Id         string `json:"Id"`
	Name       string `json:"Name"`
	GroupType  string `json:"GroupType"`
	CreateTime string `json:"CreateTime"`
	UpdateTime string `json:"UpdateTime"`
}

// ListAssetGroupsResult 对外 ListAssetGroups 本地查询的 Result 信封。
type ListAssetGroupsResult struct {
	TotalCount int              `json:"TotalCount"`
	Items      []AssetGroupItem `json:"Items"`
	PageNumber int              `json:"PageNumber"`
	PageSize   int              `json:"PageSize"`
}

// handleLocalAssetGroups 对外 ListAssetGroups 的本地实现：查当前用户（API key 所属）在当前
// provider 下的归属组，按 Filter.GroupType 过滤 + 分页，包装成 ARK 信封返回。
// 不查询上游——素材凭证共享，透传会越权看到其他租户的组。
func handleLocalAssetGroups(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		Filter struct {
			GroupType string `json:"GroupType"`
		} `json:"Filter"`
		PageNumber int `json:"PageNumber"`
		PageSize   int `json:"PageSize"`
	}
	_ = common.Unmarshal(body, &req) // 解析失败按默认分页返回
	page, pageSize := 1, 20
	if req.PageNumber >= 1 {
		page = req.PageNumber
	}
	if req.PageSize >= 1 && req.PageSize <= 100 {
		pageSize = req.PageSize
	}
	userID, _ := r.Context().Value(ctxUserID).(int)
	groups, err := model.GetMaterialGroups(userID, material_setting.GetConfig().Provider, req.Filter.GroupType)
	if err != nil {
		logger.LogError(r.Context(), "list local asset groups failed: "+err.Error())
		http.Error(w, "list asset groups failed", http.StatusInternalServerError)
		return
	}
	total := len(groups)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]AssetGroupItem, 0, end-start)
	for _, g := range groups[start:end] {
		items = append(items, AssetGroupItem{
			Id:         g.SeedanceAssetGroupId,
			Name:       g.Name,
			GroupType:  g.GroupType,
			CreateTime: time.Unix(g.CreatedAt, 0).UTC().Format(time.RFC3339),
			UpdateTime: time.Unix(g.UpdatedAt, 0).UTC().Format(time.RFC3339),
		})
	}
	envelope := map[string]any{
		"ResponseMetadata": map[string]any{
			"RequestId": "req_local_" + strconv.Itoa(userID) + "_" + strconv.Itoa(page),
			"Action":    "ListAssetGroups",
			"Version":   arkVersion,
			"Service":   "ark",
			"Region":    material_setting.GetConfig().VolcengineRegion,
		},
		"Result": ListAssetGroupsResult{TotalCount: total, Items: items, PageNumber: page, PageSize: pageSize},
	}
	data, err := common.Marshal(envelope)
	if err != nil {
		http.Error(w, "list asset groups failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// observeOwnership 观察上游响应：仅在创建/删除素材组且成功时更新归属表。
// 只有标记了记录操作的请求才携带记录信息（RecordContext 已提取）。
func observeOwnership(ctx context.Context, resp *Response) {
	userID, _ := ctx.Value(ctxUserID).(int)
	groupType, isCreate := ctx.Value(ctxGroupType).(string)
	groupID, isDelete := ctx.Value(ctxGroupID).(string)
	callbackURL, isVisual := ctx.Value(ctxCallbackURL).(string)
	if userID <= 0 || (!isCreate && !isDelete && !isVisual) {
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}

	if isVisual {
		// 真人认证会话：响应 Result.BytedToken/H5Link 写会话表（回调反查用户 + 排查链路）。
		var out struct {
			Result struct {
				BytedToken string `json:"BytedToken"`
				H5Link     string `json:"H5Link"`
			} `json:"Result"`
		}
		if err := common.Unmarshal(resp.Body, &out); err != nil || out.Result.BytedToken == "" {
			return
		}
		projectName, _ := ctx.Value(ctxProjectName).(string)
		customCallback, _ := ctx.Value(ctxCustomCallback).(string)
		// 代理路径无组名（认证后回调才拿 GroupId），name 留空；管理端点直连时由 controller 传入。
		// callbackURL 是发给上游的网关回调；customCallback 是客户自定义原回调（认证后 302 到它）。
		if err := model.AddVisualValidateSession(userID, material_setting.GetConfig().Provider, "", out.Result.BytedToken, out.Result.H5Link, callbackURL, projectName, customCallback); err != nil {
			logger.LogError(ctx, "material library record visual session failed: "+err.Error())
		}
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
	case "CreateVisualValidateSession":
		// 真人认证会话：记录发起时的回调地址与项目，供响应侧写会话表。
		fields := readJSONBodyFields(c, "CallbackURL", "ProjectName")
		ctx = context.WithValue(ctx, ctxCallbackURL, fields["CallbackURL"])
		ctx = context.WithValue(ctx, ctxProjectName, fields["ProjectName"])
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
