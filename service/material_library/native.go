package material_library

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/gin-gonic/gin"
)

// HandleNativeRequest 是火山管控面形状的素材库入口：
//
//	POST /?Action=CreateAssetGroup&Version=2024-01-01
//	POST /api/ark?Action=CreateAssetGroup&Version=2024-01-01
//
// 与 /api/material/{Action} 的差异（ADR 0024）：Action/Version 走 query、响应
// 归一成火山官方信封、GetVisualValidateResult 不读本地缓存、名称不做过滤。
// 归属隔离与路径入口一致：ListAssetGroups 查本地归属表，素材读取按 API key 校验。
// 共用的只有 provider、凭证与 user_material_groups 归属表。
//
// Version 不校验、原样透传给上游；下游没传时由 provider 补默认值。上游拒绝时
// 原样返回官方错误码，网关不改写。
func HandleNativeRequest(c *gin.Context) {
	action := strings.TrimSpace(c.Query("Action"))
	if action == "" {
		writeNativeError(c, 400, "InvalidParameter", "Action is required")
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeNativeError(c, 400, "InvalidRequest", "read request body failed")
		return
	}

	ctx := context.WithValue(c.Request.Context(), ctxUserID, c.GetInt("id"))
	request := c.Request.WithContext(ctx)

	// 组列表必须按当前 API key 的归属返回：上游是共享凭证，直接透传会把其他
	// 租户的组一起列出。与路径入口同源，查本地归属表。
	if action == "ListAssetGroups" {
		data, err := buildLocalAssetGroupsResponse(request, body, nativeVersion(c), c.GetString(common.RequestIdKey))
		if err != nil {
			logger.LogError(ctx, "list local asset groups failed: "+err.Error())
			writeNativeError(c, 500, "InternalError", "list asset groups failed")
			return
		}
		c.Data(http.StatusOK, "application/json", data)
		return
	}

	// 认证结果必须属于当前 API key：路径入口同样先校验本地会话，避免拿着他人
	// BytedToken 直查上游拿到 GroupId。拒绝时不区分「不存在」与「无权」。
	var visualSession *model.UserMaterialVisualSession
	if action == "GetVisualValidateResult" {
		var ok bool
		visualSession, ok = loadVisualSessionForCaller(request, body)
		if !ok {
			writeNativeVisualResult(c, action)
			return
		}
		// 火山契约：建会话时设了 ProjectName，查询必须传同值。客户端漏传或传错
		// 时以会话记录为准，与路径入口一致。
		if visualSession != nil && visualSession.ProjectName != "" {
			body = withProjectName(body, visualSession.ProjectName)
		}
	}

	// GetAsset 要先经上游反查 GroupId 才能校验归属；探测响应就是素材详情，
	// 直接复用，避免同一素材查两次上游。
	if action == "GetAsset" {
		if _, valid := nativeStringField(body, "Id"); !valid {
			writeNativeError(c, 400, "InvalidParameter", "Id must be a string")
			return
		}
		probe, owned := probeAssetOwnership(ctx, body)
		if !owned {
			writeNativeError(c, 404, "NotFound", "resource not found")
			return
		}
		if probe != nil {
			normalizeNativeBody(probe, action, c)
			writeNativeResponse(c, probe)
			return
		}
		// Id 为空：交给上游报参数错误。
	}

	// 真人会话：客户自定义回调 → 替换为网关回调（网关要拿到认证结果），认证后
	// 再 302 回客户。归属记录里同时保留客户原值，供回调查询时还原。
	var customCallback string
	if action == "CreateVisualValidateSession" {
		clientCallback := jsonStringFields(body, "CallbackURL")["CallbackURL"]
		body, customCallback = rewriteVisualCallbackBody(request, body)
		ctx = context.WithValue(ctx, ctxCallbackURL, clientCallback)
		if customCallback != "" {
			ctx = context.WithValue(ctx, ctxCustomCallback, customCallback)
			ctx = context.WithValue(ctx, ctxCallbackURL, gatewayCallbackURL(request))
		}
	}

	ctx = attachOwnershipFields(ctx, action, body)
	request = request.WithContext(ctx)

	if status, code, message, ok := checkNativeOwnership(request, action, body); !ok {
		writeNativeError(c, status, code, message)
		return
	}

	// 下游 query 全部透传（含 Version 与业务方自定义参数），provider 只补 Action。
	// 认证结果按会话记录的 provider 查询，避免全局配置切换后查错上游。
	provider := currentProvider()
	if visualSession != nil && visualSession.Provider != "" {
		provider = providerByName(visualSession.Provider)
	}
	resp, err := provider.DoAction(ctx, action, request.URL.Query(), body)
	if err != nil {
		// 不把 Go 内部错误细节返回给客户端，细节进带 trace_id 的后端日志。
		logger.LogError(ctx, "material library upstream error: "+err.Error())
		writeNativeError(c, 502, "InternalError", "material library upstream error")
		return
	}
	observeOwnership(ctx, resp)
	if customCallback != "" {
		resp.Body = restoreClientCallback(resp.Body, customCallback)
	}
	normalizeNativeBody(resp, action, c)
	writeNativeResponse(c, resp)
}

// normalizeNativeBody 把上游响应归一成火山官方形状。对客承诺是与火山一致，但
// 华鲲兼容面有两处偏差：
//
//   - 错误用顶层 ResponseError，火山用 ResponseMetadata.Error；
//   - 会话类接口不包信封，火山包 ResponseMetadata / Result。
//
// volcengine 直连本来就是火山形状，两处判断都不会命中，函数是空操作。
func normalizeNativeBody(resp *Response, action string, c *gin.Context) {
	// 逐层保留原始 JSON 片段，避免反序列化再序列化时大整数丢精度。
	var top map[string]json.RawMessage
	if err := common.Unmarshal(resp.Body, &top); err != nil || top == nil {
		return // 非 JSON 对象原样透传，由下游按状态码处理
	}
	if rawError, ok := top["ResponseError"]; ok {
		var meta map[string]json.RawMessage
		if rawMeta, ok := top["ResponseMetadata"]; ok {
			_ = common.Unmarshal(rawMeta, &meta)
		}
		if meta == nil {
			meta = map[string]json.RawMessage{}
		}
		meta["Error"] = rawError
		top["ResponseMetadata"], _ = common.Marshal(meta)
		delete(top, "ResponseError")
	}
	// 只在成功响应上补信封：会话类失败时上游仍走通用错误格式。
	if _, enveloped := top["ResponseMetadata"]; !enveloped &&
		resp.StatusCode >= 200 && resp.StatusCode < 300 && isBareSessionAction(action) {
		top["Result"] = json.RawMessage(resp.Body)
		top["ResponseMetadata"], _ = common.Marshal(map[string]string{
			"RequestId": c.GetString(common.RequestIdKey),
			"Action":    action,
			"Version":   nativeVersion(c),
			"Service":   "ark",
			"Region":    material_setting.GetConfig().VolcengineRegion,
		})
	}
	normalized, err := common.Marshal(top)
	if err != nil {
		return
	}
	resp.Body = normalized
}

// isBareSessionAction 判断上游是否对该 Action 返回无信封的裸字段。
func isBareSessionAction(action string) bool {
	return action == "CreateVisualValidateSession" || action == "GetVisualValidateResult"
}

// nativeVersion 取下游传入的 Version，缺省时用网关默认值——与请求侧补值规则一致。
func nativeVersion(c *gin.Context) string {
	if version := c.Query("Version"); version != "" {
		return version
	}
	return arkVersion
}

// attachOwnershipFields 提取 observeOwnership 需要的归属字段。原生入口的 Action
// 来自 query，不能复用按路径分派的 RecordContext。
func attachOwnershipFields(ctx context.Context, action string, body []byte) context.Context {
	switch action {
	case "CreateAssetGroup":
		fields := jsonStringFields(body, "GroupType", "Name")
		ctx = context.WithValue(ctx, ctxGroupType, fields["GroupType"])
		ctx = context.WithValue(ctx, ctxGroupName, fields["Name"])
	case "CreateVisualValidateSession":
		fields := jsonStringFields(body, "Name", "ProjectName")
		ctx = context.WithValue(ctx, ctxGroupName, fields["Name"])
		ctx = context.WithValue(ctx, ctxProjectName, fields["ProjectName"])
	case "DeleteAssetGroup":
		ctx = context.WithValue(ctx, ctxGroupID, jsonStringFields(body, "Id")["Id"])
	}
	return ctx
}

// checkNativeOwnership 按 Action 做归属校验：表内动作校验通过才透传，表外动作
// 直接放行。拒绝一律返回 404/400，且不泄露资源是否存在。
func checkNativeOwnership(r *http.Request, action string, body []byte) (status int, code string, message string, ok bool) {
	switch action {
	case "GetAssetGroup", "UpdateAssetGroup", "DeleteAssetGroup":
		if _, valid := nativeStringField(body, "Id"); !valid {
			return 400, "InvalidParameter", "Id must be a string", false
		}
		if !groupOwnedByCurrentUser(r, body) {
			return 404, "NotFound", "resource not found", false
		}
	case "CreateAsset":
		if _, valid := nativeStringField(body, "GroupId"); !valid {
			return 400, "InvalidParameter", "GroupId must be a string", false
		}
		if !createAssetGroupOwned(r, body) {
			return 404, "NotFound", "resource not found", false
		}
	case "ListAssets":
		// 空 GroupIds 会列出共享账号下全部素材，属越权，直接拒绝。
		if listAssetsGroupsEmpty(r, body) {
			return 400, "InvalidParameter", "Filter.GroupIds is required", false
		}
		if !groupsOwnedByCurrentUser(r, body) {
			return 404, "NotFound", "resource not found", false
		}
	case "UpdateAsset", "DeleteAsset":
		if _, valid := nativeStringField(body, "Id"); !valid {
			return 400, "InvalidParameter", "Id must be a string", false
		}
		if !assetOwnedByCurrentUser(r, body) {
			return 404, "NotFound", "resource not found", false
		}
	}
	return 0, "", "", true
}

// nativeStringField 读取请求体中的字符串字段。valid=false 表示请求体不是 JSON
// 对象、字段存在但不是字符串，或同名键大小写冲突——归属校验据此 fail-closed。
// 字段缺失（含 null）时返回 ("", true)，由调用方区分「可选参数缺省」与「格式非法」。
func nativeStringField(body []byte, field string) (value string, valid bool) {
	return jsonStringFieldFold(body, field)
}

// probeAssetOwnership 经上游 GetAsset 反查素材所属组并校验归属，返回探测响应
// 供 GetAsset 直接复用。Id 缺失时返回 (nil, true)，交上游报参数错误；查询失败、
// 无 GroupId 或非本人组返回 (nil, false)，避免越权读取/改名/删除。
func probeAssetOwnership(ctx context.Context, body []byte) (*Response, bool) {
	assetID, valid := jsonStringFieldFold(body, "Id")
	if !valid {
		return nil, false
	}
	if assetID == "" {
		return nil, true
	}
	probe, err := common.Marshal(map[string]string{"Id": assetID})
	if err != nil {
		return nil, false
	}
	resp, err := currentProvider().DoAction(ctx, "GetAsset", nil, probe)
	if err != nil {
		logger.LogError(ctx, "material library get asset ownership probe failed: "+err.Error())
		return nil, false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode >= 500 {
			logger.LogWarn(ctx, "material library get asset ownership probe failed: upstream status "+strconv.Itoa(resp.StatusCode))
		}
		return nil, false
	}
	var parsed struct {
		Result struct {
			GroupId string `json:"GroupId"`
		} `json:"Result"`
		GroupId string `json:"GroupId"`
	}
	if err := common.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, false
	}
	groupID := parsed.Result.GroupId
	if groupID == "" {
		groupID = parsed.GroupId
	}
	if groupID == "" {
		return nil, false
	}
	userID, _ := ctx.Value(ctxUserID).(int)
	if userID <= 0 {
		return nil, false
	}
	owned, _ := model.UserMaterialGroupExists(userID, material_setting.GetConfig().Provider, groupID)
	if !owned {
		return nil, false
	}
	return resp, true
}

// assetOwnedByCurrentUser 校验单素材写操作（Update/DeleteAsset）的目标组归属。
func assetOwnedByCurrentUser(r *http.Request, body []byte) bool {
	_, owned := probeAssetOwnership(r.Context(), body)
	return owned
}

// writeNativeResponse 原样回写上游状态码、头与体（火山信封不做归一化）。
func writeNativeResponse(c *gin.Context, resp *Response) {
	for key, values := range resp.Header {
		if key == "Content-Length" {
			continue // 响应体可能已改写，旧长度失效，交由 Go 按实际长度重算
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(resp.Body)
}

// loadVisualSessionForCaller 加载 BytedToken 对应的会话并校验归属。返回
// (nil, true) 表示 token 为空，交上游报参数错误；格式非法、会话不存在或不属于
// 当前用户时返回 (nil, false)，调用方按无权处理。
func loadVisualSessionForCaller(r *http.Request, body []byte) (*model.UserMaterialVisualSession, bool) {
	token, valid := jsonStringFieldFold(body, "BytedToken")
	if !valid {
		return nil, false
	}
	if token == "" {
		return nil, true
	}
	session, err := model.GetVisualValidateSessionByToken(token)
	if err != nil || session == nil {
		return nil, false
	}
	userID, _ := r.Context().Value(ctxUserID).(int)
	if session.UserId != userID {
		return nil, false
	}
	return session, true
}

// withProjectName 把会话创建时的 ProjectName 补进查询请求体。火山契约要求查询与
// 建会话传同一个值（区分大小写）；客户端漏传或传错时以会话记录为准。
func withProjectName(body []byte, projectName string) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil || m == nil {
		return body
	}
	matches := 0
	same := true
	for key, raw := range m {
		if !strings.EqualFold(key, "ProjectName") {
			continue
		}
		matches++
		if s, _ := raw.(string); s != projectName {
			same = false
		}
	}
	if matches == 1 && same {
		return body
	}
	for key := range m {
		if strings.EqualFold(key, "ProjectName") {
			delete(m, key)
		}
	}
	m["ProjectName"] = projectName
	rewritten, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return rewritten
}

// writeNativeVisualResult 在归属校验失败时返回空 GroupId（火山信封），与路径入口
// 语义一致：不区分会话不存在与不属于当前用户，避免探测。
func writeNativeVisualResult(c *gin.Context, action string) {
	c.JSON(http.StatusOK, map[string]any{
		"ResponseMetadata": map[string]any{
			"RequestId": c.GetString(common.RequestIdKey),
			"Action":    action,
			"Version":   nativeVersion(c),
			"Service":   "ark",
			"Region":    material_setting.GetConfig().VolcengineRegion,
		},
		"Result": map[string]string{"GroupId": ""},
	})
}

// writeNativeError 构造火山管控面错误信封。
func writeNativeError(c *gin.Context, statusCode int, code string, message string) {
	c.JSON(statusCode, map[string]any{
		"ResponseMetadata": map[string]any{
			"Error":     map[string]any{"Code": code, "Message": message},
			"RequestId": c.GetString(common.RequestIdKey),
		},
	})
}

// jsonStringFields 从请求体里取指定字符串字段，解析失败时返回空 map（调用方按
// 缺省处理）。body 不被消费，可继续原样发给上游。
func jsonStringFields(body []byte, fields ...string) map[string]string {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil || m == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(fields))
	for _, field := range fields {
		if v, ok := foldStringField(m, field); ok && v != "" {
			result[field] = v
		}
	}
	return result
}

// jsonStringFieldFold 按大小写不敏感读取请求体中的字符串字段。上游 Go 服务解码
// JSON 时键名大小写不敏感；归属校验必须同口径，否则 {"id":...} 会绕过按精确键名
// 的读取，直接被透传到共享上游。解析失败、类型不符或同名字段大小写冲突时返回
// ("", false)，调用方按 fail-closed 处理。
func jsonStringFieldFold(body []byte, field string) (string, bool) {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil || m == nil {
		return "", false
	}
	return foldStringField(m, field)
}

// foldStringField 在已解析的对象里按大小写不敏感取字符串字段。
func foldStringField(fields map[string]any, field string) (string, bool) {
	value, found, conflict := "", false, false
	for key, raw := range fields {
		if !strings.EqualFold(key, field) {
			continue
		}
		if raw == nil {
			raw = ""
		}
		s, ok := raw.(string)
		if !ok {
			return "", false
		}
		if found && s != value {
			conflict = true
			continue
		}
		value, found = s, true
	}
	if conflict {
		return "", false
	}
	return value, true
}
