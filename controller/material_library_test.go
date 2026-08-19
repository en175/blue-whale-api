package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setMaterialConfig 经由配置系统写入素材库配置，模拟管理员 PUT /api/option 的加载路径。
func setMaterialConfig(t *testing.T, raw string) {
	t.Helper()
	registered := config.GlobalConfig.Get("material_library")
	require.NotNil(t, registered)
	require.NoError(t, config.UpdateConfigFromMap(registered, map[string]string{"config": raw}))
}

// TestMaterialLibraryProxy_rejectsWhenNotReady 验证代理的前置守卫：未启用/凭证不全时拒绝。
// 显式把所有字段置空——配置系统的 JSON 合并语义下，空对象 {} 不会清掉已有字段。
func TestMaterialLibraryProxy_rejectsWhenNotReady(t *testing.T) {
	setMaterialConfig(t, `{"enabled":false,"ak":"","sk":""}`)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Any("/api/material/*path", MaterialLibraryProxy)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/material/ListAssetGroups", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"success":false,"message":"material library is not configured"}`, w.Body.String())
}
