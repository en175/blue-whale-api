package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVideoProxyRejectsSelfReferentialResultURL 没有上游产物的任务，轮询层会把
// ResultURL 兜底写成本网关自己的 /content 地址（taskcommon.BuildProxyURL）。该地址
// 不是下载源：继续转发等于网关请求自己，生产环境（ServerAddress 为公网域名、443
// 放行）会无限套娃。必须在发起请求之前就拒绝。
func TestVideoProxyRejectsSelfReferentialResultURL(t *testing.T) {
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))

	channel := &model.Channel{
		Id:     901,
		Type:   constant.ChannelTypeMediaKit,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "mediakit-no-artifact",
	}
	require.NoError(t, db.Create(channel).Error)
	model.InitChannelCache()

	const taskID = "task_self_ref"
	task := &model.Task{
		TaskID:    taskID,
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: taskcommon.BuildProxyURL(taskID),
		},
	}
	require.NoError(t, db.Create(task).Error)

	router := gin.New()
	router.GET("/v1/videos/:task_id/content", VideoProxy)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/videos/"+taskID+"/content", nil))

	// 守卫缺失时这里会走到转发：本地因端口限制得 403，生产环境则回环请求自身。
	assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "no downloadable artifact")
}
