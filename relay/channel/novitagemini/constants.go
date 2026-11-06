package novitagemini

const (
	ChannelName = "novitagemini"
)

// ModelList 默认模型列表。gemini-3.1-flash-lite-image 在当前上游账号下返回
// 403（model access denied），因此不纳入默认列表；管理员可按需自行添加。
//
// -as 系列走 Ai Studio 资源池：历史上（2026-10-19）有成功调用记录，2026-11-04
// 复测失败，属间歇性可用，不要当成永久不可用。
var ModelList = []string{
	"gemini-nano-banana-2.1",
	"gemini-3.1-flash-image",
	"gemini-3-pro-image",
	"gemini-2.5-flash-image",
	"nano-banana-2-t2i",
	"gemini-nano-banana-2.1-as",
	"gemini-3.1-flash-lite-image-as",
	"gemini-3.1-flash-image-as",
	"gemini-3-pro-image-as",
	"gemini-2.5-flash-image-as",
}

const (
	// maxInputImages bounds how many reference images one request may inline,
	// for both the edits translation path and the native generateContent path.
	maxInputImages = 16
	// maxInputImageBytes bounds one decoded input image. The request body as a
	// whole is capped by MAX_REQUEST_BODY_MB, but base64 encoding and re-marshal
	// can multiply memory use, so each image needs its own ceiling.
	maxInputImageBytes = 20 << 20
	// maxTotalInputImageBytes bounds the decoded size of all input images in one
	// request, for both the edits translation path and the native generateContent path.
	maxTotalInputImageBytes = 40 << 20
	// maxResponseImages bounds the per-image billing fallback against a hostile
	// or malformed upstream response.
	maxResponseImages = 16
	// estimatedTokensPerImage is the local fallback unit when upstream reported no
	// completion tokens but did return images. It matches the shared Gemini
	// handler's estimate so both routes bill the same way.
	estimatedTokensPerImage = 1400
	// maxUpstreamResponseBytes bounds how much of the upstream body is read into
	// memory. Two 4K PNGs encoded as base64 stay well under this.
	maxUpstreamResponseBytes = 64 << 20
)
