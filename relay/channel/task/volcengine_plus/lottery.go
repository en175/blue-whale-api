package volcengine_plus

import (
	"math/rand"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const (
	Res480p  = "480p"
	Res720p  = "720p"
	Res1080p = "1080p"
	Res4k    = "4k"

	DefaultRatio = "16:9"
)

// LotteryConfig is the resolution lottery percentages from channel setting.
type LotteryConfig struct {
	To720From1080 int
	To480From1080 int
	To480From720  int
	To1080From4k  int
	To720From4k   int
	To480From4k   int
}

func LotteryConfigFromSettings(s dto.ChannelSettings) LotteryConfig {
	return LotteryConfig{
		To720From1080: s.Lottery1080To720Percent,
		To480From1080: s.Lottery1080To480Percent,
		To480From720:  s.Lottery720To480Percent,
		To1080From4k:  s.Lottery4kTo1080Percent,
		To720From4k:   s.Lottery4kTo720Percent,
		To480From4k:   s.Lottery4kTo480Percent,
	}
}

func NormalizeResolution(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// DrawUpstreamResolution maps a request-tier resolution to the Seedance
// upstream tier using a single roll in [0, 100).
func DrawUpstreamResolution(requested string, cfg LotteryConfig, roll int) string {
	res := NormalizeResolution(requested)
	switch res {
	case Res1080p:
		n := cfg.To720From1080
		m := cfg.To480From1080
		if n < 0 {
			n = 0
		}
		if m < 0 {
			m = 0
		}
		if roll < n {
			return Res720p
		}
		if roll < n+m {
			return Res480p
		}
		return Res1080p
	case Res720p:
		p := cfg.To480From720
		if p < 0 {
			p = 0
		}
		if roll < p {
			return Res480p
		}
		return Res720p
	case Res4k:
		a := cfg.To1080From4k
		b := cfg.To720From4k
		c := cfg.To480From4k
		if a < 0 {
			a = 0
		}
		if b < 0 {
			b = 0
		}
		if c < 0 {
			c = 0
		}
		if roll < a {
			return Res1080p
		}
		if roll < a+b {
			return Res720p
		}
		if roll < a+b+c {
			return Res480p
		}
		return Res4k
	default:
		return res
	}
}

func needsLotteryRoll(requested string) bool {
	switch NormalizeResolution(requested) {
	case Res1080p, Res720p, Res4k:
		return true
	default:
		return false
	}
}

func drawUpstreamResolution(requested string, cfg LotteryConfig, intn func(int) int) string {
	if !needsLotteryRoll(requested) {
		return NormalizeResolution(requested)
	}
	if intn == nil {
		intn = rand.Intn
	}
	return DrawUpstreamResolution(requested, cfg, intn(100))
}

func IsDowngrade(requested, upstream string) bool {
	return resolutionRank(NormalizeResolution(upstream)) < resolutionRank(NormalizeResolution(requested))
}

func resolutionRank(res string) int {
	switch NormalizeResolution(res) {
	case Res480p:
		return 1
	case Res720p:
		return 2
	case Res1080p:
		return 3
	case Res4k:
		return 4
	default:
		return 0
	}
}
