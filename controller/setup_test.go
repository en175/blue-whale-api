package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetupCreatesRootWithDefaultToken 契约：初始化向导创建的管理员账号必须同时具备默认 token，
// 否则登录逻辑 setupLogin 会因「账号无 token」拒绝登录（见 IK8WXL）。
func TestSetupCreatesRootWithDefaultToken(t *testing.T) {
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Option{}, &model.Setup{}))
	model.InitOptionMap()

	constant.Setup = false
	constant.GenerateDefaultToken = true
	t.Cleanup(func() {
		constant.Setup = true
		constant.GenerateDefaultToken = false
	})

	router := gin.New()
	router.POST("/setup", PostSetup)

	recorder := performJSON(router, http.MethodPost, "/setup", map[string]string{
		"username":        "admin",
		"password":        "password123",
		"confirmPassword": "password123",
	}, "")
	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var root model.User
	require.NoError(t, db.Where("role = ?", common.RoleRootUser).First(&root).Error)

	var tokenCount int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", root.Id).Count(&tokenCount).Error)
	assert.GreaterOrEqual(t, tokenCount, int64(1), "初始化向导创建的管理员账号必须带默认 token")
}
