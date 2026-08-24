package material_library

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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
	var customCallback string
	if action == "CreateVisualValidateSession" {
		var rewritten []byte
		rewritten, customCallback = rewriteVisualCallbackBody(r, body)
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
	// 对外查询真人认证结果：先查本地会话（group_id 已结算则直接返回），
	// 未结算则查上游并回填数据库（会话 group_id + 归属），兼容两个上游返回格式。
	if action == "GetVisualValidateResult" {
		handleGetVisualValidateResult(w, r, body)
		return
	}
	// 对外组操作/素材写入：校验 API key 用户归属（防 IDOR，只能操作自己归属的组）。
	switch action {
	case "GetAssetGroup", "UpdateAssetGroup", "DeleteAssetGroup":
		if !groupOwnedByCurrentUser(r, body) {
			writeMaterialNotFound(w)
			return
		}
	case "ListAssets":
		if listAssetsGroupsEmpty(r, body) {
			// 空 GroupIds 会列出共享账号下全部素材，属越权，直接拒绝。
			writeMaterialBadRequest(w)
			return
		}
		if !groupsOwnedByCurrentUser(r, body) {
			writeMaterialNotFound(w)
			return
		}
	case "CreateAsset":
		// 上传素材：校验目标组归属（请求体 GroupId），防止向他人组注入素材。
		if !createAssetGroupOwned(r, body) {
			writeMaterialNotFound(w)
			return
		}
	case "CreateAssetGroup", "CreateVisualValidateSession":
		// 建组/真人会话：名称含 HTML 危险字符（<>）拒绝，防 XSS 存储。
		if createAssetGroupNameInvalid(r, body) {
			writeMaterialBadRequest(w)
			return
		}
	}
	resp, err := currentProvider().DoAction(r.Context(), action, r.URL.Query(), body)
	if err != nil {
		// 不把 Go 内部错误细节返回给客户端，细节进带 trace_id 的后端日志。
		logger.LogError(r.Context(), "material library upstream error: "+err.Error())
		http.Error(w, "material library upstream error", http.StatusBadGateway)
		return
	}
	observeOwnership(r.Context(), resp)
	// 对外真人会话：响应 Result.CallbackURL 替换回客户原值（客户应看到自己传的回调，而非网关内置）。
	if customCallback != "" {
		resp.Body = restoreClientCallback(resp.Body, customCallback)
	}
	// 统一返回 tokenhub 直接格式（去掉火山 ResponseMetadata/Result 信封），客户端只解析一种格式。
	resp.Body = normalizeUpstreamResponse(resp.Body)
	for k, vs := range resp.Header {
		if k == "Content-Length" {
			continue // 响应体已改写，旧 Content-Length 失效，交由 Go 按实际长度重算（防截断）
		}
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

// restoreClientCallback 把响应里的 CallbackURL 替换为客户原回调（客户应看到自己传的值，
// 而非网关内置的回调地址）。兼容火山信封（Result.CallbackURL）与 tokenhub 直接（顶层 CallbackURL）；
// 无 CallbackURL 或解析失败时原样返回。
func restoreClientCallback(body []byte, callbackURL string) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	if result, ok := m["Result"].(map[string]any); ok {
		if _, has := result["CallbackURL"]; has {
			result["CallbackURL"] = callbackURL
			m["Result"] = result
			if rewritten, err := common.Marshal(m); err == nil {
				return rewritten
			}
		}
	}
	if _, ok := m["CallbackURL"]; ok {
		m["CallbackURL"] = callbackURL
		if rewritten, err := common.Marshal(m); err == nil {
			return rewritten
		}
	}
	return body
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
	// 统一返回 tokenhub 直接格式（无信封）：TotalCount/Items/PageNumber/PageSize 顶层。
	data, err := common.Marshal(ListAssetGroupsResult{TotalCount: total, Items: items, PageNumber: page, PageSize: pageSize})
	if err != nil {
		http.Error(w, "list asset groups failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleGetVisualValidateResult 对外 GetVisualValidateResult 的缓存化实现：
// 先查本地会话（group_id 已结算则直接返回，避免重复查上游）；
// 未结算则按会话（或当前）provider 查上游，拿到 GroupId 后回填会话与归属（幂等）。
// 返回格式按 provider：火山信封 / tokenhub 直接，与上游透传一致。
func handleGetVisualValidateResult(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		BytedToken string `json:"BytedToken"`
	}
	_ = common.Unmarshal(body, &req)
	if req.BytedToken == "" {
		http.Error(w, "missing BytedToken", http.StatusBadRequest)
		return
	}

	provider := material_setting.GetConfig().Provider
	session, _ := model.GetVisualValidateSessionByToken(req.BytedToken)
	if session == nil {
		// 会话不存在（非本网关创建的 token）：fail-closed，不查上游，避免泄露其他实例/上游的结果。
		writeVisualValidateResult(w, provider, "")
		return
	}
	// 跨租户隔离：会话必须属于当前 API key 用户，否则视为未完成（不泄露 group_id、不查上游）。
	userID, _ := r.Context().Value(ctxUserID).(int)
	if session.UserId != userID {
		writeVisualValidateResult(w, provider, "")
		return
	}
	provider = session.Provider
	// 本地已结算：直接返回，不查上游。
	if session.GroupId != "" && session.Status == model.VisualSessionActive {
		writeVisualValidateResult(w, provider, session.GroupId)
		return
	}

	// 查上游（会话的 provider）。
	groupId, err := GetVisualValidateResult(r.Context(), provider, req.BytedToken, session.ProjectName)
	if err != nil {
		var arkErr *ArkError
		// 认证未完成：返回 200 + 空 GroupId（客户端继续轮询）。
		// 火山返回 ValidatePending（认证中）或 NotFound（未认证/未找到）；tokenhub 返回 500 InternalError。
		// 有本地会话即视为认证未完成（InternalError 仅限 tokenhub，避免掩盖火山真实故障）。
		if errors.As(err, &arkErr) && (arkErr.Code == "ValidatePending" ||
			arkErr.Code == "NotFound" ||
			(arkErr.Code == "InternalError" && provider == material_setting.ProviderTokenhub)) {
			writeVisualValidateResult(w, provider, "")
			return
		}
		logger.LogError(r.Context(), "query upstream visual validate result failed: "+err.Error())
		http.Error(w, "query visual validate result failed", http.StatusBadGateway)
		return
	}

	// 回填数据库：会话 group_id/status + 归属（幂等，用会话的 user/name/provider）。
	if groupId != "" && session != nil {
		_ = model.CompleteVisualValidateSession(req.BytedToken, groupId, model.VisualSessionActive)
		if err := model.AddUserMaterialGroup(session.UserId, session.Provider, groupId, session.Name, "LivenessFace"); err != nil {
			logger.LogError(r.Context(), "record liveness material group failed: "+err.Error())
		}
	}
	writeVisualValidateResult(w, provider, groupId)
}

// writeVisualValidateResult 按 provider 格式写查询结果：火山信封 / tokenhub 直接。
func writeVisualValidateResult(w http.ResponseWriter, provider, groupId string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buildVisualValidateResult(provider, groupId))
}

// buildVisualValidateResult 构造与上游一致的结果格式：
// 火山带 ResponseMetadata/Result 信封；tokenhub 直接返回 {"GroupId": "..."}。
func buildVisualValidateResult(_ string, groupId string) []byte {
	// 统一返回 tokenhub 直接格式 {"GroupId": ...}（不分渠道），客户端只解析一种格式。
	data, _ := common.Marshal(map[string]any{"GroupId": groupId})
	return data
}

// groupOwnedByCurrentUser 校验组操作（Get/Update/DeleteAssetGroup）请求体 Id 属于当前 API key 用户
// （防 IDOR）。无 Id 时放行，交上游报 InvalidParameter。
func groupOwnedByCurrentUser(r *http.Request, body []byte) bool {
	var req struct {
		Id string `json:"Id"`
	}
	_ = common.Unmarshal(body, &req)
	if req.Id == "" {
		return true
	}
	userID, _ := r.Context().Value(ctxUserID).(int)
	owned, _ := model.UserMaterialGroupExists(userID, material_setting.GetConfig().Provider, req.Id)
	return owned
}

// groupsOwnedByCurrentUser 校验 ListAssets 的 Filter.GroupIds 全部属于当前 API key 用户。
func groupsOwnedByCurrentUser(r *http.Request, body []byte) bool {
	var req struct {
		Filter struct {
			GroupIds []string `json:"GroupIds"`
		} `json:"Filter"`
	}
	_ = common.Unmarshal(body, &req)
	if len(req.Filter.GroupIds) == 0 {
		return true
	}
	userID, _ := r.Context().Value(ctxUserID).(int)
	provider := material_setting.GetConfig().Provider
	for _, gid := range req.Filter.GroupIds {
		owned, _ := model.UserMaterialGroupExists(userID, provider, gid)
		if !owned {
			return false
		}
	}
	return true
}

// listAssetsGroupsEmpty 判断 ListAssets 的 Filter.GroupIds 是否为空（空则上游会列出全部素材，须拒绝）。
func listAssetsGroupsEmpty(r *http.Request, body []byte) bool {
	var req struct {
		Filter struct {
			GroupIds []string `json:"GroupIds"`
		} `json:"Filter"`
	}
	_ = common.Unmarshal(body, &req)
	return len(req.Filter.GroupIds) == 0
}

// createAssetGroupOwned 校验上传素材（CreateAsset）的目标组归属，防止向他人组注入素材。
func createAssetGroupOwned(r *http.Request, body []byte) bool {
	var req struct {
		GroupId string `json:"GroupId"`
	}
	_ = common.Unmarshal(body, &req)
	if req.GroupId == "" {
		return true // 无目标组，交上游报 InvalidParameter
	}
	userID, _ := r.Context().Value(ctxUserID).(int)
	owned, _ := model.UserMaterialGroupExists(userID, material_setting.GetConfig().Provider, req.GroupId)
	return owned
}

// writeMaterialNotFound 返回上游风格 404 错误信封（不区分不存在/无权，避免探测）。
func writeMaterialNotFound(w http.ResponseWriter) {
	envelope := map[string]any{
		"ResponseError": map[string]any{"Code": "NotFound", "Message": "resource not found"},
	}
	data, _ := common.Marshal(envelope)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(data)
}

// writeMaterialBadRequest 返回上游风格 400 错误信封（请求参数非法，如 ListAssets 空 GroupIds）。
func writeMaterialBadRequest(w http.ResponseWriter) {
	envelope := map[string]any{
		"ResponseError": map[string]any{"Code": "InvalidParameter", "Message": "invalid parameter"},
	}
	data, _ := common.Marshal(envelope)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
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
		// 真人认证会话：响应 BytedToken/H5Link 写会话表（回调反查用户 + 排查链路）。
		// 兼容两种响应格式：火山信封（Result.BytedToken）与 tokenhub 直接（顶层 BytedToken）。
		var bytedToken, h5Link string
		var enveloped struct {
			Result struct {
				BytedToken string `json:"BytedToken"`
				H5Link     string `json:"H5Link"`
			} `json:"Result"`
		}
		if common.Unmarshal(resp.Body, &enveloped) == nil && enveloped.Result.BytedToken != "" {
			bytedToken, h5Link = enveloped.Result.BytedToken, enveloped.Result.H5Link
		} else {
			var direct struct {
				BytedToken string `json:"BytedToken"`
				H5Link     string `json:"H5Link"`
			}
			if common.Unmarshal(resp.Body, &direct) == nil {
				bytedToken, h5Link = direct.BytedToken, direct.H5Link
			}
		}
		if bytedToken == "" {
			return
		}
		projectName, _ := ctx.Value(ctxProjectName).(string)
		customCallback, _ := ctx.Value(ctxCustomCallback).(string)
		name, _ := ctx.Value(ctxGroupName).(string)
		// 组名 ≤255 rune 由 createAssetGroupNameInvalid 在请求层保证（见 ADR 0007），此处不再截断。
		// 组名取请求体 Name（调用方可选传入，上游忽略但不报错），认证后用作归属组名；无则留空。
		// callbackURL 是发给上游的网关回调；customCallback 是客户自定义原回调（认证后 302 到它）。
		if err := model.AddVisualValidateSession(userID, material_setting.GetConfig().Provider, name, bytedToken, h5Link, callbackURL, projectName, customCallback); err != nil {
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
		// 组名 ≤255 rune 由 createAssetGroupNameInvalid 在请求层保证（见 ADR 0007），此处不再截断。
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
		// 真人认证会话：记录发起时的组名/回调地址与项目，供响应侧写会话表。
		// Name 由调用方可选传入（上游忽略但不报错），认证后用作归属组名。
		fields := readJSONBodyFields(c, "Name", "CallbackURL", "ProjectName")
		ctx = context.WithValue(ctx, ctxGroupName, fields["Name"])
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

// createAssetGroupNameInvalid 判断建组名称非法：含 HTML 危险字符（< >）或超过 255 rune，
// 拒绝防 XSS 存储与镜像表列宽溢出。管理端已有一致校验（controller/material_console.go）。
func createAssetGroupNameInvalid(r *http.Request, body []byte) bool {
	var req struct {
		Name string `json:"Name"`
	}
	_ = common.Unmarshal(body, &req)
	return strings.ContainsAny(req.Name, "<>") || utf8.RuneCountInString(req.Name) > 255
}

// normalizeUpstreamResponse 把上游响应统一成 tokenhub 直接格式（去掉火山 ResponseMetadata/Result 信封），
// 使对外接口不分渠道返回同一种格式：
// - 无 ResponseMetadata（tokenhub 直接）→ 原样
// - 火山错误（ResponseMetadata.Error）→ 转 tokenhub 错误格式 ResponseError
// - 火山成功（Result）→ 直接返回 Result 内容
func normalizeUpstreamResponse(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	meta, ok := m["ResponseMetadata"].(map[string]any)
	if !ok {
		return body // 已是 tokenhub 直接格式
	}
	if e, ok := meta["Error"].(map[string]any); ok {
		data, _ := common.Marshal(map[string]any{"ResponseError": e})
		return data
	}
	if result, ok := m["Result"].(map[string]any); ok {
		data, _ := common.Marshal(result)
		return data
	}
	// ResponseMetadata 存在但无 Error/Result（如空 {} + 顶层 ResponseError）：剥掉信封残留，保留其余字段。
	delete(m, "ResponseMetadata")
	data, _ := common.Marshal(m)
	return data
}
