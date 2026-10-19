package novitaopenai

import (
	"github.com/QuantumNous/new-api/relay/channel/openai"
)

const (
	ChannelName = "novitaopenai"
)

// ModelList 默认模型列表。上游 /openai 端点后续新增的模型可由管理员在渠道里自行添加。
var ModelList = []string{
	"gpt-image-2",
	"gpt-image-2.5-flare-oai",
	"gpt-image-2.5-sunburst-oai",
}

// Adaptor 复用 OpenAI 适配器：Novita 的 /openai 端点与 OpenAI 图片协议一致，
// generations 与 edits（multipart / JSON）都可以零转换透传。这里只覆盖模型
// 列表，避免管理端下拉里出现 OpenAI 全量模型。
type Adaptor struct {
	openai.Adaptor
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
