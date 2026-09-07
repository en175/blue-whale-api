package volcengine_plus

const (
	ChannelName = "volcengine-enhanced"

	EnhanceStatusSkipped   = "skipped"
	EnhanceStatusPending   = "pending"
	EnhanceStatusSucceeded = "succeeded"
	EnhanceStatusFailed    = "failed"

	MaxEnhanceAttempts = 3

	DefaultMediaKitBaseURL    = "https://mediakit.cn-beijing.volces.com"
	DefaultMediaKitSubmitPath = "/api/v1/tools/enhance-video-generative"

	genericEnhanceFailReason = "processing failed"
)
