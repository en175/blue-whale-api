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
	w25, h25, err := CanvasPixels("doubao-seedance-2.5-preview", Res480p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 854, w25)
	assert.Equal(t, 480, h25)

	w20, h20, err := CanvasPixels("doubao-seedance-2-0-260128", Res480p, "16:9")
	require.NoError(t, err)
	assert.Equal(t, 864, w20)
	assert.Equal(t, 496, h20)
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
