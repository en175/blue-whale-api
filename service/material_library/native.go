package material_library

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
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
// 归一成火山官方信封、ListAssetGroups 直接查上游、
// GetVisualValidateResult 不读本地缓存、名称不做过滤。共用的只有 provider、
// 凭证与 user_material_groups 归属表。
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
	resp, err := currentProvider().DoAction(ctx, action, request.URL.Query(), body)
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
		if !groupOwnedByCurrentUser(r, body) {
			return 404, "NotFound", "resource not found", false
		}
	case "CreateAsset":
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
		if !assetOwnedByCurrentUser(r, body) {
			return 404, "NotFound", "resource not found", false
		}
	}
	return 0, "", "", true
}

// assetOwnedByCurrentUser 校验单素材操作（Update/DeleteAsset）的目标组归属。
//
// 上游素材对象不携带网关侧归属，只能先 GetAsset 反查 GroupId，再复用组归属
// 校验。查询失败或上游不返回 GroupId 时按无权处理（fail-closed），宁可拒绝也
// 不放行越权删除。Id 缺失时放行，交上游报 InvalidParameter。
func assetOwnedByCurrentUser(r *http.Request, body []byte) bool {
	assetID := jsonStringFields(body, "Id")["Id"]
	if assetID == "" {
		return true
	}
	probe, err := common.Marshal(map[string]string{"Id": assetID})
	if err != nil {
		return false
	}
	resp, err := currentProvider().DoAction(r.Context(), "GetAsset", nil, probe)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	var parsed struct {
		Result struct {
			GroupId string `json:"GroupId"`
		} `json:"Result"`
		GroupId string `json:"GroupId"`
	}
	if err := common.Unmarshal(resp.Body, &parsed); err != nil {
		return false
	}
	groupID := parsed.Result.GroupId
	if groupID == "" {
		groupID = parsed.GroupId
	}
	if groupID == "" {
		return false
	}
	userID, _ := r.Context().Value(ctxUserID).(int)
	owned, _ := model.UserMaterialGroupExists(userID, material_setting.GetConfig().Provider, groupID)
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
	if err := common.Unmarshal(body, &m); err != nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(fields))
	for _, field := range fields {
		if v, ok := m[field].(string); ok {
			result[field] = v
		}
	}
	return result
}
