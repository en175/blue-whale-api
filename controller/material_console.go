package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	materiallib "github.com/QuantumNous/new-api/service/material_library"
	"github.com/QuantumNous/new-api/setting/material_setting"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// RequireMaterialLibraryEnabled 中间件：素材库未启用/凭证不全时拒绝管理界面请求，
// 与 API 代理的 IsReady 守卫保持一致（"禁用即禁用"）。
func RequireMaterialLibraryEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !material_setting.GetConfig().IsReady() {
			logger.LogWarn(c.Request.Context(), "material console called but not ready")
			common.ApiErrorMsg(c, "material library is not configured")
			c.Abort()
			return
		}
		c.Next()
	}
}

// userOwnsMaterialGroup 校验当前用户是否有权操作该素材组（数据隔离）：
// admin 放行；普通用户必须拥有该组在当前 provider 下的归属记录，否则拒绝。
func userOwnsMaterialGroup(c *gin.Context, groupID string) bool {
	if c.GetInt("role") >= common.RoleAdminUser {
		return true
	}
	provider := material_setting.GetConfig().Provider
	owned, err := model.UserMaterialGroupExists(c.GetInt("id"), provider, groupID)
	if err != nil {
		logger.LogError(c.Request.Context(), "check material group ownership failed: "+err.Error())
		return false
	}
	return owned
}

// denyMaterialGroupAccess 拒绝无归属用户的组操作并返回统一提示。
func denyMaterialGroupAccess(c *gin.Context) {
	common.ApiErrorMsg(c, "material group not found or not owned")
}

// GetMaterialGroups 管理界面：查询当前用户有权限的素材组。
// 数据源为本地归属表（user_material_groups）：普通用户按 user_id 隔离，admin 看全部。
// 查询参数 type=AIGC|LivenessFace 可选，用于 tab 切换过滤；name 按组名/组 id 模糊匹配；
// p/page_size 分页（默认 1/20，page_size 上限 100，page 上限 maxMaterialGroupPage 防 offset 溢出）。
const maxMaterialGroupPage = 100000

func GetMaterialGroups(c *gin.Context) {
	userID := c.GetInt("id")
	role := c.GetInt("role")
	queryUserID := userID
	if role >= common.RoleAdminUser {
		queryUserID = 0 // admin 不过滤用户
	}

	page, pageSize := 1, 20
	if v, err := strconv.Atoi(c.Query("p")); err == nil && v >= 1 {
		page = min(v, maxMaterialGroupPage) // 超上限钳到 100000，防 offset 溢出
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v >= 1 {
		pageSize = min(v, 100)
	}

	// 仅展示当前 provider 下的归属（切换上游后旧组不显示，互不干扰）。
	total, groups, err := model.GetMaterialGroupsPaged(queryUserID, material_setting.GetConfig().Provider, c.Query("type"), c.Query("name"), page, pageSize)
	if err != nil {
		logger.LogError(c.Request.Context(), "query material groups failed: "+err.Error())
		common.ApiErrorMsg(c, "query material groups failed")
		return
	}
	common.ApiSuccess(c, gin.H{
		"total_count": total,
		"items":       groups,
		"page_number": page,
		"page_size":   pageSize,
	})
}

// CreateMaterialGroup 管理界面：创建素材组（上游 CreateAssetGroup），并把归属记录到当前用户。
func CreateMaterialGroup(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		GroupType   string `json:"group_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request body")
		return
	}
	if req.Name == "" || req.GroupType == "" {
		common.ApiErrorMsg(c, "name and group_type are required")
		return
	}
	if utf8.RuneCountInString(req.Name) > 255 {
		common.ApiErrorMsg(c, "name is too long")
		return
	}
	if materiallib.MaterialNameInvalid(req.Name) {
		common.ApiErrorMsg(c, "name contains invalid characters")
		return
	}

	groupID, err := materiallib.CreateAssetGroup(c.Request.Context(), req.Name, req.Description, req.GroupType)
	if err != nil {
		logger.LogError(c.Request.Context(), "create material group failed: "+err.Error())
		common.ApiErrorMsg(c, "create material group failed: "+err.Error())
		return
	}
	if err := model.AddUserMaterialGroup(c.GetInt("id"), material_setting.GetConfig().Provider, groupID, req.Name, req.GroupType); err != nil {
		logger.LogError(c.Request.Context(), "record material group failed: "+err.Error())
	}
	common.ApiSuccess(c, gin.H{"id": groupID})
}

// DeleteMaterialGroup 管理界面：删除素材组（上游 DeleteAssetGroup），并清理本地归属。
// 非 admin 用户只能删除自己拥有的组。
func DeleteMaterialGroup(c *gin.Context) {
	groupID := c.Param("id")
	if !userOwnsMaterialGroup(c, groupID) {
		denyMaterialGroupAccess(c)
		return
	}
	if err := materiallib.DeleteAssetGroup(c.Request.Context(), groupID); err != nil {
		logger.LogError(c.Request.Context(), "delete material group failed: "+err.Error())
		common.ApiErrorMsg(c, "delete material group failed: "+err.Error())
		return
	}
	if err := model.DeleteUserMaterialGroup(material_setting.GetConfig().Provider, groupID); err != nil {
		logger.LogError(c.Request.Context(), "clear material group record failed: "+err.Error())
	}
	common.ApiSuccess(c, nil)
}

// CreateMaterialAsset 管理界面：上传素材（上游 CreateAsset，URL 直传）。
// 非 admin 用户只能往自己拥有的组上传。
func CreateMaterialAsset(c *gin.Context) {
	groupID := c.Param("id")
	if !userOwnsMaterialGroup(c, groupID) {
		denyMaterialGroupAccess(c)
		return
	}
	var req struct {
		URL       string `json:"url"`
		Name      string `json:"name"`
		AssetType string `json:"asset_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request body")
		return
	}
	if req.URL == "" || req.Name == "" || req.AssetType == "" {
		common.ApiErrorMsg(c, "url, name and asset_type are required")
		return
	}
	if utf8.RuneCountInString(req.Name) > 255 {
		common.ApiErrorMsg(c, "name is too long")
		return
	}
	if materiallib.MaterialNameInvalid(req.Name) {
		common.ApiErrorMsg(c, "name contains invalid characters")
		return
	}

	assetID, err := materiallib.CreateAsset(c.Request.Context(), groupID, req.URL, req.Name, req.AssetType)
	if err != nil {
		logger.LogError(c.Request.Context(), "create material asset failed: "+err.Error())
		common.ApiErrorMsg(c, "create material asset failed: "+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"id": assetID})
}

// UpdateMaterialAsset 管理界面：编辑素材（上游 UpdateAsset，改名）。
// 非 admin 用户只能改自己拥有的组里的素材。
func UpdateMaterialAsset(c *gin.Context) {
	groupID := c.Param("id")
	if !userOwnsMaterialGroup(c, groupID) {
		denyMaterialGroupAccess(c)
		return
	}
	assetID := c.Param("assetId")
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		common.ApiErrorMsg(c, "name is required")
		return
	}
	if utf8.RuneCountInString(req.Name) > 255 {
		common.ApiErrorMsg(c, "name is too long")
		return
	}
	if materiallib.MaterialNameInvalid(req.Name) {
		common.ApiErrorMsg(c, "name contains invalid characters")
		return
	}

	if err := materiallib.UpdateAsset(c.Request.Context(), assetID, req.Name); err != nil {
		logger.LogError(c.Request.Context(), "update material asset failed: "+err.Error())
		common.ApiErrorMsg(c, "update material asset failed: "+err.Error())
		return
	}
	common.ApiSuccess(c, nil)
}

// DeleteMaterialAsset 管理界面：删除素材（上游 DeleteAsset）。
// 非 admin 用户只能删自己拥有的组里的素材。
func DeleteMaterialAsset(c *gin.Context) {
	groupID := c.Param("id")
	if !userOwnsMaterialGroup(c, groupID) {
		denyMaterialGroupAccess(c)
		return
	}
	assetID := c.Param("assetId")
	if err := materiallib.DeleteAsset(c.Request.Context(), assetID); err != nil {
		logger.LogError(c.Request.Context(), "delete material asset failed: "+err.Error())
		common.ApiErrorMsg(c, "delete material asset failed: "+err.Error())
		return
	}
	common.ApiSuccess(c, nil)
}

// ListMaterialGroupAssets 管理界面：按素材组 id 查询上游素材（ListAssets 直连）。
// 非 admin 用户只能查自己拥有的组的素材。
// 分页参数 p / page_size，透传给上游。
func ListMaterialGroupAssets(c *gin.Context) {
	groupID := c.Param("id")
	if groupID == "" {
		common.ApiErrorMsg(c, "group id is required")
		return
	}
	if !userOwnsMaterialGroup(c, groupID) {
		denyMaterialGroupAccess(c)
		return
	}

	page := 1
	if v, err := strconv.Atoi(c.Query("p")); err == nil && v >= 1 {
		page = v
	}
	pageSize := 20
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v >= 1 && v <= 100 {
		pageSize = v
	}

	// 组类型从归属表取，透传给上游 Filter.GroupType（火山管控面 ListAssets 必填）。
	provider := material_setting.GetConfig().Provider
	result, err := materiallib.ListAssets(c.Request.Context(), []string{groupID}, model.GetMaterialGroupType(provider, groupID), page, pageSize, c.Query("name"))
	if err != nil {
		logger.LogError(c.Request.Context(), "list material assets failed: "+err.Error())
		common.ApiErrorMsg(c, "list material assets failed: "+err.Error())
		return
	}
	common.ApiSuccess(c, result)
}

// CreateVisualValidateSession 管理界面：发起真人素材认证会话（上游直连，内置凭证注入）。
// 回调地址优先取配置 material_library.config 的 callback_url，未配置时回退调用方传入。
// 成功后在本地记录会话（含用户起的组名，供回调反查）。走 UserAuth 管理端点，无需 API key。
func CreateVisualValidateSession(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		CallbackURL string `json:"callback_url"`
		ProjectName string `json:"project_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "invalid request body")
		return
	}
	if req.Name == "" {
		common.ApiErrorMsg(c, "name is required")
		return
	}
	if utf8.RuneCountInString(req.Name) > 255 {
		common.ApiErrorMsg(c, "name is too long")
		return
	}
	if materiallib.MaterialNameInvalid(req.Name) {
		common.ApiErrorMsg(c, "name contains invalid characters")
		return
	}
	callbackURL := material_setting.GetConfig().CallbackURL
	if callbackURL == "" {
		callbackURL = req.CallbackURL // 调用方（前端 origin）兜底
	}
	if callbackURL == "" {
		common.ApiErrorMsg(c, "callback_url is required (configure material_library.config callback_url)")
		return
	}
	bytedToken, h5Link, err := materiallib.CreateVisualValidateSession(c.Request.Context(), req.Name, callbackURL, req.ProjectName)
	if err != nil {
		logger.LogError(c.Request.Context(), "create visual validate session failed: "+err.Error())
		common.ApiErrorMsg(c, "create visual validate session failed: "+err.Error())
		return
	}
	provider := material_setting.GetConfig().Provider
	if err := model.AddVisualValidateSession(c.GetInt("id"), provider, req.Name, bytedToken, h5Link, callbackURL, req.ProjectName, ""); err != nil {
		logger.LogError(c.Request.Context(), "record visual validate session failed: "+err.Error())
	}
	common.ApiSuccess(c, gin.H{"h5_link": h5Link, "byted_token": bytedToken})
}
