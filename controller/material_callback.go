package controller

import (
	"html"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	materiallib "github.com/QuantumNous/new-api/service/material_library"

	"github.com/gin-gonic/gin"
)

// visualValidateResultOK 是 H5 活体认证成功回调的 resultCode（火山兼容接口约定）。
const visualValidateResultOK = "10000"

// MaterialVisualValidateCallback 处理真人素材 H5 活体认证完成后的浏览器回调。
// 无登录态（浏览器跳回），靠 bytedToken 反查会话拿用户；认证成功后轮询
// GetVisualValidateResult 拿真人素材组 id，并记录归属（provider + LivenessFace），
// 使管理端"真人素材" tab 出现该组。
// 返回一个可关闭的 HTML 提示页。
func MaterialVisualValidateCallback(c *gin.Context) {
	bytedToken := c.Query("bytedToken")
	resultCode := c.Query("resultCode")
	if bytedToken == "" {
		renderVisualCallbackPage(c, "参数缺失")
		return
	}
	if resultCode != visualValidateResultOK {
		// 认证未成功（用户取消/失败），标记会话失败，不阻塞。
		_ = model.CompleteVisualValidateSession(bytedToken, "", model.VisualSessionFailed)
		renderVisualCallbackPage(c, "认证未完成")
		return
	}

	session, err := model.GetVisualValidateSessionByToken(bytedToken)
	if err != nil {
		// 会话不存在（未通过本网关发起的认证），忽略并提示。
		logger.LogWarn(c.Request.Context(), "visual validate session not found: "+bytedToken)
		renderVisualCallbackPage(c, "会话无效")
		return
	}

	// 幂等/去重守卫：已完成或已有在途轮询的会话不重复开轮询。
	switch session.Status {
	case model.VisualSessionActive:
		renderVisualCallbackPage(c, "真人素材已就绪")
		return
	case model.VisualSessionPolling:
		renderVisualCallbackPage(c, "认证处理中")
		return
	}
	if !model.ClaimVisualValidateSessionPolling(bytedToken) {
		renderVisualCallbackPage(c, "认证处理中")
		return
	}

	// 用会话记录的 provider 轮询，避免全局配置切换后走错上游。
	ctx := c.Request.Context()
	groupId, err := materiallib.WaitVisualValidateGroupId(ctx, session.Provider, bytedToken, 60*time.Second, 3*time.Second)
	if err != nil {
		_ = model.CompleteVisualValidateSession(bytedToken, "", model.VisualSessionFailed)
		logger.LogError(ctx, "get visual validate group id failed: "+err.Error())
		renderVisualCallbackPage(c, "获取素材组失败")
		return
	}

	// 归属记录用用户起的真人组名（会话表 Name），而非上游自动生成的组 id。
	if err := model.AddUserMaterialGroup(session.UserId, session.Provider, groupId, session.Name, "LivenessFace"); err != nil {
		logger.LogError(ctx, "record liveness material group failed: "+err.Error())
	}
	if err := model.CompleteVisualValidateSession(bytedToken, groupId, model.VisualSessionActive); err != nil {
		logger.LogError(ctx, "complete visual validate session failed: "+err.Error())
	}
	renderVisualCallbackPage(c, "真人素材已就绪")
}

// renderVisualCallbackPage 返回 H5 认证结果的简单 HTML 提示页（浏览器展示，用户可关闭）。
func renderVisualCallbackPage(c *gin.Context, msg string) {
	page := "<!doctype html><html><head><meta charset='utf-8'><title>素材认证</title></head>" +
		"<body style='font-family:sans-serif;text-align:center;padding-top:80px'>" +
		"<div style='font-size:20px'>" + html.EscapeString(msg) + "</div>" +
		"<div style='color:#888;margin-top:12px'>此页面可关闭</div></body></html>"
	c.Data(200, "text/html; charset=utf-8", []byte(page))
}
