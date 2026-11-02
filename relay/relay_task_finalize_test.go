package relay

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	tokenhubvolc "github.com/QuantumNous/new-api/relay/channel/task/tokenhubvolc"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	finalizeTestUserID    = 91
	finalizeTestChannelID = 6601
	finalizeTestOriginID  = "task_origin_draft"
	finalizeTestModel     = "doubao-seedance-2-5-260628"
)

// finalizeFixture 是一次 finalize 解析测试的输入：原任务的样片标记、状态、
// 提交/完成时间与上游 ID。
//
// SubmitTime 与 FinishTime 分开给：有效期从上游样片的 created_at 起算（网关侧对应
// SubmitTime），不是从生成完成时刻。测试必须能构造出「刚生成完、但提交已超 7 天」
// 的样片，否则选错字段也测不出来。
type finalizeFixture struct {
	Draft          bool
	Status         model.TaskStatus
	SubmitTime     int64
	FinishTime     int64
	UpstreamTaskID string
	HasVideoInput  bool
	OtherUserID    int // 非零表示任务归属他人（越权用例）
	QueryVideoID   string
	// NilChannelMeta 模拟 GenRelayInfo 的真实产出：它不填 ChannelMeta，
	// 而 ResolveOriginTask 要读 info.ChannelId 才能判断是否已落在原渠道上。
	NilChannelMeta bool
	// ChannelType 是原任务所属渠道类型；0 表示默认的 VolcEngine25（66）。
	ChannelType int
	// NativeRequest 表示请求来自 /api/v3 并已写入解析后的原生请求体。
	NativeRequest bool
	// ModelMapping 是渠道配置的模型映射；成片锁定渠道后必须能继续按它翻译模型名。
	ModelMapping string
}

// seedOriginFixture 建内存库、一条启用渠道与一条原任务，并返回一个指向该原任务的
// gin context。suffix 决定路径的最后一段（remix / finalize），因为 action 由路径推断。
func seedOriginFixture(t *testing.T, tc finalizeFixture, suffix string) *gin.Context {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// 内存库每条连接各自独立，必须收敛到单连接，否则建表与查询会落在不同库上。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	channelType := tc.ChannelType
	if channelType == 0 {
		channelType = constant.ChannelTypeVolcEngine25
	}
	channel := &model.Channel{
		Id:     finalizeTestChannelID,
		Type:   channelType,
		Name:   "volcengine-25-test",
		Status: common.ChannelStatusEnabled,
		Key:    "sk-test",
	}
	if tc.ModelMapping != "" {
		mapping := tc.ModelMapping
		channel.ModelMapping = &mapping
	}
	require.NoError(t, db.Create(channel).Error)

	ownerID := finalizeTestUserID
	if tc.OtherUserID != 0 {
		ownerID = tc.OtherUserID
	}
	originID := finalizeTestOriginID
	if tc.QueryVideoID != "" {
		originID = tc.QueryVideoID
	}
	require.NoError(t, db.Create(&model.Task{
		TaskID:     finalizeTestOriginID,
		UserId:     ownerID,
		ChannelId:  finalizeTestChannelID,
		Platform:   constant.TaskPlatform(strconv.Itoa(channelType)),
		Action:     constant.TaskActionGenerate,
		Status:     tc.Status,
		SubmitTime: tc.SubmitTime,
		FinishTime: tc.FinishTime,
		Properties: model.Properties{
			Draft:             tc.Draft,
			UpstreamModelName: finalizeTestModel,
		},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: tc.UpstreamTaskID, HasVideoInput: tc.HasVideoInput},
	}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if tc.NativeRequest {
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
		tokenhubvolc.StoreNativeRequest(c, &tokenhubvolc.NativeRequest{
			Model:   finalizeTestModel,
			Content: []tokenhubvolc.NativeContentItem{{Type: "draft_task", DraftTask: &tokenhubvolc.DraftTaskRef{ID: originID}}},
		})
	} else {
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/"+originID+"/"+suffix, nil)
		c.Params = gin.Params{{Key: "video_id", Value: originID}}
	}
	c.Set("id", finalizeTestUserID)
	return c
}

// taskErrCode 取错误码，nil 错误返回空串。
func taskErrCode(taskErr *dto.TaskError) string {
	if taskErr == nil {
		return ""
	}
	return taskErr.Code
}

// runResolveOriginTask 在内存库上跑一次 finalize 的 ResolveOriginTask，
// 返回解析出的 info 与错误码（无错误为空串）。
func runResolveOriginTask(t *testing.T, tc finalizeFixture) (*relaycommon.RelayInfo, string) {
	t.Helper()

	c := seedOriginFixture(t, tc, "finalize")
	info := &relaycommon.RelayInfo{
		UserId:        finalizeTestUserID,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		// 与原任务同渠道 → 跳过 key 轮换分支，只验证归属与锁定
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: finalizeTestChannelID},
	}
	if tc.NilChannelMeta {
		info.ChannelMeta = nil
	}
	return info, taskErrCode(ResolveOriginTask(c, info))
}

func TestResolveOriginTaskFinalizeHappyPath(t *testing.T) {
	info, code := runResolveOriginTask(t, finalizeFixture{
		Draft:          true,
		Status:         model.TaskStatusSuccess,
		SubmitTime:     time.Now().Unix() - 3600,
		FinishTime:     time.Now().Unix() - 3500,
		UpstreamTaskID: "cgt-origin-123",
		HasVideoInput:  true,
	})

	require.Empty(t, code)
	assert.Equal(t, constant.TaskActionFinalize, info.Action)
	assert.Equal(t, finalizeTestOriginID, info.OriginTaskID)
	assert.Equal(t, "cgt-origin-123", info.OriginUpstreamTaskID)
	assert.True(t, info.OriginHasVideoInput, "成片档位必须继承样片的视频输入维度")
	assert.Equal(t, finalizeTestModel, info.OriginModelName)
	require.NotNil(t, info.LockedChannel, "必须锁定到原任务的渠道")

	// 成片绝不能继承第一步的 OtherRatios：它的 seconds/size 会把 token 结算放大数倍
	assert.Empty(t, info.PriceData.OtherRatios(), "finalize 不得继承样片的 OtherRatios")
}

// TestResolveOriginTaskFinalizeRejections 各种不能作为成片源的样片：
// 非样片任务、未完成、已过期、越权、上游 ID 缺失。
func TestResolveOriginTaskFinalizeRejections(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name string
		tc   finalizeFixture
		want string
	}{
		{
			name: "非样片任务",
			tc: finalizeFixture{
				Draft: false, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
				UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_not_draft",
		},
		{
			name: "样片尚未完成",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusInProgress, SubmitTime: now, FinishTime: 0,
				UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_task_not_finished",
		},
		{
			// FinishTime 故意设成刚刚——有效期从 created_at（网关侧 SubmitTime）起算，
			// 不是从生成完成时刻。按 FinishTime 判会把上游已过期的样片放行。
			name: "样片已过期（提交超过 7 天，但刚生成完）",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess,
				SubmitTime:     now - relaycommon.DraftTaskValidSeconds - 1,
				FinishTime:     now,
				UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_draft_expired",
		},
		{
			name: "上游任务 ID 缺失",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
				UpstreamTaskID: "", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_upstream_id_missing",
		},
		{
			name: "越权（他人任务）",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
				UpstreamTaskID: "cgt-1", OtherUserID: finalizeTestUserID + 1,
				QueryVideoID: finalizeTestOriginID,
			},
			want: "task_not_exist",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, code := runResolveOriginTask(t, tt.tc)
			assert.Equal(t, tt.want, code)
		})
	}
}

// TestResolveOriginTaskWithoutChannelMeta 回归：GenRelayInfo 不填 ChannelMeta，
// 而 ResolveOriginTask 要读 info.ChannelId 判断请求是否已落在原任务的渠道上。
// 不在函数内补一次 InitChannelMeta 就是解引用空指针——remix 与 finalize 双双 500。
// 这里同时覆盖两条 action，断言不 panic 且渠道被正确切到原任务渠道。
func TestResolveOriginTaskWithoutChannelMeta(t *testing.T) {
	tests := []struct {
		name       string
		pathSuffix string
		wantAction string
	}{
		{name: "remix", pathSuffix: "remix", wantAction: constant.TaskActionRemix},
		{name: "finalize", pathSuffix: "finalize", wantAction: constant.TaskActionFinalize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := seedOriginFixture(t, finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess,
				SubmitTime: time.Now().Unix() - 60, FinishTime: time.Now().Unix() - 30,
				UpstreamTaskID: "cgt-origin-nil-meta",
			}, tt.pathSuffix)

			info := &relaycommon.RelayInfo{
				UserId:        finalizeTestUserID,
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				ChannelMeta:   nil, // 关键：模拟 GenRelayInfo 的真实产出
			}

			require.Empty(t, taskErrCode(ResolveOriginTask(c, info)))
			assert.Equal(t, tt.wantAction, info.Action)
			require.NotNil(t, info.ChannelMeta, "必须补上 ChannelMeta")
			assert.Equal(t, finalizeTestChannelID, info.ChannelId, "应切到原任务的渠道")
			assert.Equal(t, constant.ChannelTypeVolcEngine25, info.ChannelType)
			assert.Equal(t, "sk-test", info.ApiKey)
		})
	}
}

// TestResolveOriginTaskFinalizeJustInsideValidity 刚好处在 7 天窗口内（少 1 秒）
// 的样片仍然可用——边界不能被写成「>=」而误杀。
func TestResolveOriginTaskFinalizeJustInsideValidity(t *testing.T) {
	info, code := runResolveOriginTask(t, finalizeFixture{
		Draft: true, Status: model.TaskStatusSuccess,
		SubmitTime:     time.Now().Unix() - relaycommon.DraftTaskValidSeconds + 60,
		FinishTime:     time.Now().Unix() - 60,
		UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
	})

	require.Empty(t, code)
	assert.Equal(t, "cgt-1", info.OriginUpstreamTaskID)
}

// TestResolveOriginTaskFinalizeIgnoresClientModel 验证成片模型以原任务为准：
// 成片请求仍带 model（协议要求），客户端可以传一个更便宜的模型来压低预扣与
// 结算，而实际产出是原样片的模型。
func TestResolveOriginTaskFinalizeIgnoresClientModel(t *testing.T) {
	now := time.Now().Unix()
	c := seedOriginFixture(t, finalizeFixture{
		Draft: true, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
		UpstreamTaskID: "cgt-origin-123",
	}, "finalize")
	info := &relaycommon.RelayInfo{
		UserId:        finalizeTestUserID,
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: finalizeTestChannelID},
		// 模拟客户端在成片请求里声明了一个更便宜的模型
		OriginModelName: "doubao-seedance-2-0-mini-260128",
	}

	require.Empty(t, taskErrCode(ResolveOriginTask(c, info)))
	assert.Equal(t, finalizeTestModel, info.OriginModelName, "计费模型必须取原任务记录的模型")
}

// TestResolveOriginTaskFinalizeLoadsChannelModelMapping 回归：原生协议的成片
// 请求在 distributor 里跳过渠道选择，渠道的 model_mapping 不会进上下文。
// ResolveOriginTask 锁定原渠道时必须自己补上，否则 ModelMappedHelper 会把
// 对客模型名（doubao-...）原样发给上游，上游报「无效的模型」。
func TestResolveOriginTaskFinalizeLoadsChannelModelMapping(t *testing.T) {
	const mapping = `{"doubao-seedance-2-5-260628":"cdance2.5"}`
	now := time.Now().Unix()
	c := seedOriginFixture(t, finalizeFixture{
		Draft: true, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
		UpstreamTaskID: "cgt-origin-123",
		ChannelType:    constant.ChannelTypeAistoreVolc,
		NativeRequest:  true,
		ModelMapping:   mapping,
	}, "")
	info := &relaycommon.RelayInfo{
		UserId:          finalizeTestUserID,
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: finalizeTestModel,
	}

	require.Empty(t, taskErrCode(ResolveOriginTask(c, info)))
	assert.Equal(t, mapping, common.GetContextKeyString(c, constant.ContextKeyChannelModelMapping))

	require.NoError(t, helper.ModelMappedHelper(c, info, nil))
	assert.Equal(t, "cdance2.5", info.UpstreamModelName, "成片必须按原渠道的映射翻译上游模型名")
	assert.Equal(t, finalizeTestModel, info.OriginModelName)
}

// TestResolveOriginTaskFinalizeChannelRouting 验证成片停留在能承载样片的通道上：
// 原生协议的样片只能落在服务 /api/v3 的渠道（70 / 71），不能跨到不认 draft_task
// 引用的渠道；类型 70 兼容旧视频接口，所以 /v1/videos 引用的 70 渠道样片可以在原
// 渠道成片，而只服务原生协议的 71 不行。
func TestResolveOriginTaskFinalizeChannelRouting(t *testing.T) {
	now := time.Now().Unix()
	base := finalizeFixture{
		Draft: true, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
		UpstreamTaskID: "cgt-origin-123",
	}
	newInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			UserId:        finalizeTestUserID,
			TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: finalizeTestChannelID},
		}
	}

	t.Run("原生请求引用非 70 渠道的样片", func(t *testing.T) {
		tc := base
		tc.ChannelType = constant.ChannelTypeVolcEngine25
		tc.NativeRequest = true
		c := seedOriginFixture(t, tc, "")
		assert.Equal(t, "origin_task_not_native", taskErrCode(ResolveOriginTask(c, newInfo())))
	})

	t.Run("/v1/videos 引用 70 渠道的样片", func(t *testing.T) {
		tc := base
		tc.ChannelType = constant.ChannelTypeTokenHubVolc
		c := seedOriginFixture(t, tc, "finalize")

		info := newInfo()
		require.Empty(t, taskErrCode(ResolveOriginTask(c, info)))
		assert.Equal(t, constant.TaskActionFinalize, info.Action)
		assert.Equal(t, "cgt-origin-123", info.OriginUpstreamTaskID)
	})

	t.Run("原生请求引用 71 渠道的样片", func(t *testing.T) {
		tc := base
		tc.ChannelType = constant.ChannelTypeAistoreVolc
		tc.NativeRequest = true
		c := seedOriginFixture(t, tc, "")

		info := newInfo()
		require.Empty(t, taskErrCode(ResolveOriginTask(c, info)))
		assert.Equal(t, constant.TaskActionFinalize, info.Action)
		assert.Equal(t, "cgt-origin-123", info.OriginUpstreamTaskID)
	})

	// 71 只服务原生协议：旧接口的成片入口不能用它在 dofinalize 里成片。
	t.Run("/v1/videos 引用 71 渠道的样片被拒", func(t *testing.T) {
		tc := base
		tc.ChannelType = constant.ChannelTypeAistoreVolc
		c := seedOriginFixture(t, tc, "finalize")
		assert.Equal(t, "origin_task_path_mismatch", taskErrCode(ResolveOriginTask(c, newInfo())))
	})
}

// TestResolveOriginTaskRemixRespectsChannelPath 回归：remix 会锁定原任务渠道，
// 不经过 Distribute 的路径校验，因此 ResolveOriginTask 必须自己补一次。类型 70
// 不支持 remix，不能因为渠道被锁定就把一个它不认识的动作交给它。
func TestResolveOriginTaskRemixRespectsChannelPath(t *testing.T) {
	now := time.Now().Unix()
	base := finalizeFixture{
		Draft: true, Status: model.TaskStatusSuccess, SubmitTime: now, FinishTime: now,
		UpstreamTaskID: "cgt-origin-123",
	}
	runRemix := func(t *testing.T, tc finalizeFixture) (*relaycommon.RelayInfo, string) {
		t.Helper()
		c := seedOriginFixture(t, tc, "remix")
		info := &relaycommon.RelayInfo{
			UserId:        finalizeTestUserID,
			TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			ChannelMeta:   &relaycommon.ChannelMeta{ChannelId: finalizeTestChannelID},
		}
		return info, taskErrCode(ResolveOriginTask(c, info))
	}

	t.Run("70 渠道拒绝 remix", func(t *testing.T) {
		tc := base
		tc.ChannelType = constant.ChannelTypeTokenHubVolc
		_, code := runRemix(t, tc)
		assert.Equal(t, "origin_task_path_mismatch", code)
	})

	t.Run("66 渠道仍可 remix", func(t *testing.T) {
		info, code := runRemix(t, base)
		require.Empty(t, code)
		assert.Equal(t, constant.TaskActionRemix, info.Action)
	})
}
