package volcengine_plus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanvasPixels720p169(t *testing.T) {
	w, h, err := CanvasPixels("doubao-seedance-2-0-260128", Res720p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 1280, w)
	assert.Equal(t, 720, h)
}

func TestCanvasPixels480pDiffersBySeries(t *testing.T) {
	// Real Seedance 2.5 model IDs spell the version with a hyphen ("2-5"),
	// so this guards the series matcher against regressing to dot-only.
	const realModel25 = "doubao-seedance-2-5-260628"

	w25, h25, err := CanvasPixels(realModel25, Res480p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 854, w25)
	assert.Equal(t, 480, h25)

	w20, h20, err := CanvasPixels("doubao-seedance-2-0-260128", Res480p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 864, w20)
	assert.Equal(t, 496, h20)
}

func TestCanvasPixelsSharedAcrossSeriesAt720pAnd1080p(t *testing.T) {
	models := []string{
		"doubao-seedance-2-5-260628",
		"doubao-seedance-2-0-260128",
		"doubao-seedance-2-0-fast-260128",
		"doubao-seedance-2-0-mini-260615",
	}
	ratios := []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9"}
	resolutions := []string{Res720p, Res1080p, Res4k}

	for _, resolution := range resolutions {
		for _, ratio := range ratios {
			wantW, wantH, err := CanvasPixels(models[0], resolution, ratio)
			require.NoError(t, err)
			for _, model := range models[1:] {
				gotW, gotH, err := CanvasPixels(model, resolution, ratio)
				require.NoError(t, err)
				assert.Equal(t, wantW, gotW, "%s %s %s", model, resolution, ratio)
				assert.Equal(t, wantH, gotH, "%s %s %s", model, resolution, ratio)
			}
		}
	}
}

func TestCanvasPixelsUnknownRatio(t *testing.T) {
	_, _, err := CanvasPixels("doubao-seedance-2-0-260128", Res1080p, "2:1")
	require.Error(t, err)
}

func TestCanvasPixelsDefaultRatio(t *testing.T) {
	w, h, err := CanvasPixels("doubao-seedance-2-0-260128", Res1080p, "")
	require.NoError(t, err)
	assert.Equal(t, 1920, w)
	assert.Equal(t, 1080, h)
}

func TestKnownRatio(t *testing.T) {
	assert.True(t, KnownRatio(""))
	assert.True(t, KnownRatio("16:9"))
	assert.False(t, KnownRatio("2:1"))
}
