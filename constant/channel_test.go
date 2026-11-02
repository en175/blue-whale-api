package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestChannelServesRequestPath 锁定渠道 70 / 71 / 72 的路径级隔离：火山原生协议路径只
// 允许类型 70 和 71；旧视频任务接口（/v1/video/generations 提交与查询、/v1/videos/{id}/finalize
// 成片）只有类型 70 与 72 兼容；其余路径一律不允许这三个类型。渠道选择的三条路径（亲和、
// 内存缓存、数据库能力表）都依赖这个判定，口径必须唯一。
func TestChannelServesRequestPath(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		path        string
		want        bool
	}{
		{"70 可服务原生创建", ChannelTypeTokenHubVolc, "/api/v3/contents/generations/tasks", true},
		{"70 可服务原生查询", ChannelTypeTokenHubVolc, "/api/v3/contents/generations/tasks/task_1", true},
		{"70 可服务旧接口提交", ChannelTypeTokenHubVolc, "/v1/video/generations", true},
		{"70 可服务旧接口查询", ChannelTypeTokenHubVolc, "/v1/video/generations/task_1", true},
		{"70 可服务成片", ChannelTypeTokenHubVolc, "/v1/videos/task_1/finalize", true},
		{"70 不可服务 OpenAI 视频", ChannelTypeTokenHubVolc, "/v1/videos", false},
		{"70 不可服务 remix", ChannelTypeTokenHubVolc, "/v1/videos/task_1/remix", false},
		{"70 不可服务 chat", ChannelTypeTokenHubVolc, "/v1/chat/completions", false},
		{"71 可服务原生创建", ChannelTypeAistoreVolc, "/api/v3/contents/generations/tasks", true},
		{"71 可服务原生查询", ChannelTypeAistoreVolc, "/api/v3/contents/generations/tasks/task_1", true},
		{"71 不可服务旧接口提交", ChannelTypeAistoreVolc, "/v1/video/generations", false},
		{"71 不可服务成片", ChannelTypeAistoreVolc, "/v1/videos/task_1/finalize", false},
		{"71 不可服务 OpenAI 视频", ChannelTypeAistoreVolc, "/v1/videos", false},
		{"71 不可服务 chat", ChannelTypeAistoreVolc, "/v1/chat/completions", false},
		{"72 不可服务原生创建", ChannelTypeAistoreVolcLegacy, "/api/v3/contents/generations/tasks", false},
		{"72 不可服务原生查询", ChannelTypeAistoreVolcLegacy, "/api/v3/contents/generations/tasks/task_1", false},
		{"72 可服务旧接口提交", ChannelTypeAistoreVolcLegacy, "/v1/video/generations", true},
		{"72 可服务旧接口查询", ChannelTypeAistoreVolcLegacy, "/v1/video/generations/task_1", true},
		{"72 可服务成片", ChannelTypeAistoreVolcLegacy, "/v1/videos/task_1/finalize", true},
		{"72 不可服务 OpenAI 视频", ChannelTypeAistoreVolcLegacy, "/v1/videos", false},
		{"72 不可服务 remix", ChannelTypeAistoreVolcLegacy, "/v1/videos/task_1/remix", false},
		{"72 不可服务 chat", ChannelTypeAistoreVolcLegacy, "/v1/chat/completions", false},
		{"61 不可服务原生路径", ChannelTypeTokenHub, "/api/v3/contents/generations/tasks", false},
		{"66 不可服务原生路径", ChannelTypeVolcEngine25, "/api/v3/contents/generations/tasks", false},
		{"普通渠道可服务 OpenAI 视频", ChannelTypeTokenHub25, "/v1/videos", true},
		{"普通渠道可服务 chat", ChannelTypeOpenAI, "/v1/chat/completions", true},
		// 前缀判定不能被嵌在别处的同名字符串骗过
		{"70 不受路径夹带欺骗", ChannelTypeTokenHubVolc, "/v1/videos/api/v3/contents/generations/tasks", false},
		{"71 不受路径夹带欺骗", ChannelTypeAistoreVolc, "/v1/videos/api/v3/contents/generations/tasks", false},
		{"72 不受路径夹带欺骗", ChannelTypeAistoreVolcLegacy, "/v1/videos/api/v3/contents/generations/tasks", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ChannelServesRequestPath(tt.channelType, tt.path))
		})
	}
}
