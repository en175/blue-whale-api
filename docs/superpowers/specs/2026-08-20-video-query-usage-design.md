# 视频查询任务响应返回真实消耗 token · 设计文档（Spec）

- **日期：** 2026-08-20
- **状态：** 待用户审阅
- **关联工单：** IK9ZGK
- **依据：** 与用户评审确认（token 口径 = 真实 token 数字；交付层 = 后端加 usage 字段 + api-docs 同步；实现方案 A）

---

## 1. 需求

api 文档站 `GET /v1/video/generations/{task_id}`（`zh/docs/video/query` 查询任务）的响应中没有返回真实消耗 token。需求方需要在查询任务响应中看到**真实消耗的 token**（completion / total）。

### 1.1 已确认口径

| 决策点 | 结论 |
|---|---|
| token 含义 | **真实 token 数字**（prompt/completion/total），非网关 quota、非积分 |
| 交付层 | 后端查询响应加一等 `usage` 字段 + 同步更新 api-docs 文档 |
| 实现方案 | 方案 A：查询时从 `task.Data` 提取 |
| kling `final_unit_deduction` | **不返回**（非真实 token，避免伪装误导） |
| `prompt_tokens` 字段 | 保留（`omitempty`）但视频上游基本只回 completion+total，不填充 |

## 2. 现状与事实

- 轮询时 `task.Data = redactVideoResponseBody(responseBody)`（`service/task_polling.go:501`），**完整上游信封已持久化**，历史+新任务都有数据，无需回填。
- 查询响应 `dto.TaskDto`（`dto/task.go:32`）无 `usage` 字段；`TaskModel2Dto`（`relay/relay_task.go:550`）输出 `TaskDto`，其中 `Data` 为原始信封（token 埋在里面，非一等字段）。
- 上游返回差异：
  - **newapi（seedance）/ tokenhub / doubao / xfai**：返回真实 `usage.completion_tokens` / `usage.total_tokens`（各适配器已解析用于结算）。
  - **kling**：仅 `final_unit_deduction`（扣费单位，非 token）。
  - **ali**：`usage` 为 `{duration, video_count, SR}`，非 token。
  - **gemini/vertex**：查询走 `tryRealtimeFetch` 自定义格式，不走 `TaskDto` 路径（本次不覆盖）。

## 3. 设计

### 3.1 数据结构（`dto/task.go`）

```go
// TaskUsage 任务真实消耗统计（仅支持返回 token 的上游填充）
type TaskUsage struct {
    PromptTokens     int `json:"prompt_tokens,omitempty"`     // 视频上游基本不返回，保留但不填充
    CompletionTokens int `json:"completion_tokens,omitempty"`
    TotalTokens      int `json:"total_tokens,omitempty"`
}

// TaskDto 新增字段
Usage *TaskUsage `json:"usage,omitempty"`
```

### 3.2 提取接口（`relay/channel/adapter.go`）

```go
// TaskUsageExtractor 可选接口：从任务存储数据（task.Data）提取真实 token 消耗。
type TaskUsageExtractor interface {
    ExtractTaskUsage(task *model.Task) *dto.TaskUsage
}
```

实现适配器（复用各自已有信封解析，不重复造解析逻辑）：

| 平台 | 数据来源（复用现有代码） |
|---|---|
| newapi | `taskResultEnvelope.usageTokens()`（平铺 `usage_*_tokens` / 嵌套 `data.data.usage`） |
| tokenhub | `ResultSummary.Usage.{completion_tokens,total_tokens}` |
| doubao | `Usage.{completion_tokens,total_tokens}` |
| xfai | `Usage.{completion_tokens,total_tokens}` |
| kling / ali / gemini / jimeng / sora / vidu / hailuo | 不实现 → usage 不返回 |

### 3.3 查询链路接入（`relay/relay_task.go` → `TaskModel2Dto`）

```go
func TaskModel2Dto(task *model.Task) *dto.TaskDto {
    result := &dto.TaskDto{ /* 现有字段不变 */ }
    if adaptor := GetTaskAdaptor(task.Platform); adaptor != nil {
        if ue, ok := adaptor.(channel.TaskUsageExtractor); ok {
            result.Usage = ue.ExtractTaskUsage(task)
        }
    }
    return result
}
```

- 生效路径：`/v1/video/generations/{task_id}`（通用 TaskDto 路径）。
- suno 也走 `TaskModel2Dto`，但适配器不实现该接口 → 不受影响。
- Gemini/Vertex realtime 自定义格式不在本次范围。
- 解析失败返回 nil（usage 不返回），查询不受影响。

### 3.4 文档更新（`web/api-docs/content/docs/{zh,en}/video/query.mdx`）

- 响应示例增加：

```json
"usage": {
  "completion_tokens": 108900,
  "total_tokens": 108900
}
```

- 字段说明注明：**仅 doubao / seedance 等返回真实 token 的上游在任务完成后才有此字段**；kling / ali 等无此字段；token 与网关计费（quota）有关联但单位不同。

## 4. 错误处理

| 场景 | 处理 |
|---|---|
| `task.Data` 为空 / 解析失败 | `ExtractTaskUsage` 返回 nil，usage 不返回，查询正常 |
| 平台未实现接口 | 类型断言失败，usage 不返回 |
| 上游返回 0 token | **统一规则：仅当 `total_tokens > 0` 时才返回非 nil usage，否则返回 nil**（由 omitempty 省略字段） |

## 5. 测试策略（TDD）

| 需求 | 测试文件 | 关键断言 |
|---|---|---|
| newapi 提取 | `relay/channel/task/newapi/adaptor_test.go` | 伪造平铺/嵌套 `task.Data` 信封 → `ExtractTaskUsage` 断言 token |
| tokenhub 提取 | `relay/channel/task/tokenhub/adaptor_test.go` | 伪造 `resultSummary.usage` → 断言 |
| doubao / xfai 提取 | 对应 `adaptor_test.go` | 伪造 `usage` → 断言 |
| TaskModel2Dto 接入 | `relay/relay_task_test.go`（新增） | 带 usage 的 task → TaskDto.Usage 非空；kling/suno 平台 → nil |
| 无 token 信封 | 各适配器测试 | `ExtractTaskUsage` 返回 nil，不 panic |

验证命令：

```bash
cd blue-whale-worktrees/ik9zgk-video-usage
go test ./relay/channel/task/newapi/... ./relay/channel/task/tokenhub/... ./relay/channel/task/doubao/... ./relay/channel/task/xfai/...
go test ./relay/ -run TaskModel2Dto
```

## 6. 不在本次范围

- Gemini/Vertex realtime 查询自定义格式的 usage。
- kling `final_unit_deduction` 透出（已定：不返回）。
- OpenAI 兼容路径 `/v1/videos/{task_id}`（`OpenAIVideo`）加 usage（后续如需另开工单）。
- openapi 生成文档（`getvideogeneration.mdx`）重新生成。

---

*本 spec 由 brainstorming 流程产出（需求已与用户逐点确认）。下一步：用户审阅 → writing-plans 生成实现计划。*
