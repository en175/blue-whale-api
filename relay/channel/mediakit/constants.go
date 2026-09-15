package mediakit

// ChannelName 渠道展示名。
const ChannelName = "mediakit"

const (
	// DefaultBaseURL 是火山 AI MediaKit 的默认上游域名。
	DefaultBaseURL = "https://mediakit.cn-beijing.volces.com"

	// taskPathPrefix 是异步工具的提交路径前缀，完整路径为 {base}/api/v1/tools/{tool}。
	taskPathPrefix = "/api/v1/tools/"
	// syncPathPrefix 是同步工具的提交路径前缀，完整路径为 {base}/api/v1/tools-sync/{tool}。
	syncPathPrefix = "/api/v1/tools-sync/"
	// queryPathPrefix 是异步任务查询路径前缀，完整路径为 {base}/api/v1/tasks/{task_id}。
	queryPathPrefix = "/api/v1/tasks/"
)

// ModelList 默认模型列表。MediaKit 的工具名映射为模型名（见 registry.go），
// 管理员在渠道模型列表中勾选需要的工具即可，故此处不提供默认模型。
var ModelList = []string{}

// TaskPath 返回异步任务的查询路径。taskID 必须是调用方从上游取回、或从本库
// tasks 表读出的 id，不接受客户端字符串。
func TaskPath(taskID string) string {
	return queryPathPrefix + taskID
}
