package taskcommon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsProxyURL 锁住自引用判定：只有指向同一任务的本网关 /content 地址才算
// 「没有上游产物」。判错两个方向都有害——漏判会让网关请求自己（生产环境无限
// 套娃），误判会把真实的上游产物当成空产物丢掉。
func TestIsProxyURL(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		taskID string
		want   bool
	}{
		{"本网关代理地址", "https://gw.example.com/v1/videos/task_abc/content", "task_abc", true},
		{"无 ServerAddress 前缀", "/v1/videos/task_abc/content", "task_abc", true},
		{"上游直链不匹配", "https://up.example.com/v.mp4", "task_abc", false},
		{"别的任务的代理地址", "https://gw.example.com/v1/videos/task_other/content", "task_abc", false},
		{"空 URL", "", "task_abc", false},
		{"空白 URL", "   ", "task_abc", false},
		{"空 taskID", "https://gw.example.com/v1/videos/task_abc/content", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsProxyURL(tt.url, tt.taskID))
		})
	}
}
