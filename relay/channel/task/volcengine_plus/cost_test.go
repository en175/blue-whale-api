package volcengine_plus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnhanceAmountFen5s1080p24fps(t *testing.T) {
	// 5/60 × 5 yuan/min × 100 = 41.666… → 42 分
	assert.Equal(t, 42, EnhanceAmountFen(5000, 24, RateTier1080p))
}

func TestEnhanceAmountFenFailedIsZero(t *testing.T) {
	assert.Equal(t, 0, EnhanceAmountFen(0, 24, RateTier1080p))
	assert.Equal(t, 0, EnhanceAmountFen(5000, 24, ""))
}

func TestRateTierForRequested(t *testing.T) {
	assert.Equal(t, RateTier1080p, RateTierForRequested(Res1080p))
	assert.Equal(t, RateTier720p, RateTierForRequested(Res720p))
	assert.Equal(t, RateTier4k, RateTierForRequested(Res4k))
	assert.Equal(t, RateTier4k, RateTierForRequested("  4K "))
	assert.Equal(t, "", RateTierForRequested(Res480p))
	assert.Equal(t, "", RateTierForRequested(""))
}

func TestEnhanceAmountFen4k(t *testing.T) {
	// 5/60 × 20 yuan/min × 100 = 166.666… → 167 分
	assert.Equal(t, 167, EnhanceAmountFen(5000, 24, RateTier4k))
	// 60 分钟 × 80 yuan/min × 100 = 480000 分
	assert.Equal(t, 480000, EnhanceAmountFen(3600000, 120, RateTier4k))
}

func TestYuanPerMinute4kFpsTiers(t *testing.T) {
	assert.Equal(t, 20.0, yuanPerMinute(RateTier4k, 30))
	assert.Equal(t, 40.0, yuanPerMinute(RateTier4k, 31))
	assert.Equal(t, 40.0, yuanPerMinute(RateTier4k, 60))
	assert.Equal(t, 80.0, yuanPerMinute(RateTier4k, 61))
	assert.Equal(t, 80.0, yuanPerMinute(RateTier4k, 120))
	// 超出官方价表最高档（>120fps）无价，记 0。
	assert.Equal(t, 0.0, yuanPerMinute(RateTier4k, 121))
}
