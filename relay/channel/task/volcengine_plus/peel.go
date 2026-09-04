package volcengine_plus

import (
	"github.com/QuantumNous/new-api/common"
)

// PeelTokens scales upstream Seedance tokens from the upstream-tier canvas to
// the request-tier canvas. Same model family and ratio; round half-away-from-zero.
func PeelTokens(upstreamTokens int, requestedW, requestedH, upstreamW, upstreamH int) int {
	if upstreamTokens <= 0 {
		return 0
	}
	if requestedW <= 0 || requestedH <= 0 || upstreamW <= 0 || upstreamH <= 0 {
		return upstreamTokens
	}
	if requestedW == upstreamW && requestedH == upstreamH {
		return upstreamTokens
	}
	n, _ := common.QuotaRoundChecked(float64(upstreamTokens) * float64(requestedW) * float64(requestedH) / float64(upstreamW) / float64(upstreamH))
	if n < 0 {
		return 0
	}
	return n
}

func PeelTokensForTiers(upstreamTokens int, modelName, requestedRes, upstreamRes, ratio string) (int, error) {
	if !IsDowngrade(requestedRes, upstreamRes) {
		return upstreamTokens, nil
	}
	reqW, reqH, err := CanvasPixels(modelName, requestedRes, ratio)
	if err != nil {
		return 0, err
	}
	upW, upH, err := CanvasPixels(modelName, upstreamRes, ratio)
	if err != nil {
		return 0, err
	}
	return PeelTokens(upstreamTokens, reqW, reqH, upW, upH), nil
}
