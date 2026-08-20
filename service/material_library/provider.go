package material_library

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/material_setting"
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

// currentProvider 按配置返回当前素材库上游。tokenhub 是默认实现；volcengine 在后续任务接入。
func currentProvider() Provider {
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
