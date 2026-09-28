package volcengine_plus

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeedanceSeriesOf(t *testing.T) {
	cases := []struct {
		model string
		want  SeedanceSeries
	}{
		{"doubao-seedance-2-5-260628", SeriesSeedance25},
		{"doubao-seedance-2.5-preview", SeriesSeedance25},
		{"doubao-seedance-2-0-260128", SeriesSeedance20},
		{"doubao-seedance-2-0-fast-260128", SeriesSeedance20},
		{"doubao-seedance-2-0-mini-260615", SeriesSeedance20},
		{"doubao-seedance-1-0-pro-250528", SeriesUnsupported},
		{"doubao-seedance-1-0-lite-t2v", SeriesUnsupported},
		{"doubao-seedance-1-5-pro-251215", SeriesUnsupported},
		{"  DOUBAO-Seedance-2-5-260628  ", SeriesSeedance25},
		{"some-future-model", SeriesSeedance20},
		{"", SeriesSeedance20},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, SeedanceSeriesOf(tc.model), "model=%q", tc.model)
	}
}

func TestResolutionSupported(t *testing.T) {
	const (
		m25   = "doubao-seedance-2-5-260628"
		m20   = "doubao-seedance-2-0-260128"
		mFast = "doubao-seedance-2-0-fast-260128"
		mMini = "doubao-seedance-2-0-mini-260615"
		m10   = "doubao-seedance-1-0-pro-250528"
		m15   = "doubao-seedance-1-5-pro-251215"
	)
	cases := []struct {
		model      string
		resolution string
		want       bool
	}{
		{m20, Res4k, true},
		{m25, Res4k, false},
		{mFast, Res4k, false},
		{mMini, Res4k, false},
		{m10, Res4k, false},
		{m15, Res4k, false},

		{m20, Res1080p, true},
		{m25, Res1080p, true},
		{mFast, Res1080p, false},
		{mMini, Res1080p, false},
		{m10, Res1080p, false},

		{m20, Res720p, true},
		{m25, Res720p, true},
		{mFast, Res720p, true},
		{mMini, Res720p, true},
		{m10, Res720p, false},

		{m20, Res480p, true},
		{m25, Res480p, true},
		{mFast, Res480p, true},
		{mMini, Res480p, true},
		{m10, Res480p, false},

		{m20, "", true},
		{mFast, "", true},
		{m10, "", false},

		// Unknown tiers stay permissive: the upstream reports them.
		{m20, "2k", true},
		{mFast, "2k", true},
		{m10, "2k", false},
		{m20, "  4K  ", true},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, ResolutionSupported(tc.model, tc.resolution),
			"model=%q resolution=%q", tc.model, tc.resolution)
	}
}
