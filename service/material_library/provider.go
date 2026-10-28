package material_library

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/volcengine/volc-sdk-golang/base"
)

// materialUpstreamTimeout 素材上游调用超时。GetHttpClient 默认无超时（RelayTimeout=0），
// 无效 URL 等上游挂起时由该超时兜底快速失败，避免客户端无限等待。
const materialUpstreamTimeout = 30 * time.Second

// Provider 抽象素材库上游：执行一个 Action，返回上游完整响应。
// 各 provider 负责自己的地址映射与鉴权（tokenhub=Bearer，volcengine=SigV4 签名）。
// 非 2xx 不视为 error，调用方依 StatusCode 透传或解析 ArkError。
type Provider interface {
	DoAction(ctx context.Context, action string, query url.Values, body []byte) (*Response, error)
}

// Response 上游响应。代理需要原样回写状态码/头/体，故保留完整信息。
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// currentProvider 按配置返回当前素材库上游（tokenhub / volcengine）。
func currentProvider() Provider {
	return providerByName(material_setting.GetConfig().Provider)
}

// providerByName 按名称返回 Provider 实现。回调轮询等场景使用会话记录的 provider，
// 避免全局配置切换后轮询走错上游。
func providerByName(name string) Provider {
	if name == material_setting.ProviderVolcengine {
		return volcengineProvider{}
	}
	return tokenhubProvider{}
}

// tokenhubProvider 对接 ai-tokenhub（火山 ARK 兼容接口）：
// POST {base_url}/ark?Action=...&Version=...，Bearer {ak}:{sk} 凭证注入。
type tokenhubProvider struct{}

func (tokenhubProvider) DoAction(ctx context.Context, action string, query url.Values, body []byte) (*Response, error) {
	// tokenhub 的 ListAssets 过滤枚举与对客契约不一致，出站前统一翻译。
	if action == "ListAssets" {
		body = translateTokenhubAssetStatuses(body)
	}
	ctx, cancel := context.WithTimeout(ctx, materialUpstreamTimeout)
	defer cancel()
	req, err := tokenhubProvider{}.newRequest(ctx, action, query, body)
	if err != nil {
		return nil, err
	}
	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// newRequest 构造上游请求：Action 注入 query；Version 缺省时补默认值，下游传值原样透传；
// 其余客户端 query 全部保留。注入 Bearer 凭证。
// 抽取出来供单测直接验证地址映射与凭证头。
func (tokenhubProvider) newRequest(ctx context.Context, action string, query url.Values, body []byte) (*http.Request, error) {
	cfg := material_setting.GetConfig()
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	target := *base
	target.Path = "/ark"
	q := target.Query()
	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("Action", action)
	// Version 完全透传：下游传什么上游就收到什么；缺省时补默认值，保证火山形状完整。
	if q.Get("Version") == "" {
		q.Set("Version", arkVersion)
	}
	target.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AK+":"+cfg.SK)
	return req, nil
}

// tokenhubAssetStatusAliases 把对客的火山素材状态枚举翻译成 tokenhub 实际匹配的
// 内部值。上游文档写 Active/Processing/Failed，但 ListAssets 过滤只认小写内部
// 枚举，必须在出站请求里兼容。上游修复后本表可整体删除——不在表内的值原样放行。
//
// 实测边界（图片素材、2026-10-27）：
//   - Filter.Statuses=["success"] 命中，但命中的素材里 Active 和 Processing 都有，
//     上游的 success 不是状态一一对应；
//   - ["active"]、["processing"] 返回 0 条（上游 enum 大小写敏感，只认小写内部值）；
//   - ["verifying"]、["failed"] 也返回 0 条，但当时账号内没有这两种状态的素材，
//     属于无效探测——processing→verifying、failed→failed 两条映射未经证实，
//     上线后需用真实素材复核；
//   - 响应里的 Result.Items[].Status 未做反向翻译，客户端按 Active 过滤成功后，
//     读回的 Status 仍可能是上游内部值。
var tokenhubAssetStatusAliases = map[string]string{
	"active":     "success",
	"processing": "verifying",
	"failed":     "failed",
}

// translateTokenhubAssetStatuses 改写 ListAssets 请求体里的 Filter.Statuses。
// 只动命中的状态值，其余字段以原始 JSON 片段保留；请求体不是预期形状时原样
// 返回，交上游报参数错误。
func translateTokenhubAssetStatuses(body []byte) []byte {
	var top map[string]json.RawMessage
	if err := common.Unmarshal(body, &top); err != nil || top == nil {
		return body
	}
	filterKey, filterRaw := rawFieldFold(top, "Filter")
	if filterRaw == nil {
		return body
	}
	var filter map[string]json.RawMessage
	if err := common.Unmarshal(filterRaw, &filter); err != nil || filter == nil {
		return body
	}
	statusKey, statusRaw := rawFieldFold(filter, "Statuses")
	if statusRaw == nil {
		return body
	}
	var statuses []string
	if err := common.Unmarshal(statusRaw, &statuses); err != nil {
		return body
	}

	changed := false
	for i, status := range statuses {
		mapped, ok := tokenhubAssetStatusAliases[strings.ToLower(strings.TrimSpace(status))]
		if ok && mapped != status {
			statuses[i] = mapped
			changed = true
		}
	}
	if !changed {
		return body
	}

	encoded, err := common.Marshal(statuses)
	if err != nil {
		return body
	}
	filter[statusKey] = encoded
	encoded, err = common.Marshal(filter)
	if err != nil {
		return body
	}
	top[filterKey] = encoded
	encoded, err = common.Marshal(top)
	if err != nil {
		return body
	}
	return encoded
}

// rawFieldFold 按大小写不敏感查找字段，返回实际键名与原始片段。
func rawFieldFold(fields map[string]json.RawMessage, name string) (string, json.RawMessage) {
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			return key, value
		}
	}
	return "", nil
}

// volcengineProvider 对接火山引擎官方素材库（管控面 API）：
// POST {volcengine_base_url}/?Action=...&Version=...，火山账号 AK/SK 做 SigV4 签名（Service=ark）。
type volcengineProvider struct{}

func (volcengineProvider) DoAction(ctx context.Context, action string, query url.Values, body []byte) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, materialUpstreamTimeout)
	defer cancel()
	req, err := volcengineProvider{}.newRequest(ctx, action, query, body)
	if err != nil {
		return nil, err
	}
	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// newRequest 构造火山管控面请求：Action 注入 query；Version 缺省时补默认、下游传值原样透传；
// 用账号 AK/SK 签名（volc-sdk-golang base.Credentials.Sign，SigV4，Service=ark）。
func (volcengineProvider) newRequest(ctx context.Context, action string, query url.Values, body []byte) (*http.Request, error) {
	cfg := material_setting.GetConfig()
	target, err := url.Parse(cfg.VolcengineBaseURL)
	if err != nil {
		return nil, err
	}
	q := target.Query()
	for k, vs := range query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	q.Set("Action", action)
	// Version 完全透传：下游传什么上游就收到什么；缺省时补默认值，保证火山形状完整。
	if q.Get("Version") == "" {
		q.Set("Version", arkVersion)
	}
	target.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// 管控面 API 走 SigV4 签名（非 Bearer）：Service=ark，Region 取配置（默认 cn-beijing）。
	base.Credentials{
		AccessKeyID:     cfg.VolcengineAccessKey,
		SecretAccessKey: cfg.VolcengineSecretKey,
		Service:         "ark",
		Region:          cfg.VolcengineRegion,
	}.Sign(req)
	return req, nil
}
