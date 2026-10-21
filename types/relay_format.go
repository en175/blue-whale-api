package types

type RelayFormat string

const (
	RelayFormatOpenAI                    RelayFormat = "openai"
	RelayFormatClaude                                = "claude"
	RelayFormatGemini                                = "gemini"
	RelayFormatOpenAIResponses                       = "openai_responses"
	RelayFormatOpenAIResponsesCompaction             = "openai_responses_compaction"
	RelayFormatOpenAIAudio                           = "openai_audio"
	RelayFormatOpenAIImage                           = "openai_image"
	RelayFormatOpenAIRealtime                        = "openai_realtime"
	RelayFormatRerank                                = "rerank"
	RelayFormatEmbedding                             = "embedding"

	RelayFormatTask    = "task"
	RelayFormatMjProxy = "mj_proxy"

	// RelayFormatMediaKit 覆盖 /v1/mediakit/*：请求体按 MediaKit 原生协议透传，
	// 网关只做鉴权、锁模型与计费。
	RelayFormatMediaKit = "mediakit"

	// RelayFormatDeepSeekTools 覆盖 /tools/search 与 /tools/fetch：请求体原样
	// 透传到 DeepSeek 渠道，按成功调用次数计费。
	RelayFormatDeepSeekTools = "deepseek_tools"
)
