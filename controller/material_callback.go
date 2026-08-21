package controller

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	materiallib "github.com/QuantumNous/new-api/service/material_library"

	"github.com/gin-gonic/gin"
)

// visualValidateResultOK 是 H5 活体认证成功回调的 resultCode（火山兼容接口约定）。
const visualValidateResultOK = "10000"

// MaterialVisualValidateCallback 处理真人素材 H5 活体认证完成后的浏览器回调。
// 无登录态（浏览器跳回），靠 bytedToken 反查会话拿用户。
// 客户自定义回调（对外 API）：无论认证成功/失败都 302 到客户（带原始 query 透传），
// 客户自行按 resultCode 处理；成功时后台异步轮询记录归属（浏览器不阻塞）。
// 管理端（无自定义回调）：同步轮询拿 GroupId 并记录归属，渲染可关闭页。
func MaterialVisualValidateCallback(c *gin.Context) {
	bytedToken := c.Query("bytedToken")
	resultCode := c.Query("resultCode")
	if bytedToken == "" {
		renderVisualCallbackPage(c, "参数缺失")
		return
	}
	session, err := model.GetVisualValidateSessionByToken(bytedToken)
	if err != nil {
		// 会话不存在（未通过本网关发起的认证），忽略并提示。
		logger.LogWarn(c.Request.Context(), "visual validate session not found: "+bytedToken)
		renderVisualCallbackPage(c, "会话无效")
		return
	}

	// 客户自定义回调（对外 API）：先加载会话再分支，保证成功/失败都 302 到客户。
	if session.CustomCallback != "" {
		if !validVisualCallbackScheme(session.CustomCallback) {
			// 非 http/https（javascript:/data: 等）开放重定向风险，拒绝 302 降级渲染页。
			logger.LogWarn(c.Request.Context(), "visual callback has invalid scheme, skip redirect: "+session.CustomCallback)
			renderVisualCallbackPage(c, "真人素材已就绪")
			return
		}
		if resultCode == visualValidateResultOK {
			// claim 成功才开始后台轮询（幂等/去重）；已在途则不重复。
			if model.ClaimVisualValidateSessionPolling(bytedToken) {
				ctx := context.WithoutCancel(c.Request.Context()) // 保留 trace_id/request_id，脱离请求取消
				go func() {
					defer func() {
						if r := recover(); r != nil {
							logger.LogWarn(ctx, "visual settle panic: "+fmt.Sprint(r))
						}
					}()
					settleVisualValidate(ctx, session.Provider, bytedToken, session)
				}()
			}
		} else {
			_ = model.CompleteVisualValidateSession(bytedToken, "", model.VisualSessionFailed)
		}
		c.Redirect(http.StatusFound, buildCustomRedirectURL(session.CustomCallback, c.Request.URL.RawQuery))
		return
	}

	// 管理端（无自定义回调）。
	if resultCode != visualValidateResultOK {
		// 认证未成功（用户取消/失败），标记会话失败，不阻塞。
		_ = model.CompleteVisualValidateSession(bytedToken, "", model.VisualSessionFailed)
		renderVisualCallbackPage(c, "认证未完成")
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

	ctx := context.WithoutCancel(c.Request.Context())
	if settleVisualValidate(ctx, session.Provider, bytedToken, session) {
		renderVisualCallbackPage(c, "真人素材已就绪")
	} else {
		renderVisualCallbackPage(c, "获取素材组失败")
	}
}

// settleVisualValidate 轮询 GetVisualValidateResult 拿真人组 id，记录 LivenessFace 归属（用会话组名）
// 并回填会话状态。返回是否成功。管理端同步调用；对外 API 场景由后台 goroutine 调用。
// ctx 应携带 trace_id/request_id（后台用 context.WithoutCancel 派生）。
func settleVisualValidate(ctx context.Context, provider, bytedToken string, session *model.UserMaterialVisualSession) bool {
	groupId, err := materiallib.WaitVisualValidateGroupId(ctx, provider, bytedToken, 60*time.Second, 3*time.Second)
	if err != nil {
		_ = model.CompleteVisualValidateSession(bytedToken, "", model.VisualSessionFailed)
		logger.LogError(ctx, "get visual validate group id failed: "+err.Error())
		return false
	}
	if err := model.AddUserMaterialGroup(session.UserId, session.Provider, groupId, session.Name, "LivenessFace"); err != nil {
		logger.LogError(ctx, "record liveness material group failed: "+err.Error())
	}
	if err := model.CompleteVisualValidateSession(bytedToken, groupId, model.VisualSessionActive); err != nil {
		logger.LogError(ctx, "complete visual validate session failed: "+err.Error())
	}
	return true
}

// validVisualCallbackScheme 校验重定向目标是 http/https（拒绝 javascript:/data: 等，防开放重定向）。
func validVisualCallbackScheme(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// buildCustomRedirectURL 拼接客户回调地址与火山原始 query（原样透传）。
func buildCustomRedirectURL(callbackURL, rawQuery string) string {
	if rawQuery == "" {
		return callbackURL
	}
	sep := "?"
	if strings.Contains(callbackURL, "?") {
		sep = "&"
	}
	return callbackURL + sep + rawQuery
}

// renderVisualCallbackPage 返回 H5 认证结果的简单 HTML 提示页（浏览器展示，用户可关闭）。
func renderVisualCallbackPage(c *gin.Context, msg string) {
	page := "<!doctype html><html><head><meta charset='utf-8'><title>素材认证</title></head>" +
		"<body style='font-family:sans-serif;text-align:center;padding-top:80px'>" +
		"<div style='font-size:20px'>" + html.EscapeString(msg) + "</div>" +
		"<div style='color:#888;margin-top:12px'>此页面可关闭</div></body></html>"
	c.Data(200, "text/html; charset=utf-8", []byte(page))
}
