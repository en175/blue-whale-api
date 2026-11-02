package arknative

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// legacyFinalizeOptionsKey 保存旧接口成片请求体解析出的输出规格参数。
const legacyFinalizeOptionsKey = "ark_legacy_finalize_options"

// FinalizeOptionKeys 是成片请求里可以重设的输出规格类参数。content / prompt /
// duration / ratio / seed / generate_audio 刻意不在其中：成片沿用样片的内容与
// 时长，客户端传什么都影响不了成片本身。
var FinalizeOptionKeys = []string{
	"output_format",
	"watermark",
	"return_last_frame",
	"service_tier",
	"execution_expires_after",
	"priority",
	"callback_url",
	"safety_identifier",
}

// ValidateLegacyFinalizeRequest 校验旧接口成片第二步。原生入口的 draft_task 引用
// 已在请求转换中间件里校验，这里直接放行；旧接口没有 prompt，走不了通用校验，
// 只解析输出规格类参数，并拒绝非 1080p 的分辨率——上游成片档位只有 1080p。
//
// 空请求体与 `{}` 都必须通过：客户端通常直接 POST 空体，由网关套用默认值。
func ValidateLegacyFinalizeRequest(c *gin.Context, allowedResolution string) *dto.TaskError {
	if _, ok := NativeRequestFrom(c); ok {
		return nil
	}

	options := map[string]any{}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_request_body_failed", http.StatusBadRequest)
	}
	if storage.Size() > 0 {
		if err := common.UnmarshalBodyReusable(c, &options); err != nil {
			return service.TaskErrorWrapper(err, "invalid_json", http.StatusBadRequest)
		}
	}

	if resolution, ok := options["resolution"].(string); ok {
		if normalized := strings.ToLower(strings.TrimSpace(resolution)); normalized != "" && normalized != allowedResolution {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("finalize only supports resolution %s", allowedResolution), "invalid_resolution", http.StatusBadRequest)
		}
	}

	c.Set(legacyFinalizeOptionsKey, options)
	return nil
}

// FinalizeOptionValue 按入口取成片可重设参数：原生入口读原始 JSON，旧接口读
// 校验阶段解析出的请求体。两者都只认 FinalizeOptionKeys 白名单。
func FinalizeOptionValue(c *gin.Context, native *NativeRequest, key string) any {
	if native != nil {
		return native.Raw[key]
	}
	value, _ := c.Get(legacyFinalizeOptionsKey)
	options, _ := value.(map[string]any)
	return options[key]
}
