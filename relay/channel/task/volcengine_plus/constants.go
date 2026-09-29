package volcengine_plus

var ModelList = []string{
	"doubao-seedance-2-5-260628",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
}

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
