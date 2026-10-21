package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

const (
	DeepSeekToolSearch = "search"
	DeepSeekToolFetch  = "fetch"

	DeepSeekSearchModelName = "deepseek-search"
	DeepSeekFetchModelName  = "deepseek-fetch"

	deepSeekToolMaxRequestBytes  = 1 << 20  // 1MB：search 只有 query/count，fetch 只有 url
	deepSeekToolMaxResponseBytes = 10 << 20 // 10MB
	deepSeekToolTimeout          = 120 * time.Second
)

type deepSeekToolSpec struct {
	priceKey  string
	modelName string
}

var deepSeekToolSpecs = map[string]deepSeekToolSpec{
	DeepSeekToolSearch: {priceKey: "deepseek_search", modelName: DeepSeekSearchModelName},
	DeepSeekToolFetch:  {priceKey: "deepseek_fetch", modelName: DeepSeekFetchModelName},
}

// DeepSeekToolProxy relays /tools/search and /tools/fetch to a DeepSeek channel.
// Requests are passed through unchanged; the gateway only validates, selects a
// channel, and bills one call when the upstream reports a successful result.
func DeepSeekToolProxy(c *gin.Context, tool string) *types.NewAPIError {
	spec, ok := deepSeekToolSpecs[tool]
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("unsupported tool: %s", tool),
			types.ErrorCodeInvalidRequest,
			http.StatusNotFound,
			types.ErrOptionWithSkipRetry(),
		)
	}

	body, apiErr := readDeepSeekToolBody(c)
	if apiErr != nil {
		return apiErr
	}
	if err := validateDeepSeekToolRequest(tool, body); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	channel, group, apiErr := selectDeepSeekToolChannel(c)
	if apiErr != nil {
		return apiErr
	}
	if apiErr := setupDeepSeekToolChannel(c, channel, spec.modelName); apiErr != nil {
		return apiErr
	}

	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	groupRatio, groupRatioSource := ResolveGroupRatio(userGroup, group, "")

	relayInfo := deepSeekToolRelayInfo(c, group, spec.modelName, groupRatio, groupRatioSource)
	price := operation_setting.GetToolPrice(spec.priceKey)
	quota := deepSeekToolQuota(c, price, groupRatio, relayInfo)
	if quota > 0 {
		if apiErr := PreConsumeBilling(c, quota, relayInfo); apiErr != nil {
			return apiErr
		}
	}

	billingHandled := quota == 0
	defer func() {
		if !billingHandled && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	respBody, upstreamStatus, contentType, apiErr := callDeepSeekTool(c, channel, tool, body)
	if apiErr != nil {
		return apiErr
	}
	if upstreamStatus != http.StatusOK {
		// 上游失败不扣费，响应原样透传。
		logger.LogWarn(c, fmt.Sprintf("deepseek %s upstream returned status %d", tool, upstreamStatus))
		writeDeepSeekToolResponse(c, upstreamStatus, contentType, respBody)
		return nil
	}

	billable := isDeepSeekToolBillable(tool, respBody)
	billedQuota := 0
	if billable {
		if err := SettleBilling(c, relayInfo, quota); err != nil {
			logger.LogError(c, "failed to settle deepseek tool billing: "+err.Error())
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
		} else {
			billedQuota = quota
			model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
			DeductChannelQuota(channel.Id, quota, groupRatio)
		}
	} else if err := SettleBilling(c, relayInfo, 0); err != nil {
		logger.LogError(c, "failed to refund deepseek tool pre-consume: "+err.Error())
		if relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, 0)
	}
	billingHandled = true

	recordDeepSeekToolLog(c, relayInfo, channel, tool, price, groupRatio, billedQuota, billable)
	writeDeepSeekToolResponse(c, upstreamStatus, contentType, respBody)
	return nil
}

func readDeepSeekToolBody(c *gin.Context) ([]byte, *types.NewAPIError) {
	if c.Request.ContentLength > deepSeekToolMaxRequestBytes {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("request body exceeds %d bytes", deepSeekToolMaxRequestBytes),
			types.ErrorCodeInvalidRequest,
			http.StatusRequestEntityTooLarge,
			types.ErrOptionWithSkipRetry(),
		)
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, deepSeekToolMaxRequestBytes+1))
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if len(body) > deepSeekToolMaxRequestBytes {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("request body exceeds %d bytes", deepSeekToolMaxRequestBytes),
			types.ErrorCodeInvalidRequest,
			http.StatusRequestEntityTooLarge,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return body, nil
}

func validateDeepSeekToolRequest(tool string, body []byte) error {
	var req struct {
		Query *string `json:"query"`
		Count *int    `json:"count"`
		URL   *string `json:"url"`
	}
	if err := common.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}

	switch tool {
	case DeepSeekToolSearch:
		if req.Query == nil || *req.Query == "" {
			return errors.New("query is required")
		}
		if utf8.RuneCountInString(*req.Query) > 128 {
			return errors.New("query must be at most 128 characters")
		}
		if req.Count != nil && (*req.Count < 1 || *req.Count > 50) {
			return errors.New("count must be between 1 and 50")
		}
	case DeepSeekToolFetch:
		if req.URL == nil || *req.URL == "" {
			return errors.New("url is required")
		}
		if len(*req.URL) > 8192 {
			return errors.New("url must be at most 8192 characters")
		}
		parsed, err := url.Parse(*req.URL)
		if err != nil || parsed.Host == "" {
			return errors.New("url must be a valid absolute URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("url must use http or https")
		}
		if parsed.User != nil {
			return errors.New("url must not contain credentials")
		}
	}
	return nil
}

func selectDeepSeekToolChannel(c *gin.Context) (*model.Channel, string, *types.NewAPIError) {
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	groups := []string{usingGroup}
	if usingGroup == "auto" {
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		groups = GetUserAutoGroup(userGroup)
		if len(groups) == 0 {
			return nil, usingGroup, types.NewErrorWithStatusCode(
				errors.New("auto groups is not enabled"),
				types.ErrorCodeGetChannelFailed,
				http.StatusServiceUnavailable,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	deniedByToken := false
	for _, group := range groups {
		allowed, err := tokenAllowsDeepSeekTool(c, group)
		if err != nil {
			return nil, group, types.NewErrorWithStatusCode(
				fmt.Errorf("校验分组 %s 的令牌模型权限失败: %s", group, err.Error()),
				types.ErrorCodeGetChannelFailed,
				http.StatusServiceUnavailable,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if !allowed {
			deniedByToken = true
			continue
		}
		channel, err := model.GetRandomSatisfiedChannelByType(group, constant.ChannelTypeDeepSeek, 0)
		if err != nil {
			return nil, group, types.NewErrorWithStatusCode(
				fmt.Errorf("获取分组 %s 下的 DeepSeek 渠道失败: %s", group, err.Error()),
				types.ErrorCodeGetChannelFailed,
				http.StatusServiceUnavailable,
				types.ErrOptionWithSkipRetry(),
			)
		}
		if channel != nil {
			return channel, group, nil
		}
	}

	if deniedByToken {
		return nil, usingGroup, types.NewErrorWithStatusCode(
			errors.New("当前令牌无权调用 DeepSeek 工具，请为令牌开放至少一个 DeepSeek 模型"),
			types.ErrorCodeAccessDenied,
			http.StatusForbidden,
			types.ErrOptionWithSkipRetry(),
		)
	}

	return nil, usingGroup, types.NewErrorWithStatusCode(
		fmt.Errorf("分组 %s 下没有可用的 DeepSeek 渠道", usingGroup),
		types.ErrorCodeGetChannelFailed,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)
}

// tokenAllowsDeepSeekTool mirrors the token model whitelist for endpoints that
// have no requested model. A restricted token may call the tools only when it
// can access at least one model served by a DeepSeek channel in this group, or
// when the whitelist explicitly names the tool pseudo models.
func tokenAllowsDeepSeekTool(c *gin.Context, group string) (bool, error) {
	if !common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
		return true, nil
	}
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	limits, typeOK := value.(map[string]bool)
	if !ok || !typeOK || len(limits) == 0 {
		return false, nil
	}
	if limits[DeepSeekSearchModelName] || limits[DeepSeekFetchModelName] {
		return true, nil
	}
	models, err := model.GetEnabledChannelTypeModels(group, constant.ChannelTypeDeepSeek)
	if err != nil {
		return false, err
	}
	for name := range models {
		if limits[name] {
			return true, nil
		}
	}
	return false, nil
}

// setupDeepSeekToolChannel mirrors middleware.SetupContextForSelectedChannel for
// the subset this endpoint needs. It cannot call the middleware directly because
// middleware already imports service.
func setupDeepSeekToolChannel(c *gin.Context, channel *model.Channel, modelName string) *types.NewAPIError {
	c.Set("original_model", modelName)
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())

	key, keyIndex, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		return apiErr
	}
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, keyIndex)
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())
	return nil
}

func deepSeekToolRelayInfo(c *gin.Context, group string, modelName string, groupRatio float64, groupRatioSource types.GroupRatioSource) *relaycommon.RelayInfo {
	info := relaycommon.GenRelayInfoTool(c, modelName)
	info.InitChannelMeta(c)
	info.UsingGroup = group
	info.PriceData.GroupRatioInfo = types.NewGroupRatioInfo(groupRatio, groupRatioSource)
	return info
}

func deepSeekToolQuota(c *gin.Context, price float64, groupRatio float64, relayInfo *relaycommon.RelayInfo) int {
	if price < 0 || groupRatio < 0 || math.IsNaN(price) || math.IsNaN(groupRatio) || math.IsInf(price, 0) || math.IsInf(groupRatio, 0) {
		logger.LogError(c, fmt.Sprintf("invalid deepseek tool pricing: price=%v group_ratio=%v", price, groupRatio))
		return 0
	}
	quotaDecimal := decimal.NewFromFloat(price).
		Div(decimal.NewFromInt(1000)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(groupRatio))
	quota, clamp := common.QuotaFromDecimalChecked(quotaDecimal)
	noteQuotaClamp(relayInfo, clamp)
	if quota < 0 {
		// 负 quota 会在 SettleBilling 的无会话回退路径里变成给用户加额度。
		logger.LogError(c, fmt.Sprintf("deepseek tool quota cannot be negative: price=%v group_ratio=%v quota=%d", price, groupRatio, quota))
		return 0
	}
	return quota
}

func callDeepSeekTool(c *gin.Context, channel *model.Channel, tool string, body []byte) ([]byte, int, string, *types.NewAPIError) {
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeDeepSeek]
	}
	target := strings.TrimRight(baseURL, "/") + "/v1/tools/" + tool
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)

	ctx, cancel := context.WithTimeout(c.Request.Context(), deepSeekToolTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, 0, "", types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := GetHttpClient().Do(req)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("deepseek %s request failed: %s", tool, err.Error()))
		return nil, 0, "", types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, deepSeekToolMaxResponseBytes+1))
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to read deepseek %s response: %s", tool, err.Error()))
		return nil, 0, "", types.NewErrorWithStatusCode(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
	}
	if len(respBody) > deepSeekToolMaxResponseBytes {
		logger.LogError(c, fmt.Sprintf("deepseek %s response exceeds %d bytes", tool, deepSeekToolMaxResponseBytes))
		return nil, 0, "", types.NewErrorWithStatusCode(
			fmt.Errorf("upstream response exceeds %d bytes", deepSeekToolMaxResponseBytes),
			types.ErrorCodeBadResponseBody,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		)
	}

	return respBody, resp.StatusCode, resp.Header.Get("Content-Type"), nil
}

func isDeepSeekToolBillable(tool string, body []byte) bool {
	switch tool {
	case DeepSeekToolSearch:
		results := gjson.GetBytes(body, "results")
		return results.IsArray() && len(results.Array()) > 0
	case DeepSeekToolFetch:
		content := gjson.GetBytes(body, "content")
		return content.Type == gjson.String && content.String() != ""
	}
	return false
}

func recordDeepSeekToolLog(c *gin.Context, relayInfo *relaycommon.RelayInfo, channel *model.Channel, tool string, price float64, groupRatio float64, billedQuota int, billable bool) {
	other := map[string]interface{}{
		"tool":        tool,
		"call_count":  1,
		"tool_price":  price,
		"group_ratio": groupRatio,
		"billable":    billable,
	}
	attachQuotaSaturation(c, relayInfo, other)

	content := fmt.Sprintf("工具 %s，单价 %.2f 美元/千次，分组倍率 %.2f", tool, price, groupRatio)
	if !billable {
		content += "（上游无有效结果，未计费）"
	}

	model.RecordConsumeLog(c, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:      channel.Id,
		ModelName:      relayInfo.OriginModelName,
		TokenName:      c.GetString("token_name"),
		Quota:          billedQuota,
		Content:        content,
		TokenId:        relayInfo.TokenId,
		UseTimeSeconds: int(time.Since(relayInfo.StartTime).Seconds()),
		IsStream:       false,
		Group:          relayInfo.UsingGroup,
		Other:          other,
	})
}

func writeDeepSeekToolResponse(c *gin.Context, status int, contentType string, body []byte) {
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(status, contentType, body)
}
