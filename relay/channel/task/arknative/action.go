package arknative

import "github.com/QuantumNous/new-api/constant"

// InferAction 按 content[] 的实际输入形态写入任务类型。任务日志与筛选依赖
// action 区分「文生 / 图生 / 视频生」：视频优先，其次图片，纯文本兜底。
// 音频与 draft_task 不单独构成一种类型。
func InferAction(native *NativeRequest) string {
	hasImage := false
	for _, item := range native.Content {
		if item.Type == "video_url" || item.VideoURL != nil {
			return constant.TaskActionVideoGenerate
		}
		if item.Type == "image_url" || item.ImageURL != nil {
			hasImage = true
		}
	}
	if hasImage {
		return constant.TaskActionGenerate
	}
	return constant.TaskActionTextGenerate
}
