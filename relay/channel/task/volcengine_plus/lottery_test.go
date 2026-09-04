package volcengine_plus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDrawUpstreamResolution1080pBoundaries(t *testing.T) {
	cfg := LotteryConfig{To720From1080: 30, To480From1080: 20, To480From720: 10}
	tests := []struct {
		name string
		roll int
		want string
	}{
		{name: "0 maps to 720p", roll: 0, want: Res720p},
		{name: "N-1 stays 720p", roll: 29, want: Res720p},
		{name: "N maps to 480p", roll: 30, want: Res480p},
		{name: "N+M-1 stays 480p", roll: 49, want: Res480p},
		{name: "N+M keeps 1080p", roll: 50, want: Res1080p},
		{name: "99 keeps 1080p", roll: 99, want: Res1080p},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DrawUpstreamResolution(Res1080p, cfg, tt.roll))
		})
	}
}

func TestDrawUpstreamResolution1080pFullDowngrade(t *testing.T) {
	cfg := LotteryConfig{To720From1080: 40, To480From1080: 60}
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res1080p, cfg, 0))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res1080p, cfg, 40))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res1080p, cfg, 99))
}

func TestDrawUpstreamResolutionAllZero(t *testing.T) {
	cfg := LotteryConfig{}
	assert.Equal(t, Res1080p, DrawUpstreamResolution(Res1080p, cfg, 0))
	assert.Equal(t, Res1080p, DrawUpstreamResolution(Res1080p, cfg, 99))
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res720p, cfg, 0))
}

func TestDrawUpstreamResolution720p(t *testing.T) {
	cfg := LotteryConfig{To480From720: 10}
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res720p, cfg, 0))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res720p, cfg, 9))
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res720p, cfg, 10))
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res720p, cfg, 99))
}

func TestDrawUpstreamResolutionNeverLotteries4kOr480p(t *testing.T) {
	cfg := LotteryConfig{To720From1080: 100, To480From1080: 0, To480From720: 100}
	assert.Equal(t, Res4k, DrawUpstreamResolution(Res4k, cfg, 0))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res480p, cfg, 0))
}

func TestDrawUpstreamResolutionRandInjected(t *testing.T) {
	cfg := LotteryConfig{To720From1080: 30, To480From1080: 20}
	got := drawUpstreamResolution(Res1080p, cfg, func(n int) int {
		require.Equal(t, 100, n)
		return 30
	})
	assert.Equal(t, Res480p, got)
}

func TestIsDowngrade(t *testing.T) {
	assert.True(t, IsDowngrade(Res1080p, Res720p))
	assert.True(t, IsDowngrade(Res1080p, Res480p))
	assert.True(t, IsDowngrade(Res720p, Res480p))
	assert.False(t, IsDowngrade(Res1080p, Res1080p))
	assert.False(t, IsDowngrade(Res720p, Res720p))
	assert.False(t, IsDowngrade(Res4k, Res4k))
	assert.False(t, IsDowngrade(Res480p, Res480p))
}
