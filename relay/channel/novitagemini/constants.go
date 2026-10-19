package novitagemini

const (
	ChannelName = "novitagemini"
)

// ModelList 默认模型列表。gemini-3.1-flash-lite-image 在当前上游账号下返回
// 403（model access denied），因此不纳入默认列表；管理员可按需自行添加。
var ModelList = []string{
	"gemini-nano-banana-2.1",
	"gemini-3.1-flash-image",
	"gemini-3-pro-image",
	"gemini-2.5-flash-image",
	"gemini-nano-banana-2.1-as",
	"gemini-3.1-flash-lite-image-as",
	"gemini-3.1-flash-image-as",
	"gemini-3-pro-image-as",
	"gemini-2.5-flash-image-as",
}

const (
	// maxInputImages bounds how many reference images one edit request may inline.
	maxInputImages = 16
	// maxInputImageBytes bounds one decoded input image. The request body as a
	// whole is capped by MAX_REQUEST_BODY_MB, but base64 encoding and re-marshal
	// can multiply memory use, so each image needs its own ceiling.
	maxInputImageBytes = 20 << 20
	// maxTotalInputImageBytes bounds the decoded size of all input images in one
	// edit request.
	maxTotalInputImageBytes = 40 << 20
	// maxResponseImages bounds the per-image billing fallback against a hostile
	// or malformed upstream response.
	maxResponseImages = 16
	// maxUpstreamResponseBytes bounds how much of the upstream body is read into
	// memory. Two 4K PNGs encoded as base64 stay well under this.
	maxUpstreamResponseBytes = 64 << 20
)
