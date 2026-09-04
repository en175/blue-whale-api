package volcengine_plus

import (
	"github.com/QuantumNous/new-api/common"
)

const (
	RateTier720p  = "720p"
	RateTier1080p = "1080p"
	RateTier2k    = "2k"

	defaultEnhanceFPS = 24
)

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
	}
	return 0
}

func RateTierForRequested(requested string) string {
	switch NormalizeResolution(requested) {
	case Res720p:
		return RateTier720p
	case Res1080p:
		return RateTier1080p
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
