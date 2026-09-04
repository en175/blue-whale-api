package volcengine_plus

import (
	"fmt"
	"strings"
)

type canvasSize struct {
	W int
	H int
}

var knownRatios = map[string]struct{}{
	"16:9": {},
	"4:3":  {},
	"1:1":  {},
	"3:4":  {},
	"9:16": {},
	"21:9": {},
}

var canvas480p25 = map[string]canvasSize{
	"16:9": {854, 480},
	"4:3":  {752, 560},
	"1:1":  {640, 640},
	"3:4":  {560, 752},
	"9:16": {480, 854},
	"21:9": {992, 432},
}

var canvas480p20 = map[string]canvasSize{
	"16:9": {864, 496},
	"4:3":  {752, 560},
	"1:1":  {640, 640},
	"3:4":  {560, 752},
	"9:16": {496, 864},
	"21:9": {992, 432},
}

var canvas720p = map[string]canvasSize{
	"16:9": {1280, 720},
	"4:3":  {1112, 834},
	"1:1":  {960, 960},
	"3:4":  {834, 1112},
	"9:16": {720, 1280},
	"21:9": {1470, 630},
}

var canvas1080p = map[string]canvasSize{
	"16:9": {1920, 1080},
	"4:3":  {1664, 1248},
	"1:1":  {1440, 1440},
	"3:4":  {1248, 1664},
	"9:16": {1080, 1920},
	"21:9": {2206, 946},
}

var canvas4k = map[string]canvasSize{
	"16:9": {3840, 2160},
	"4:3":  {3326, 2494},
	"1:1":  {2880, 2880},
	"3:4":  {2494, 3326},
	"9:16": {2160, 3840},
	"21:9": {4398, 1886},
}

func NormalizeRatio(ratio string) string {
	ratio = strings.TrimSpace(ratio)
	if ratio == "" {
		return DefaultRatio
	}
	return ratio
}

func KnownRatio(ratio string) bool {
	_, ok := knownRatios[NormalizeRatio(ratio)]
	return ok
}

func isSeedance25(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "2.5")
}

// CanvasPixels returns the billing canvas for a model series, resolution tier, and ratio.
func CanvasPixels(modelName, resolution, ratio string) (int, int, error) {
	ratio = NormalizeRatio(ratio)
	if !KnownRatio(ratio) {
		return 0, 0, fmt.Errorf("unsupported ratio: %s", ratio)
	}
	res := NormalizeResolution(resolution)
	var table map[string]canvasSize
	switch res {
	case Res480p:
		if isSeedance25(modelName) {
			table = canvas480p25
		} else {
			table = canvas480p20
		}
	case Res720p:
		table = canvas720p
	case Res1080p:
		table = canvas1080p
	case Res4k:
		table = canvas4k
	default:
		return 0, 0, fmt.Errorf("unsupported resolution: %s", resolution)
	}
	size, ok := table[ratio]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported ratio: %s", ratio)
	}
	return size.W, size.H, nil
}
