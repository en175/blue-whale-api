package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openMaterialTokenDB 为素材库路由鉴权测试准备最小库（user + token）。
func openMaterialTokenDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	return db
}

// setupMaterialRouter 镜像 SetApiRouter 里素材库的接线：/api/material/*path 带 TokenAuth 的通配代理。
func setupMaterialRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	materialRoute := r.Group("/api/material")
	materialRoute.Use(middleware.TokenAuth())
	{
		materialRoute.Any("/*path", controller.MaterialLibraryProxy)
	}
	return r
}

func TestMaterialRoute_tokenAuth(t *testing.T) {
	db := openMaterialTokenDB(t)
	r := setupMaterialRouter()

	t.Run("无 API key 被拒", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/material/ListAssetGroups", nil))
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("有效 API key 通过鉴权并到达代理", func(t *testing.T) {
		user := model.User{Username: "mat-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
		require.NoError(t, db.Create(&user).Error)
		token := model.Token{
			UserId:         user.Id,
			Key:            "materialtestkey00000000000000000000000000",
			Name:           "mat-token",
			Status:         common.TokenStatusEnabled,
			ExpiredTime:    -1,
			RemainQuota:    500000,
			UnlimitedQuota: true,
		}
		require.NoError(t, db.Create(&token).Error)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/material/ListAssetGroups", nil)
		req.Header.Set("Authorization", "Bearer sk-"+token.Key)
		r.ServeHTTP(w, req)

		// 鉴权通过 → 代理守卫返回"未配置"（配置未就绪），证明 handler 已被执行
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "material library is not configured")
	})
}
