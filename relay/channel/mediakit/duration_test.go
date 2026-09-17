package mediakit

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
)

// TestTaskDurationSeconds 固定上游回包取时长的口径。取不到时必须返回 0——调用方
// 以 0 为「放弃差额结算、保持预扣」的信号，猜一个值会直接变成错收费。
func TestTaskDurationSeconds(t *testing.T) {
	cases := []struct {
		name string
		body string
		want float64
	}{
		{"秒字段", `{"success":true,"result":{"duration":9.5}}`, 9.5},
		{"毫秒字段换算成秒", `{"success":true,"result":{"duration_ms":90000}}`, 90},
		{"毫秒优先于秒", `{"success":true,"result":{"duration_ms":90000,"duration":9.5}}`, 90},
		{"视频时长字段", `{"success":true,"result":{"video_duration":12}}`, 12},
		{"无 result", `{"success":true,"status":"completed"}`, 0},
		{"result 里没有时长", `{"success":true,"result":{"video_url":"https://x/a.mp4"}}`, 0},
		{"空响应体", ``, 0},
		{"非法 JSON", `not json`, 0},
		{"字符串时长不认", `{"success":true,"result":{"duration":"9.5"}}`, 0},
		{"零与负数不认", `{"success":true,"result":{"duration":0}}`, 0},
		{"负数不认", `{"success":true,"result":{"duration":-5}}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, TaskDurationSeconds([]byte(tc.body)))
		})
	}
}

// TestTaskDurationSecondsClamps 上游回包是上游可控数据，它声称的时长会变成计费
// 乘数，必须钳到全局上界。
func TestTaskDurationSecondsClamps(t *testing.T) {
	body := `{"success":true,"result":{"duration":999999}}`
	assert.Equal(t, float64(relaycommon.MaxTaskDurationSeconds), TaskDurationSeconds([]byte(body)))
}

// TestResultKeys 固定「拿不到时长时打日志」用的键名清单：这是首次接真实上游时
// 核对候选字段是否命中的唯一手段，顺序必须稳定。
func TestResultKeys(t *testing.T) {
	got := ResultKeys([]byte(`{"success":true,"result":{"video_url":"u","duration":3,"fps":24}}`))
	assert.Equal(t, "duration,fps,video_url", got)
	assert.Equal(t, "", ResultKeys([]byte(`not json`)))
}
