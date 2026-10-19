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

// maxInputImages bounds how many reference images one edit request may inline.
const maxInputImages = 16
