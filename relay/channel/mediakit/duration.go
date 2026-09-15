package mediakit

import (
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 官方未统一时长字段的键名与单位，故显式列出候选字段：先认带 _ms 后缀的
// 毫秒字段，再认按秒记的通用字段。两处都不命中就返回 0，由调用方放弃差额
// 结算、保持预扣——宁可少结算，也不靠猜。
var (
	durationMillisecondKeys = []string{"duration_ms", "duration_millis", "durationmillis"}
	durationSecondKeys      = []string{"duration", "video_duration", "audio_duration", "duration_seconds", "output_duration"}
)

// TaskDurationSeconds 从任务查询响应体里取真实处理时长（秒）。
//
// 入参是轮询层存进 task.Data 的原始查询响应（形如
// {"success":true,"status":"completed","result":{"duration":9.5,...}}）。
// 取到的值按 MaxTaskDurationSeconds 钳制：上游回包属于上游可控数据，
// 不能让它直接变成计费倍数。
func TaskDurationSeconds(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := common.Unmarshal(data, &envelope); err != nil {
		return 0
	}

	for _, key := range durationMillisecondKeys {
		if value, ok := positiveNumber(envelope.Result, key); ok {
			return clampDurationSeconds(value / 1000)
		}
	}
	for _, key := range durationSecondKeys {
		if value, ok := positiveNumber(envelope.Result, key); ok {
			return clampDurationSeconds(value)
		}
	}
	return 0
}

// positiveNumber 取数值字段，拒绝缺失、非数、NaN、Inf 与非正值。
func positiveNumber(scope map[string]any, key string) (float64, bool) {
	value, ok := scope[key].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0, false
	}
	return value, true
}

func clampDurationSeconds(seconds float64) float64 {
	if seconds > float64(relaycommon.MaxTaskDurationSeconds) {
		return float64(relaycommon.MaxTaskDurationSeconds)
	}
	return seconds
}

// ResultKeys 返回查询响应里 result 对象的键名（已排序），供拿不到时长时打日志。
//
// 官方各工具的 result 键名不统一（duration / video_duration / ...），首次接
// 真实上游时需要靠这行日志核对 TaskDurationSeconds 的候选字段是否命中。
func ResultKeys(data []byte) string {
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := common.Unmarshal(data, &envelope); err != nil {
		return ""
	}
	keys := make([]string, 0, len(envelope.Result))
	for key := range envelope.Result {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
