package highwayapi

const (
	ChannelName = "highwayapi"
)

// ModelList 默认模型列表。HighwayAPI 的模型名拼接在请求路径上（{baseURL}/{model}），
// 上游 /v3 下的其他模型可由管理员在渠道配置中自行添加。
var ModelList = []string{
	"gpt-image-2-text-to-image",
	"gpt-image-2-light-text-to-image",
}
