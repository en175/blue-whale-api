package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	materiallib "github.com/QuantumNous/new-api/service/material_library"
	"github.com/QuantumNous/new-api/setting/material_setting"

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
// admin 放行；普通用户必须拥有该组的归属记录，否则拒绝。
func userOwnsMaterialGroup(c *gin.Context, groupID string) bool {
	if c.GetInt("role") >= common.RoleAdminUser {
		return true
	}
	owned, err := model.UserMaterialGroupExists(c.GetInt("id"), groupID)
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
// 查询参数 type=AIGC|LivenessFace 可选，用于 tab 切换过滤。
func GetMaterialGroups(c *gin.Context) {
	userID := c.GetInt("id")
	role := c.GetInt("role")
	queryUserID := userID
	if role >= common.RoleAdminUser {
		queryUserID = 0 // admin 不过滤用户
	}

	groups, err := model.GetMaterialGroups(queryUserID, c.Query("type"))
	if err != nil {
		logger.LogError(c.Request.Context(), "query material groups failed: "+err.Error())
		common.ApiErrorMsg(c, "query material groups failed")
		return
	}
	common.ApiSuccess(c, groups)
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

	groupID, err := materiallib.CreateAssetGroup(c.Request.Context(), req.Name, req.Description, req.GroupType)
	if err != nil {
		logger.LogError(c.Request.Context(), "create material group failed: "+err.Error())
		common.ApiErrorMsg(c, "create material group failed: "+err.Error())
		return
	}
	if err := model.AddUserMaterialGroup(c.GetInt("id"), groupID, req.Name, req.GroupType); err != nil {
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
	if err := model.DeleteUserMaterialGroup(groupID); err != nil {
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

	result, err := materiallib.ListAssets(c.Request.Context(), []string{groupID}, page, pageSize, c.Query("name"))
	if err != nil {
		logger.LogError(c.Request.Context(), "list material assets failed: "+err.Error())
		common.ApiErrorMsg(c, "list material assets failed: "+err.Error())
		return
	}
	common.ApiSuccess(c, result)
}
