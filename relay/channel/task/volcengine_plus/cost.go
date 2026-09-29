package volcengine_plus

import (
	"github.com/QuantumNous/new-api/common"
)

const (
	RateTier720p  = "720p"
	RateTier1080p = "1080p"
	RateTier2k    = "2k"
	RateTier4k    = "4k"

	defaultEnhanceFPS = 24
)

// yuanPerMinute 是官方「输出分辨率档 × 帧率档 → 换算后单价（元/分钟）」价表。
// 原表给的是换算系数（1 系 = 2.5 元/分钟），这里直接存系数 × 2.5 后的单价。
// 2K 档本渠道不出，代码带着备用；未知档位返回 0。
func yuanPerMinute(rateTier string, fps int) float64 {
	if fps <= 0 {
		fps = defaultEnhanceFPS
	}
	switch rateTier {
	case RateTier720p:
		switch {
		case fps <= 30:
			return 2.5
		case fps <= 60:
			return 5
		case fps <= 120:
			return 10
		}
	case RateTier1080p:
		switch {
		case fps <= 30:
			return 5
		case fps <= 60:
			return 10
		case fps <= 120:
			return 20
		}
	case RateTier2k:
		switch {
		case fps <= 30:
			return 10
		case fps <= 60:
			return 20
		case fps <= 120:
			return 40
		}
	case RateTier4k:
		switch {
		case fps <= 30:
			return 20
		case fps <= 60:
			return 40
		case fps <= 120:
			return 80
		}
	}
	return 0
}

func RateTierForRequested(requested string) string {
	switch NormalizeResolution(requested) {
	case Res720p:
		return RateTier720p
	case Res1080p:
		return RateTier1080p
	case Res4k:
		return RateTier4k
	default:
		return ""
	}
}

// EnhanceAmountFen returns the RMB fen for a successful enhance.
// Failed attempts use 0. Example: 5s / 1080p / ≤30fps → 42 fen.
func EnhanceAmountFen(durationMs int64, fps int, rateTier string) int {
	if durationMs <= 0 || rateTier == "" {
		return 0
	}
	yuan := yuanPerMinute(rateTier, fps)
	if yuan <= 0 {
		return 0
	}
	minutes := float64(durationMs) / 60000.0
	fen, _ := common.QuotaRoundChecked(minutes * yuan * 100)
	if fen < 0 {
		return 0
	}
	return fen
}
