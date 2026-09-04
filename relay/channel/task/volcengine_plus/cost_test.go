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
	assert.Equal(t, "", RateTierForRequested(Res4k))
	assert.Equal(t, "", RateTierForRequested(Res480p))
}
