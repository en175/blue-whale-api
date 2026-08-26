package helper

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func ResolveIncomingBillingExprRequestInput(c *gin.Context, info *relaycommon.RelayInfo) (billingexpr.RequestInput, error) {
	if info != nil && info.BillingRequestInput != nil {
		input := cloneRequestInput(*info.BillingRequestInput)
		merged := cloneStringMap(info.RequestHeaders)
		for k, v := range input.Headers {
			merged[k] = v
		}
		input.Headers = merged
		return input, nil
	}

	input := billingexpr.RequestInput{}
	if info != nil {
		input.Headers = cloneStringMap(info.RequestHeaders)
	}

	bodyBytes, err := readIncomingBillingExprBody(c)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	return input, nil
}

func BuildBillingExprRequestInputFromRequest(request dto.Request, headers map[string]string) (billingexpr.RequestInput, error) {
	input := billingexpr.RequestInput{
		Headers: cloneStringMap(headers),
	}
	if request == nil {
		return input, nil
	}

	bodyBytes, err := common.Marshal(request)
	if err != nil {
		return billingexpr.RequestInput{}, err
	}
	input.Body = bodyBytes
	return input, nil
}

// NormalizeBillingRequestInput overlays validated billing parameters onto the
// request body tiered expressions read via param(). The raw body alone is
// unreliable: multipart requests (image edits) carry no JSON body at all, and
// JSON requests may omit fields the validator normalized (image n defaults to
// 1). Without the overlay expressions read nil and either fail or mis-tier the
// request. Pre-consume and settlement share the same RequestInput, so one
// normalization keeps both phases consistent.
func NormalizeBillingRequestInput(c *gin.Context, input billingexpr.RequestInput, request dto.Request, meta *types.TokenCountMeta) billingexpr.RequestInput {
	overlays := normalizedBillingOverlays(c, request, meta)
	if len(overlays) == 0 {
		return input
	}

	body := map[string]interface{}{}
	if len(input.Body) > 0 {
		// Keep the raw body as the base; a non-object body falls back to an
		// empty object so the overlays still apply.
		if err := common.Unmarshal(input.Body, &body); err != nil {
			body = map[string]interface{}{}
		}
	}
	for key, value := range overlays {
		body[key] = value
	}
	merged, err := common.Marshal(body)
	if err != nil {
		return input
	}
	input.Body = merged
	return input
}

func normalizedBillingOverlays(c *gin.Context, request dto.Request, meta *types.TokenCountMeta) map[string]interface{} {
	overlays := map[string]interface{}{}
	if meta != nil {
		for name, value := range meta.BillingRatios {
			overlays[name] = value
		}
	}
	imageRequest, ok := request.(*dto.ImageRequest)
	if !ok || imageRequest == nil {
		return overlays
	}
	// Validated values take precedence over the raw body: the validator
	// normalizes n (nil/0 -> 1, capped at dto.MaxImageN), and that normalized
	// count is what the relay actually bills and generates.
	if imageRequest.N != nil {
		overlays["n"] = *imageRequest.N
	}
	if imageRequest.Size != "" {
		overlays["size"] = imageRequest.Size
	}
	if imageRequest.Quality != "" {
		overlays["quality"] = imageRequest.Quality
	}
	if imageCount := countImageEditFileUploads(c); imageCount > 0 {
		// Cap the reference-image count that becomes the billing multiplier in
		// param("image.#") expressions. A multipart edit can carry an unbounded
		// number of files, so without a bound a client could inflate the billed
		// per-image charge. dto.MaxImageN is the project-wide image bound (the
		// validator rejects requests generating more than this many images).
		if imageCount > dto.MaxImageN {
			imageCount = dto.MaxImageN
		}
		// param("image.#") counts array elements; a placeholder array keeps
		// the same gjson path working for multipart reference-image uploads,
		// whose file count is otherwise invisible to expressions.
		overlays["image"] = make([]string, imageCount)
	}
	return overlays
}

// countImageEditFileUploads counts reference-image files on multipart image
// edit requests, mirroring the field names the channel adaptors accept
// (image, image[], image[...]). Returns 0 for non-multipart requests.
func countImageEditFileUploads(c *gin.Context) int {
	if c == nil || c.Request == nil || c.Request.MultipartForm == nil || c.Request.MultipartForm.File == nil {
		return 0
	}
	files := c.Request.MultipartForm.File
	if headers, ok := files["image"]; ok && len(headers) > 0 {
		return len(headers)
	}
	if headers, ok := files["image[]"]; ok && len(headers) > 0 {
		return len(headers)
	}
	count := 0
	for name, headers := range files {
		if strings.HasPrefix(name, "image[") {
			count += len(headers)
		}
	}
	return count
}

func readIncomingBillingExprBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil || !isJSONContentType(c.Request.Header.Get("Content-Type")) {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func cloneRequestInput(src billingexpr.RequestInput) billingexpr.RequestInput {
	input := billingexpr.RequestInput{
		Headers: cloneStringMap(src.Headers),
	}
	if len(src.Body) > 0 {
		input.Body = append([]byte(nil), src.Body...)
	}
	return input
}

func isJSONContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json")
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		if strings.TrimSpace(key) == "" {
			continue
		}
		dst[key] = value
	}
	return dst
}
