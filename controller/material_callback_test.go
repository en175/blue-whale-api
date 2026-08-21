package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openVisualCallbackDB 为真人认证回调测试准备隔离的归属表 + 会话表。
func openVisualCallbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitCol()
	service.InitHttpClient() // 回调轮询走 service.GetHttpClient()，需初始化

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserMaterialGroup{}, &model.UserMaterialVisualSession{}))
	model.DB = db
	return db
}

// TestMaterialVisualValidateCallback_recordsLivenessGroup 端到端：H5 认证成功回调
// （resultCode=10000）→ 轮询 GetVisualValidateResult 拿 GroupId → 记录 LivenessFace 归属 + 会话 active。
func TestMaterialVisualValidateCallback_recordsLivenessGroup(t *testing.T) {
	openVisualCallbackDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "tokenhub", "真人组1", "bt-1", "https://h5/1", "https://gw/cb", "default"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"GroupId":"group-live-1"}}`))
	}))
	defer upstream.Close()
	setMaterialConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	r := gin.New()
	r.GET("/v1/material/h5-callback", MaterialVisualValidateCallback)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/material/h5-callback?bytedToken=bt-1&resultCode=10000", nil))

	require.Equal(t, http.StatusOK, w.Code)

	groups, err := model.GetMaterialGroups(0, "tokenhub", "LivenessFace")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "group-live-1", groups[0].SeedanceAssetGroupId)
	assert.Equal(t, "LivenessFace", groups[0].GroupType)
	assert.Equal(t, "真人组1", groups[0].Name)

	s, err := model.GetVisualValidateSessionByToken("bt-1")
	require.NoError(t, err)
	assert.Equal(t, model.VisualSessionActive, s.Status)
	assert.Equal(t, "group-live-1", s.GroupId)
}

// TestMaterialVisualValidateCallback_marksFailedOnBadResultCode 验证认证未完成（resultCode != 10000）
// 时标记会话失败、不产生归属。
func TestMaterialVisualValidateCallback_marksFailedOnBadResultCode(t *testing.T) {
	openVisualCallbackDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "tokenhub", "真人组2", "bt-2", "https://h5/2", "https://gw/cb", "default"))

	r := gin.New()
	r.GET("/v1/material/h5-callback", MaterialVisualValidateCallback)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/material/h5-callback?bytedToken=bt-2&resultCode=9999", nil))

	require.Equal(t, http.StatusOK, w.Code)
	s, err := model.GetVisualValidateSessionByToken("bt-2")
	require.NoError(t, err)
	assert.Equal(t, model.VisualSessionFailed, s.Status)

	groups, err := model.GetMaterialGroups(0, "tokenhub", "LivenessFace")
	require.NoError(t, err)
	assert.Len(t, groups, 0)
}

// TestMaterialVisualValidateCallback_unknownSession 验证未知会话的回调被忽略。
func TestMaterialVisualValidateCallback_unknownSession(t *testing.T) {
	openVisualCallbackDB(t)

	r := gin.New()
	r.GET("/v1/material/h5-callback", MaterialVisualValidateCallback)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/material/h5-callback?bytedToken=ghost&resultCode=10000", nil))

	require.Equal(t, http.StatusOK, w.Code)
	groups, err := model.GetMaterialGroups(0, "tokenhub", "")
	require.NoError(t, err)
	assert.Len(t, groups, 0)
}

// TestCreateVisualValidateSession_usesConfiguredCallbackURL 验证管理端发起认证时，
// 回调地址优先取配置 material_library.config 的 callback_url（而非依赖调用方传入）。
func TestCreateVisualValidateSession_usesConfiguredCallbackURL(t *testing.T) {
	openVisualCallbackDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"BytedToken":"bt-cfg","H5Link":"https://h5.example/1"}}`))
	}))
	defer upstream.Close()
	setMaterialConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"callback_url":"https://gw.example.com/v1/material/h5-callback"}`, upstream.URL))

	r := gin.New()
	r.POST("/api/materials/visual-sessions", func(c *gin.Context) {
		c.Set("id", 42)
		c.Set("role", 1)
		CreateVisualValidateSession(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/materials/visual-sessions", strings.NewReader(`{"name":"真人组1"}`)))

	require.Equal(t, http.StatusOK, w.Code)
	s, err := model.GetVisualValidateSessionByToken("bt-cfg")
	require.NoError(t, err)
	assert.Equal(t, "真人组1", s.Name)
	assert.Equal(t, "https://gw.example.com/v1/material/h5-callback", s.CallbackURL)
	assert.Equal(t, "https://h5.example/1", s.H5Link)
}
