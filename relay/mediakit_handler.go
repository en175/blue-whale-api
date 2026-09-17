package relay

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// MediaKitSyncHelper 服务 MediaKit 的同步工具（图像类），一次往返返回产物。
//
// 与其它 handler 的关键差别是请求体零改写：客户端按 MediaKit 原生协议写请求，
// 工具名只出现在路径上（由 distributor 解析成对客模型名 mediakit-<tool>），
// body 里根本没有 model 字段。网关只负责换鉴权头、回写响应、按次计费。
//
// 这里刻意不调 helper.ModelMappedHelper：模型名在 MediaKit 里是协议的一部分
// （它决定打到哪个上游端点），不是可以按渠道改写的别名。一旦被 model_mapping
// 改写，GetRequestURL 的注册表反查就会失败。
func MediaKitSyncHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// MediaKit 的请求体里没有 token 概念，按次计价的唯一来源是模型定价。
	// 只配了倍率时，预扣按倍率算、结算却按上游返回的 1 个 token 折算并把差额
	// 退回，实际等于白送。这里显式拦下，避免静默少收费；管理员确实想免费就把
	// model_price 配成 0。
	if _, ok := ratio_setting.GetModelPrice(info.OriginModelName, false); !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("mediakit model %s has no model price configured", info.OriginModelName),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	// 上游路径由注册表按 info.UpstreamModelName 反查得出，客户端无法参与拼接。
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, common.ReaderOnly(storage))
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	httpResp, ok := resp.(*http.Response)
	if !ok || httpResp == nil {
		return types.NewError(fmt.Errorf("mediakit upstream returned no response"), types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// 上游以 HTTP 200 + success=false 表达业务失败，DoResponse 已转为错误；
		// controller.Relay 的 defer 据此退款——未成功的调用不计费。
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usageDTO, ok := usage.(*dto.Usage)
	if !ok || usageDTO == nil {
		return types.NewError(fmt.Errorf("unexpected usage type %T from mediakit adaptor", usage), types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}
	service.PostTextConsumeQuota(c, info, usageDTO, []string{fmt.Sprintf("工具 %s", info.UpstreamModelName)})
	return nil
}
