package mediakit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doSubmitResponse 用给定的上游响应体驱动 DoResponse，返回它交给上层的三件返回值
// 与写回客户端的状态码/响应体。
func doSubmitResponse(t *testing.T, upstreamBody string) (taskID string, taskErr *dto.TaskError, clientStatus int, clientBody string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/mediakit/video-frame-interpolation", nil)

	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public_1"}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}

	adaptor := &TaskAdaptor{}
	id, _, err := adaptor.DoResponse(c, resp, info)
	return id, err, rec.Code, rec.Body.String()
}

// TestDoResponseMapsUpstreamBusinessFailureTo400 保护 IKHQKY 修的契约：上游以
// HTTP 200 + success=false 表达提交失败时，只有「请求体不符合上游 schema」这一类
// 会走到这里，因此必须映射成 400 而不是 502。映射影响两件事：客户端看到的错误
// 归因（自己的入参问题 vs 网关/上游故障），以及 shouldRetryTaskRelay 是否跨渠道
// 重试一个注定失败的请求——5xx 会重试，400 不会。
func TestDoResponseMapsUpstreamBusinessFailureTo400(t *testing.T) {
	cases := []struct {
		name           string
		upstreamBody   string
		wantMessageHas string
	}{
		{
			name:           "带 error 对象",
			upstreamBody:   `{"success":false,"message":"invalid params","error":{"code":"InvalidParameter","message":"fps must be one of 30/60/120"}}`,
			wantMessageHas: "InvalidParameter: fps must be one of 30/60/120",
		},
		{
			name:           "只有 message，无 error 对象",
			upstreamBody:   `{"success":false,"message":"boom"}`,
			wantMessageHas: "boom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskID, taskErr, _, clientBody := doSubmitResponse(t, tc.upstreamBody)

			require.NotNil(t, taskErr, "business failure must surface as a task error")
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, "invalid_response", taskErr.Code)
			assert.Contains(t, taskErr.Message, tc.wantMessageHas, "上游原文不能丢，否则无法归因")
			assert.Empty(t, taskID)
			// 错误响应体由 controller 的 respondTaskError 统一写，这里若也写一份
			// 成功信封就会出现两个响应体。recorder 默认 200，故只断言没写内容。
			assert.Empty(t, clientBody, "失败分支不得写对客信封")
		})
	}
}

// TestDoResponseTreatsSuccessWithoutTaskIDAsUpstreamFault 固定另一半契约：上游
// 声称成功却没给 task_id 是上游侧的畸形响应，与客户端入参无关，必须维持 5xx，
// 以便 shouldRetryTaskRelay 仍会换渠道重试。
func TestDoResponseTreatsSuccessWithoutTaskIDAsUpstreamFault(t *testing.T) {
	taskID, taskErr, _, _ := doSubmitResponse(t, `{"success":true,"task_id":""}`)

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	assert.Empty(t, taskID)
}

// TestDoResponseReturnsUpstreamTaskIDOnSuccess 保证正常路径没有被上面的分支改动
// 波及：返回上游 task_id 落库，同时只把网关公开任务 ID 写给客户端。
func TestDoResponseReturnsUpstreamTaskIDOnSuccess(t *testing.T) {
	taskID, taskErr, clientStatus, clientBody := doSubmitResponse(t, `{"success":true,"task_id":"upstream_abc"}`)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream_abc", taskID)
	assert.Equal(t, http.StatusOK, clientStatus)

	var envelope map[string]any
	require.NoError(t, common.Unmarshal([]byte(clientBody), &envelope))
	assert.Equal(t, true, envelope["success"])
	assert.Equal(t, "task_public_1", envelope["task_id"], "对客只能暴露网关公开任务 ID")
}
