package material_library

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/material_setting"

	"github.com/volcengine/volc-sdk-golang/base"
)

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

// newRequest 构造上游请求：Action/Version 注入 query（客户端 query 保留）、注入 Bearer 凭证。
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
	q.Set("Version", arkVersion)
	target.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AK+":"+cfg.SK)
	return req, nil
}

// volcengineProvider 对接火山引擎官方素材库（管控面 API）：
// POST {volcengine_base_url}/?Action=...&Version=...，火山账号 AK/SK 做 SigV4 签名（Service=ark）。
type volcengineProvider struct{}

func (volcengineProvider) DoAction(ctx context.Context, action string, query url.Values, body []byte) (*Response, error) {
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

// newRequest 构造火山管控面请求：Action/Version 注入 query（客户端 query 保留）、
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
	q.Set("Version", arkVersion)
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
