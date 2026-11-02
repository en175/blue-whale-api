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
	pathIsolationGroup              = "default"
	pathIsolationModel              = "doubao-seedance-2-5-260628"
	pathIsolationAistoreModel       = "cdance2.5"
	pathIsolationAistoreLegacyModel = "cdance2.0"
	pathIsolationOpenAICh           = 6701 // 类型 66，服务 /v1/videos
	pathIsolationNativeCh           = 6702 // 类型 70，服务 /api/v3 与旧视频任务接口
	pathIsolationAistoreCh          = 6703 // 类型 71，只服务 /api/v3
	pathIsolationAistoreLegacyCh    = 6704 // 类型 72，只服务旧视频任务接口
	pathIsolationAbortCode          = http.StatusTeapot
)

// seedPathIsolationDB 建内存库，登记同分组下 66 / 70 / 71 / 72 四条渠道。渠道同时
// 可用，路径隔离才有可排除的对象：只留一条时即使过滤失效也测不出来。
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
		{Id: pathIsolationAistoreCh, Type: constant.ChannelTypeAistoreVolc, Name: "aistore-ark", Key: "sk-c", Status: common.ChannelStatusEnabled},
		{Id: pathIsolationAistoreLegacyCh, Type: constant.ChannelTypeAistoreVolcLegacy, Name: "aistore-legacy", Key: "sk-d", Status: common.ChannelStatusEnabled},
	}
	require.NoError(t, db.Create(&channels).Error)
	for _, ch := range channels {
		priority := int64(0)
		addAbility := func(modelName string) {
			require.NoError(t, db.Create(&model.Ability{
				Group:     pathIsolationGroup,
				Model:     modelName,
				ChannelId: ch.Id,
				Enabled:   true,
				Priority:  &priority,
			}).Error)
		}
		// 71 不挂火山模型：与 70 同模型会让「原生路径选中 70」变成 70/71 随机
		// 二选一，测不出目标。
		if ch.Type != constant.ChannelTypeAistoreVolc && ch.Type != constant.ChannelTypeAistoreVolcLegacy {
			addAbility(pathIsolationModel)
		}
		// 66 与 71 另挂在厂商模型名下，让「类型 71 只在原生路径可选」有对照渠道。
		if ch.Type != constant.ChannelTypeTokenHubVolc && ch.Type != constant.ChannelTypeAistoreVolcLegacy {
			addAbility(pathIsolationAistoreModel)
		}
		// 72 独占一个模型名：与 66 同模型会让「旧接口选中 72」变成随机二选一。
		if ch.Type == constant.ChannelTypeAistoreVolcLegacy {
			addAbility(pathIsolationAistoreLegacyModel)
		}
	}
}

// runDistribute 通过真实的 gin 路由跑一次 Distribute，返回状态码与最终落到
// 上下文里的渠道 ID（未选中为 0）。handler 以 418 结束，避免触发成功路径上的
// 渠道亲和度记录（那部分依赖 Redis，与本测试无关）。
func runDistribute(t *testing.T, path, modelName string, pinnedChannelID int) (int, int) {
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
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"`+modelName+`"}`))
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
			code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationModel, 0)
			require.Equal(t, pathIsolationAbortCode, code, "应选出渠道并进入 handler")
			assert.Equal(t, pathIsolationNativeCh, channelID, "同一模型下类型 66 必须被路径过滤掉")
		}
	})

	t.Run("openai path excludes type 70", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, "/v1/videos", pathIsolationModel, 0)
			require.Equal(t, pathIsolationAbortCode, code)
			assert.Equal(t, pathIsolationOpenAICh, channelID)
		}
	})

	t.Run("pinned channel cannot bypass isolation", func(t *testing.T) {
		code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationModel, pathIsolationOpenAICh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /api/v3 钉到类型 66 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("pinned native channel on openai path rejected", func(t *testing.T) {
		code, channelID := runDistribute(t, "/v1/videos", pathIsolationModel, pathIsolationNativeCh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /v1/videos 钉到类型 70 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("pinned channel on its own path accepted", func(t *testing.T) {
		code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationModel, pathIsolationNativeCh)
		require.Equal(t, pathIsolationAbortCode, code)
		assert.Equal(t, pathIsolationNativeCh, channelID)
	})

	t.Run("native path selects type 71 for aistore model", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationAistoreModel, 0)
			require.Equal(t, pathIsolationAbortCode, code, "应选出渠道并进入 handler")
			assert.Equal(t, pathIsolationAistoreCh, channelID, "同一模型下类型 66 必须被路径过滤掉")
		}
	})

	t.Run("openai path excludes type 71", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, "/v1/videos", pathIsolationAistoreModel, 0)
			require.Equal(t, pathIsolationAbortCode, code)
			assert.Equal(t, pathIsolationOpenAICh, channelID)
		}
	})

	t.Run("pinned type 71 on openai path rejected", func(t *testing.T) {
		code, channelID := runDistribute(t, "/v1/videos", pathIsolationAistoreModel, pathIsolationAistoreCh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /v1/videos 钉到类型 71 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("legacy video path selects type 72", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, "/v1/video/generations", pathIsolationAistoreLegacyModel, 0)
			require.Equal(t, pathIsolationAbortCode, code, "应选出渠道并进入 handler")
			assert.Equal(t, pathIsolationAistoreLegacyCh, channelID)
		}
	})

	t.Run("native path excludes type 72", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationAistoreLegacyModel, 0)
			assert.NotEqual(t, pathIsolationAbortCode, code, "类型 72 不服务原生路径，必须选不到渠道")
			assert.Zero(t, channelID)
		}
	})

	t.Run("pinned type 72 on native path rejected", func(t *testing.T) {
		code, channelID := runDistribute(t, constant.NativeArkVideoTaskPath, pathIsolationAistoreLegacyModel, pathIsolationAistoreLegacyCh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /api/v3 钉到类型 72 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("pinned type 72 on openai path rejected", func(t *testing.T) {
		code, channelID := runDistribute(t, "/v1/videos", pathIsolationAistoreLegacyModel, pathIsolationAistoreLegacyCh)
		assert.Equal(t, http.StatusBadRequest, code, "把 /v1/videos 钉到类型 72 必须拒绝")
		assert.Zero(t, channelID)
	})

	t.Run("pinned type 72 on its own path accepted", func(t *testing.T) {
		code, channelID := runDistribute(t, "/v1/video/generations", pathIsolationAistoreLegacyModel, pathIsolationAistoreLegacyCh)
		require.Equal(t, pathIsolationAbortCode, code)
		assert.Equal(t, pathIsolationAistoreLegacyCh, channelID)
	})
}
