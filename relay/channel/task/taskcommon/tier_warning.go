package taskcommon

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const tierWarnInterval = 10 * time.Minute

// missingTierWarned 去重档位缺失告警：同一 model + 档位 + 输入维度在
// tierWarnInterval 内只告警一次，避免每次请求刷屏；超过间隔重新告警，
// 管理员修好配置后又配错时无需重启进程。
var missingTierWarned sync.Map

// WarnMissingTier 在按档位收费、但该档位没有显式单价时告警，避免静默按基准价
// 结算（PRD 5.6）。只认「漏配」：倍率为 1.0 可能是配置了与基准价相同的单价，
// 用 VideoTierPriceConfigured 区分，不误报。
// 样片固定 480p，与 720p 同属基准键（video_tier_pricing 的 base 覆盖
// 480p/720p），基准价本就是它的正确价格，因此样片不调用本函数。
func WarnMissingTier(c *gin.Context, info *relaycommon.RelayInfo, resolution string, hasVideo bool, ratios map[string]float64) {
	if info == nil || ratios != nil {
		return
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	if VideoTierPriceConfigured(info.UpstreamModelName, resolution, hasVideo) ||
		VideoTierPriceConfigured(info.OriginModelName, resolution, hasVideo) {
		return // 档位有显式单价，只是恰好等于基准价
	}
	key := fmt.Sprintf("%s|%s|%t", model, resolution, hasVideo)
	now := time.Now()
	if last, loaded := missingTierWarned.Load(key); loaded && now.Sub(last.(time.Time)) < tierWarnInterval {
		return
	}
	missingTierWarned.Store(key, now)
	logger.LogWarn(c.Request.Context(), "video tier pricing missing: model="+model+
		" resolution="+resolution+" has_video="+strconv.FormatBool(hasVideo)+
		" action="+string(info.Action)+", billing falls back to base price")
}
