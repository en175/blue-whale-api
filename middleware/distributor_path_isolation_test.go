package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	pathIsolationGroup     = "default"
	pathIsolationModel     = "doubao-seedance-2-5-260628"
	pathIsolationOpenAICh  = 6701 // 类型 66，服务 /v1/videos
	pathIsolationNativeCh  = 6702 // 类型 70，只服务 /api/v3
	pathIsolationAbortCode = http.StatusTeapot
)

// seedPathIsolationDB 建内存库，登记同一个模型下、同分组的 66 与 70 两条渠道。
// 两条渠道同时可用，路径隔离才有可排除的对象：只留一条时即使过滤失效也测不出来。
func seedPathIsolationDB(t *testing.T) {
	t.Helper()

	// 测试不经 InitDB，列名常量与 i18n 文案都要手动初始化：前者决定
	// `group` 的引号，后者在拒绝分支里被 abort 消息使用。
	model.InitCol()
	require.NoError(t, i18n.Init())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	previousDB := model.DB
	model.DB = db
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false // 走 DB 选路，才能覆盖 filterAbilitiesByRequestPathAndModel
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousCache
	})

	channels := []model.Channel{
		{Id: pathIsolationOpenAICh, Type: constant.ChannelTypeVolcEngine25, Name: "openai-shape", Key: "sk-a", Status: common.ChannelStatusEnabled},
		{Id: pathIsolationNativeCh, Type: constant.ChannelTypeTokenHubVolc, Name: "native-ark", Key: "sk-b", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&channels).Error)
	for _, ch := range channels {
		priority := int64(0)
		require.NoError(t, db.Create(&model.Ability{
			Group:     pathIsolationGroup,
			Model:     pathIsolationModel,
			ChannelId: ch.Id,
			Enabled:   true,
			Priority:  &priority,
		}).Error)
	}
}

// runDistribute 通过真实的 gin 路由跑一次 Distribute，返回状态码与最终落到
// 上下文里的渠道 ID（未选中为 0）。handler 以 418 结束，避免触发成功路径上的
// 渠道亲和度记录（那部分依赖 Redis，与本测试无关）。
func runDistribute(t *testing.T, path string, pinnedChannelID int) (int, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	selectedChannelID := 0

	router := gin.New()
	handler := func(c *gin.Context) {
		selectedChannelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		c.AbortWithStatus(pathIsolationAbortCode)
	}
	router.POST(path, func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, pathIsolationGroup)
		if pinnedChannelID != 0 {
			common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(pinnedChannelID))
		}
	}, Distribute(), handler)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"`+pathIsolationModel+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w.Code, selectedChannelID
}

// TestDistributePathIsolation 验证两条协议路径互斥选路，覆盖普通选路与
// specific_channel_id 钉死渠道这两条入口。两条入口都必须走同一个谓词，
// 否则令牌可以绕过路径隔离，把 /api/v3 钉到 OpenAI 形状的渠道上。
func TestDistributePathIsolation(t *testing.T) {
	seedPathIsolationDB(t)

	t.Run("native path selects type 70", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, 0)
			require.Equal(t, pathIsolationAbortCode, code, "应选出渠道并进入 handler")
			assert.Equal(t, pathIsolationNativeCh, channelID, "同一模型下类型 66 必须被路径过滤掉")
		}
	})

	t.Run("openai path excludes type 70", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, "/v1/videos", 0)
			require.Equal(t, pathIsolationAbortCode, code)
			assert.Equal(t, pathIsolationOpenAICh, channelID)
		}
	})

	t.Run("pinned channel cannot bypass isolation", func(t *testing.T) {
		code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationOpenAICh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /api/v3 钉到类型 66 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("pinned native channel on openai path rejected", func(t *testing.T) {
		code, channelID := runDistribute(t, "/v1/videos", pathIsolationNativeCh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /v1/videos 钉到类型 70 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("pinned channel on its own path accepted", func(t *testing.T) {
		code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationNativeCh)
		require.Equal(t, pathIsolationAbortCode, code)
		assert.Equal(t, pathIsolationNativeCh, channelID)
	})
}
