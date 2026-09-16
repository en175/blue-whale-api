package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestTaskModel2DtoPopulatesUsage newapi 平台（返回 token 的上游）→ usage 非空。
func TestTaskModel2DtoPopulatesUsage(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_usage",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPI)),
		Status:   model.TaskStatusSuccess,
		Data:     json.RawMessage(`{"code":"success","data":{"task_id":"task_usage","status":"SUCCESS","usage_completion_tokens":108900,"usage_total_tokens":108900}}`),
	}
	dto_ := TaskModel2Dto(task)
	require.NotNil(t, dto_.Usage)
	assert.Equal(t, 108900, dto_.Usage.CompletionTokens)
	assert.Equal(t, 108900, dto_.Usage.TotalTokens)
}

// TestTaskModel2DtoNoUsageForKling 未实现 TaskUsageExtractor 的平台（kling）→ usage 为 nil。
func TestTaskModel2DtoNoUsageForKling(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_kling",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)),
		Status:   model.TaskStatusSuccess,
		Data:     json.RawMessage(`{"data":{"task_status":"succeed","final_unit_deduction":"10"}}`),
	}
	dto_ := TaskModel2Dto(task)
	assert.Nil(t, dto_.Usage)
}

// runMediaKitFetchBuilder 在内存库上跑一次查询响应构造，返回解析后的响应体。
func runMediaKitFetchBuilder(t *testing.T, upstreamResult string, storedResultURL string) map[string]any {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// 内存库每条连接各自独立，必须收敛到单连接，否则建表与查询会落在不同库上。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Task{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	const userID = 77
	const taskID = "task_mediakit_case"
	task := &model.Task{
		TaskID: taskID,
		UserId: userID,
		Status: model.TaskStatusSuccess,
		Data:   json.RawMessage(upstreamResult),
		PrivateData: model.TaskPrivateData{
			ResultURL: storedResultURL,
		},
	}
	require.NoError(t, db.Create(task).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/mediakit/tasks/"+taskID, nil)
	c.Params = gin.Params{{Key: "task_id", Value: taskID}}
	c.Set("id", userID)

	body, taskResp := mediaKitFetchRespBodyBuilder(c)
	require.Nil(t, taskResp)

	var response map[string]any
	require.NoError(t, json.Unmarshal(body, &response))
	result, ok := response["result"].(map[string]any)
	require.True(t, ok, "response must carry a result object")
	return result
}

// TestMediaKitFetchBuilderReturnsNestedResult 产物嵌在数组里时，客户端必须拿到
// 完整结构而不是被压成一个 url 字段——下游要靠 video_urls/snapshots 自己取产物。
func TestMediaKitFetchBuilderReturnsNestedResult(t *testing.T) {
	result := runMediaKitFetchBuilder(t,
		`{"success":true,"status":"completed","task_type":"drama-recap-vertical","result":{"mode":"Text","video_urls":["https://up.example.com/v.mp4"],"video_infos":[{"video_url":"https://up.example.com/v.mp4","duration":43.5}],"input_duration":246.63,"output_duration":43.5}}`,
		"https://up.example.com/v.mp4")

	assert.Equal(t, "Text", result["mode"])
	assert.Equal(t, []any{"https://up.example.com/v.mp4"}, result["video_urls"])
	assert.Equal(t, 43.5, result["output_duration"])
	assert.Equal(t, "https://up.example.com/v.mp4", result["url"])
}

// TestMediaKitFetchBuilderDropsSelfReferentialURL 没有产物的工具（轮询层兜底写成
// 本网关代理地址）不得回传该地址：它会让调用方拿到指向网关自身的 URL。
func TestMediaKitFetchBuilderDropsSelfReferentialURL(t *testing.T) {
	result := runMediaKitFetchBuilder(t,
		`{"success":true,"status":"completed","task_type":"asr-subtitles","result":{"duration":14.12,"subtitles":[]}}`,
		taskcommon.BuildProxyURL("task_mediakit_case"))

	_, hasURL := result["url"]
	assert.False(t, hasURL, "self-referential proxy URL must not be exposed as a result URL")
	// 没有 URL 的产物（字幕、评分）仍要完整回传，否则客户端什么都拿不到。
	assert.Equal(t, 14.12, result["duration"])
	assert.Equal(t, []any{}, result["subtitles"])
}
