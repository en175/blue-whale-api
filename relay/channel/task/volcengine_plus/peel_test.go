package volcengine_plus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeelTokens169720to1080(t *testing.T) {
	got := PeelTokens(108000, 1920, 1080, 1280, 720)
	assert.Equal(t, 243000, got)
}

func TestPeelTokensForTiers169(t *testing.T) {
	got, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res1080p, Res720p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 243000, got)
}

func TestPeelTokensNoDowngrade(t *testing.T) {
	got, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res720p, Res720p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 108000, got)
}

func TestPeelTokens480pSeriesDiffer(t *testing.T) {
	p25 := PeelTokens(108000, 1280, 720, 854, 480)
	p20 := PeelTokens(108000, 1280, 720, 864, 496)
	assert.NotEqual(t, p25, p20)

	got25, err := PeelTokensForTiers(108000, "doubao-seedance-2-5-260628", Res720p, Res480p, "16:9")
	require.NoError(t, err)
	got20, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res720p, Res480p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, p25, got25)
	assert.Equal(t, p20, got20)
}

func TestPeelTokensForTiers4k(t *testing.T) {
	const model = "doubao-seedance-2-0-260128"
	tests := []struct {
		name     string
		upstream string
		want     int
	}{
		{name: "from 1080p", upstream: Res1080p, want: 432000},
		{name: "from 720p", upstream: Res720p, want: 972000},
		{name: "from 480p", upstream: Res480p, want: 2090323},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PeelTokensForTiers(108000, model, Res4k, tt.upstream, "16:9")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPeelTokensForTiers4kRatiosShareCanvases(t *testing.T) {
	// 4k 的 21:9 画布与 1080p/720p 的 21:9 画布同源，剥皮倍率必须随 ratio 变化。
	p169, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res4k, Res1080p, "16:9")
	require.NoError(t, err)
	p219, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res4k, Res1080p, "21:9")
	require.NoError(t, err)
	assert.NotEqual(t, p169, p219)
}

func TestPeelTokens43Vs169(t *testing.T) {
	p169, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res1080p, Res720p, "16:9")
	require.NoError(t, err)
	p43, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res1080p, Res720p, "4:3")
	require.NoError(t, err)
	assert.NotEqual(t, p169, p43)
}
