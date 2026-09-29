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

func TestDrawUpstreamResolution480pNeverLotteries(t *testing.T) {
	cfg := LotteryConfig{
		To720From1080: 100, To480From1080: 100, To480From720: 100,
		To1080From4k: 100, To720From4k: 100, To480From4k: 100,
	}
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res480p, cfg, 0))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res480p, cfg, 99))
}

func TestDrawUpstreamResolution4kBoundaries(t *testing.T) {
	cfg := LotteryConfig{To1080From4k: 30, To720From4k: 20, To480From4k: 10}
	tests := []struct {
		name string
		roll int
		want string
	}{
		{name: "0 maps to 1080p", roll: 0, want: Res1080p},
		{name: "A-1 stays 1080p", roll: 29, want: Res1080p},
		{name: "A maps to 720p", roll: 30, want: Res720p},
		{name: "A+B-1 stays 720p", roll: 49, want: Res720p},
		{name: "A+B maps to 480p", roll: 50, want: Res480p},
		{name: "A+B+C-1 stays 480p", roll: 59, want: Res480p},
		{name: "A+B+C keeps 4k", roll: 60, want: Res4k},
		{name: "99 keeps 4k", roll: 99, want: Res4k},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DrawUpstreamResolution(Res4k, cfg, tt.roll))
		})
	}
}

func TestDrawUpstreamResolution4kAllZero(t *testing.T) {
	cfg := LotteryConfig{To720From1080: 100, To480From1080: 100, To480From720: 100}
	assert.Equal(t, Res4k, DrawUpstreamResolution(Res4k, cfg, 0))
	assert.Equal(t, Res4k, DrawUpstreamResolution(Res4k, cfg, 99))
}

func TestDrawUpstreamResolution4kFullDowngrade(t *testing.T) {
	cfg := LotteryConfig{To1080From4k: 40, To720From4k: 30, To480From4k: 30}
	assert.Equal(t, Res1080p, DrawUpstreamResolution(Res4k, cfg, 0))
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res4k, cfg, 40))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res4k, cfg, 70))
	assert.Equal(t, Res480p, DrawUpstreamResolution(Res4k, cfg, 99))
}

func TestDrawUpstreamResolutionClampsNegativePercents(t *testing.T) {
	cfg := LotteryConfig{To1080From4k: -5, To720From4k: 20, To480From4k: -1}
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res4k, cfg, 0))
	assert.Equal(t, Res720p, DrawUpstreamResolution(Res4k, cfg, 19))
	assert.Equal(t, Res4k, DrawUpstreamResolution(Res4k, cfg, 20))
}

func TestNeedsLotteryRoll(t *testing.T) {
	assert.True(t, needsLotteryRoll(Res1080p))
	assert.True(t, needsLotteryRoll(Res720p))
	assert.True(t, needsLotteryRoll(Res4k))
	assert.True(t, needsLotteryRoll(" 4K "))
	assert.False(t, needsLotteryRoll(Res480p))
	assert.False(t, needsLotteryRoll("2k"))
	assert.False(t, needsLotteryRoll(""))
}

func TestDrawUpstreamResolutionRandInjected(t *testing.T) {
	cfg := LotteryConfig{To720From1080: 30, To480From1080: 20}
	got := drawUpstreamResolution(Res1080p, cfg, func(n int) int {
		require.Equal(t, 100, n)
		return 30
	})
	assert.Equal(t, Res480p, got)
}

func TestDrawUpstreamResolutionRandInjected4k(t *testing.T) {
	cfg := LotteryConfig{To1080From4k: 30, To720From4k: 20, To480From4k: 10}
	got := drawUpstreamResolution(Res4k, cfg, func(n int) int {
		require.Equal(t, 100, n)
		return 50
	})
	assert.Equal(t, Res480p, got)
}

func TestIsDowngrade(t *testing.T) {
	assert.True(t, IsDowngrade(Res1080p, Res720p))
	assert.True(t, IsDowngrade(Res1080p, Res480p))
	assert.True(t, IsDowngrade(Res720p, Res480p))
	assert.True(t, IsDowngrade(Res4k, Res1080p))
	assert.True(t, IsDowngrade(Res4k, Res720p))
	assert.True(t, IsDowngrade(Res4k, Res480p))
	assert.False(t, IsDowngrade(Res1080p, Res1080p))
	assert.False(t, IsDowngrade(Res720p, Res720p))
	assert.False(t, IsDowngrade(Res4k, Res4k))
	assert.False(t, IsDowngrade(Res480p, Res480p))
}
