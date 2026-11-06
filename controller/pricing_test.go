package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 分组只出现在这个测试里，保证不会命中 UserUsableGroups 的默认配置。
const pricingExclusiveGroup = "pricing-test-exclusive-group"

// 管理员要配置全部分组的模型折扣，/api/pricing 不能按管理员自己的可用分组裁剪；
// 普通用户仍只能看到自己可用分组的模型。
func TestGetPricingFiltersModelsOnlyForNonAdmin(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	admin := model.User{
		Username: "pricing-admin",
		Password: "hashed-password",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     999,
		Name:   "pricing-test-channel",
		Type:   1,
		Status: common.ChannelStatusEnabled,
		Group:  pricingExclusiveGroup,
		Models: "exclusive-model",
		Key:    "sk-test",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     pricingExclusiveGroup,
		Model:     "exclusive-model",
		ChannelId: 999,
		Enabled:   true,
	}).Error)

	// 定价结果进程内缓存，测试前后都要失效
	model.InvalidatePricingCache()
	t.Cleanup(model.InvalidatePricingCache)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("pricing-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", admin.Username)
		session.Set("role", admin.Role)
		session.Set("id", admin.Id)
		session.Set("status", admin.Status)
		session.Set("group", admin.Group)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET("/api/pricing", middleware.HeaderNavModuleAuth("pricing"), GetPricing)

	requestModelNames := func(t *testing.T, authenticated bool) []string {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
		if authenticated {
			loginRecorder := httptest.NewRecorder()
			router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
			require.Equal(t, http.StatusNoContent, loginRecorder.Code)
			for _, item := range loginRecorder.Result().Cookies() {
				request.AddCookie(item)
			}
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)

		var body struct {
			Success bool `json:"success"`
			Data    []struct {
				ModelName string `json:"model_name"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.True(t, body.Success)

		names := make([]string, 0, len(body.Data))
		for _, item := range body.Data {
			names = append(names, item.ModelName)
		}
		return names
	}

	require.Contains(t, requestModelNames(t, true), "exclusive-model",
		"管理员应看到自己可用分组之外的模型")
	require.NotContains(t, requestModelNames(t, false), "exclusive-model",
		"普通用户不应看到自己可用分组之外的模型")
}
