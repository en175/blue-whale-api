package volcengine_plus

import "strings"

// SeedanceSeries identifies the billing canvas family of a Seedance model.
type SeedanceSeries int

const (
	// SeriesSeedance20 covers Seedance 2.0 and its fast / mini variants.
	SeriesSeedance20 SeedanceSeries = iota
	SeriesSeedance25
	// SeriesUnsupported covers the 1.0 / 1.5 families, which this channel does not serve.
	SeriesUnsupported
)

// SeedanceSeriesOf classifies a model name. Unknown names default to Seedance 2.0.
func SeedanceSeriesOf(modelName string) SeedanceSeries {
	name := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(name, "1-0"), strings.Contains(name, "1.0"),
		strings.Contains(name, "1-5"), strings.Contains(name, "1.5"):
		return SeriesUnsupported
	case strings.Contains(name, "2.5"), strings.Contains(name, "2-5"):
		return SeriesSeedance25
	default:
		return SeriesSeedance20
	}
}

func isFastOrMini(modelName string) bool {
	name := strings.ToLower(modelName)
	return strings.Contains(name, "fast") || strings.Contains(name, "mini")
}

// ResolutionSupported reports whether a model can serve the requested resolution tier.
func ResolutionSupported(modelName, resolution string) bool {
	series := SeedanceSeriesOf(modelName)
	if series == SeriesUnsupported {
		return false
	}
	switch NormalizeResolution(resolution) {
	case Res1080p:
		return !isFastOrMini(modelName)
	case Res4k:
		return series == SeriesSeedance20 && !isFastOrMini(modelName)
	default:
		return true
	}
}
