package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
// 完成时间与上游 ID。
type finalizeFixture struct {
	Draft          bool
	Status         model.TaskStatus
	FinishTime     int64
	UpstreamTaskID string
	HasVideoInput  bool
	OtherUserID    int // 非零表示任务归属他人（越权用例）
	QueryVideoID   string
	// NilChannelMeta 模拟 GenRelayInfo 的真实产出：它不填 ChannelMeta，
	// 而 ResolveOriginTask 要读 info.ChannelId 才能判断是否已落在原渠道上。
	NilChannelMeta bool
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

	channel := &model.Channel{
		Id:     finalizeTestChannelID,
		Type:   constant.ChannelTypeVolcEngine25,
		Name:   "volcengine-25-test",
		Status: common.ChannelStatusEnabled,
		Key:    "sk-test",
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
		Platform:   constant.TaskPlatform("66"),
		Action:     constant.TaskActionGenerate,
		Status:     tc.Status,
		FinishTime: tc.FinishTime,
		Properties: model.Properties{
			Draft:             tc.Draft,
			UpstreamModelName: finalizeTestModel,
		},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: tc.UpstreamTaskID, HasVideoInput: tc.HasVideoInput},
	}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/"+originID+"/"+suffix, nil)
	c.Params = gin.Params{{Key: "video_id", Value: originID}}
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
		FinishTime:     time.Now().Unix() - 3600,
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
				Draft: false, Status: model.TaskStatusSuccess, FinishTime: now,
				UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_not_draft",
		},
		{
			name: "样片尚未完成",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusInProgress, FinishTime: 0,
				UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_task_not_finished",
		},
		{
			name: "样片已过期（超过 7 天）",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess,
				FinishTime:     now - relaycommon.DraftTaskValidSeconds - 1,
				UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_draft_expired",
		},
		{
			name: "上游任务 ID 缺失",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess, FinishTime: now,
				UpstreamTaskID: "", QueryVideoID: finalizeTestOriginID,
			},
			want: "origin_upstream_id_missing",
		},
		{
			name: "越权（他人任务）",
			tc: finalizeFixture{
				Draft: true, Status: model.TaskStatusSuccess, FinishTime: now,
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
				Draft: true, Status: model.TaskStatusSuccess, FinishTime: time.Now().Unix() - 60,
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
		FinishTime:     time.Now().Unix() - relaycommon.DraftTaskValidSeconds + 60,
		UpstreamTaskID: "cgt-1", QueryVideoID: finalizeTestOriginID,
	})

	require.Empty(t, code)
	assert.Equal(t, "cgt-1", info.OriginUpstreamTaskID)
}
