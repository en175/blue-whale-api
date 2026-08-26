package helper

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveIncomingBillingExprRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	body := []byte(`{"service_tier":"fast"}`)
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	ctx.Set(common.KeyRequestBody, body)

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{"Content-Type": "application/json"},
	}

	input, err := ResolveIncomingBillingExprRequestInput(ctx, info)
	require.NoError(t, err)
	require.Equal(t, body, input.Body)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
}

func TestBuildBillingExprRequestInputFromRequest(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:  "gemini-3.1-pro-preview",
		Stream: lo.ToPtr(true),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: lo.ToPtr(uint(3000)),
	}

	input, err := BuildBillingExprRequestInputFromRequest(request, map[string]string{
		"Content-Type": "application/json",
		"X-Test":       "1",
	})
	require.NoError(t, err)
	require.Equal(t, "application/json", input.Headers["Content-Type"])
	require.Equal(t, "1", input.Headers["X-Test"])
	require.True(t, gjson.GetBytes(input.Body, "stream").Bool())
	require.Equal(t, "user", gjson.GetBytes(input.Body, "messages.0.role").String())
	require.Equal(t, float64(3000), gjson.GetBytes(input.Body, "max_tokens").Float())
}

// multipartEditContext builds a request context carrying a parsed multipart
// form with the given number of reference-image file parts under "image".
func multipartEditContext(t *testing.T, imageFiles int) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	form := &multipart.Form{File: map[string][]*multipart.FileHeader{}}
	for i := 0; i < imageFiles; i++ {
		form.File["image"] = append(form.File["image"], &multipart.FileHeader{Filename: "input.png"})
	}
	c.Request.MultipartForm = form
	return c
}

func TestNormalizeBillingRequestInput_ImageOverlays(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		setupCtx   func(t *testing.T) *gin.Context
		input      billingexpr.RequestInput
		request    dto.Request
		meta       *types.TokenCountMeta
		wantFields map[string]interface{}
		wantAbsent []string
	}{
		{
			// Multipart edits carry no JSON body: validated n/size/quality and the
			// uploaded reference-image count must be materialized for param().
			name:     "multipart empty body gains normalized fields",
			setupCtx: func(t *testing.T) *gin.Context { return multipartEditContext(t, 2) },
			input:    billingexpr.RequestInput{},
			request: &dto.ImageRequest{
				Model:   "doubao-seedream-5-0-pro-260628",
				Prompt:  "edit",
				N:       lo.ToPtr(uint(1)),
				Size:    "2048x2048",
				Quality: "standard",
			},
			meta: &types.TokenCountMeta{BillingRatios: map[string]float64{"n": 1}},
			wantFields: map[string]interface{}{
				"n":       float64(1),
				"size":    "2048x2048",
				"quality": "standard",
			},
		},
		{
			// JSON generations may omit n; the validator-normalized count wins
			// over the raw body so billing matches what is actually generated.
			name: "json body keeps fields and overrides n=0",
			setupCtx: func(t *testing.T) *gin.Context {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				return c
			},
			input: billingexpr.RequestInput{Body: []byte(`{"model":"m","prompt":"p","n":0,"size":"1024x1024"}`)},
			request: &dto.ImageRequest{
				Model:  "m",
				Prompt: "p",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
			},
			meta: &types.TokenCountMeta{BillingRatios: map[string]float64{"n": 1}},
			wantFields: map[string]interface{}{
				"n":      float64(1),
				"size":   "1024x1024",
				"prompt": "p",
			},
			wantAbsent: []string{"quality"},
		},
		{
			// Non-image requests without billing ratios are left byte-identical.
			name: "non-image request unchanged",
			setupCtx: func(t *testing.T) *gin.Context {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				return c
			},
			input:      billingexpr.RequestInput{Body: []byte(`{"model":"m"}`)},
			request:    &dto.GeneralOpenAIRequest{Model: "m"},
			meta:       &types.TokenCountMeta{},
			wantFields: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.setupCtx(t)
			got := NormalizeBillingRequestInput(c, tt.input, tt.request, tt.meta)

			if tt.wantFields == nil {
				assert.Equal(t, tt.input.Body, got.Body, "body must stay untouched")
				return
			}
			for key, want := range tt.wantFields {
				res := gjson.GetBytes(got.Body, key)
				require.True(t, res.Exists(), "field %s must exist in %s", key, got.Body)
				assert.Equal(t, want, res.Value(), "field %s", key)
			}
			for _, key := range tt.wantAbsent {
				assert.False(t, gjson.GetBytes(got.Body, key).Exists(), "field %s must be absent", key)
			}
		})
	}
}

func TestNormalizeBillingRequestInput_ImageArrayCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := multipartEditContext(t, 3)
	req := &dto.ImageRequest{Model: "m", Prompt: "p", N: lo.ToPtr(uint(1))}

	got := NormalizeBillingRequestInput(c, billingexpr.RequestInput{}, req, &types.TokenCountMeta{BillingRatios: map[string]float64{"n": 1}})

	// param("image.#") must report the uploaded reference-image count.
	assert.Equal(t, int64(3), gjson.GetBytes(got.Body, "image.#").Int())
}

func TestNormalizeBillingRequestInput_ImageCountCapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// A multipart edit carrying more reference images than the project-wide
	// bound must clamp the billed param("image.#") multiplier instead of
	// letting a client inflate the per-image charge.
	c := multipartEditContext(t, dto.MaxImageN+50)
	req := &dto.ImageRequest{Model: "m", Prompt: "p", N: lo.ToPtr(uint(1))}

	got := NormalizeBillingRequestInput(c, billingexpr.RequestInput{}, req, &types.TokenCountMeta{BillingRatios: map[string]float64{"n": 1}})

	assert.Equal(t, int64(dto.MaxImageN), gjson.GetBytes(got.Body, "image.#").Int())
}
