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

	got25, err := PeelTokensForTiers(108000, "seedance-2.5", Res720p, Res480p, "16:9")
	require.NoError(t, err)
	got20, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res720p, Res480p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, p25, got25)
	assert.Equal(t, p20, got20)
}

func TestPeelTokens43Vs169(t *testing.T) {
	p169, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res1080p, Res720p, "16:9")
	require.NoError(t, err)
	p43, err := PeelTokensForTiers(108000, "doubao-seedance-2-0-260128", Res1080p, Res720p, "4:3")
	require.NoError(t, err)
	assert.NotEqual(t, p169, p43)
}
