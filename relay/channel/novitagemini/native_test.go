package novitagemini

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newNativeRequest(t *testing.T, raw string) *dto.GeminiChatRequest {
	t.Helper()
	var request dto.GeminiChatRequest
	require.NoError(t, request.UnmarshalJSON([]byte(raw)))
	return &request
}

func TestConvertGeminiRequestPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeGemini}

	request := newNativeRequest(t, `{
		"contents":[{"role":"user","parts":[{"text":"a cute cat"}]}],
		"generationConfig":{"responseModalities":["IMAGE"],"imageConfig":{"aspectRatio":"16:9","imageSize":"2K"}}
	}`)

	converted, err := adaptor.ConvertGeminiRequest(c, info, request)
	require.NoError(t, err)
	assert.Same(t, request, converted, "native path must forward the parsed request unchanged")

	forwarded, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, forwarded.Contents, 1)
	assert.Equal(t, "a cute cat", forwarded.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"IMAGE"}, forwarded.GenerationConfig.ResponseModalities)
	assert.JSONEq(t, `{"aspectRatio":"16:9","imageSize":"2K"}`, string(forwarded.GenerationConfig.ImageConfig))
}

func TestConvertGeminiRequestRejections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeGemini}

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "batch requests are rejected",
			raw:  `{"requests":[{"contents":[{"role":"user","parts":[{"text":"a"}]}]}]}`,
			want: "does not support batch requests",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adaptor.ConvertGeminiRequest(c, info, newNativeRequest(t, tc.raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Equal(t, http.StatusBadRequest, types.NewError(err, types.ErrorCodeConvertRequestFailed).StatusCode)
		})
	}

	t.Run("nil request is rejected", func(t *testing.T) {
		_, err := adaptor.ConvertGeminiRequest(c, info, nil)
		require.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, types.NewError(err, types.ErrorCodeConvertRequestFailed).StatusCode)
	})

	// 非透传链路不经过 DoRequest 的兜底校验，转换阶段必须自己拒绝超限内联图。
	t.Run("oversized inline image is rejected in conversion", func(t *testing.T) {
		request := newNativeRequest(t, `{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"`+
			strings.Repeat("A", base64.StdEncoding.EncodedLen(maxInputImageBytes)+4)+`"}}]}]}`)
		_, err := adaptor.ConvertGeminiRequest(c, info, request)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is too large")
		assert.Equal(t, http.StatusBadRequest, types.NewError(err, types.ErrorCodeConvertRequestFailed).StatusCode)
	})
}

func TestValidateInlineImages(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfake"))

	build := func(t *testing.T, parts ...dto.GeminiPart) *dto.GeminiChatRequest {
		t.Helper()
		return &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Role: "user", Parts: parts}}}
	}
	inline := func(data string) dto.GeminiPart {
		return dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: data}}
	}

	t.Run("text-only request passes", func(t *testing.T) {
		require.NoError(t, validateInlineImages(build(t, dto.GeminiPart{Text: "a cat"})))
	})

	t.Run("single inline image passes", func(t *testing.T) {
		require.NoError(t, validateInlineImages(build(t, dto.GeminiPart{Text: "edit"}, inline(payload))))
	})

	t.Run("empty inline data is skipped, not rejected", func(t *testing.T) {
		require.NoError(t, validateInlineImages(build(t, inline(""))))
	})

	t.Run("too many inline images is rejected", func(t *testing.T) {
		parts := make([]dto.GeminiPart, 0, maxInputImages+1)
		for i := 0; i <= maxInputImages; i++ {
			parts = append(parts, inline(payload))
		}
		err := validateInlineImages(build(t, parts...))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many inline images")
	})

	t.Run("invalid base64 is rejected", func(t *testing.T) {
		err := validateInlineImages(build(t, inline("!!!")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not valid base64")
	})

	// 解压后的 body 已经被 MAX_REQUEST_BODY_MB 限制，但单张图仍可能吃掉大量内存；
	// 这张用编码后长度先判，避免为超限图片分配解码缓冲。
	t.Run("oversized single image is rejected without decoding", func(t *testing.T) {
		huge := strings.Repeat("A", base64.StdEncoding.EncodedLen(maxInputImageBytes)+4)
		err := validateInlineImages(build(t, inline(huge)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is too large")
	})

	t.Run("cumulative size over the total limit is rejected", func(t *testing.T) {
		// 每张略小于单张上限，三张之和超过 40MB 总量上限。
		perImage := base64.StdEncoding.EncodedLen(maxTotalInputImageBytes/3 + 1)
		blob := strings.Repeat("A", perImage)
		err := validateInlineImages(build(t, inline(blob), inline(blob), inline(blob)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too large in total")
	})
}

func TestDoRequestPassthroughScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	imageInfo := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, err := adaptor.DoRequest(c, imageInfo, strings.NewReader(`{"prompt":"a cat"}`))
	require.Error(t, err, "image path must keep rejecting passthrough")
	assert.Contains(t, err.Error(), "does not support request body passthrough")

	// 原生链路的原始体本来就是 Gemini 协议，透传是期望行为：DoRequest 不应拦下，
	// 原始体必须原样到达上游。
	var receivedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`))
	}))
	defer upstream.Close()
	service.InitHttpClient()

	nativeInfo := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeGemini,
		Request: newNativeRequest(t, `{
			"contents":[{"role":"user","parts":[{"text":"a cute cat"}]}],
			"generationConfig":{"responseModalities":["IMAGE"]}
		}`),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
			ChannelBaseUrl:    upstream.URL,
			UpstreamModelName: "nano-banana-2-t2i",
			ApiKey:            "test-key",
		},
	}
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nano-banana-2-t2i:generateContent", nil)

	rawBody := `{"contents":[{"role":"user","parts":[{"text":"a cute cat"}]}],"generationConfig":{"responseModalities":["IMAGE"]}}`
	result, err := adaptor.DoRequest(c2, nativeInfo, strings.NewReader(rawBody))
	require.NoError(t, err)
	resp, ok := result.(*http.Response)
	require.True(t, ok, "DoRequest should return the upstream *http.Response")
	defer resp.Body.Close()
	assert.Equal(t, rawBody, receivedBody, "native passthrough must not be blocked by the image-path guard")
}

// 透传模式会跳过 ConvertGeminiRequest，批量与内联图上限必须在 DoRequest 补上，
// 否则这两条约束在整个渠道上静默失效。
func TestDoRequestPassthroughStillValidates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	service.InitHttpClient()

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	passthroughInfo := func(request *dto.GeminiChatRequest) *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeGemini,
			Request:   request,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
				ChannelBaseUrl:    upstream.URL,
				UpstreamModelName: "nano-banana-2-t2i",
				ApiKey:            "test-key",
			},
		}
	}

	t.Run("batch requests are rejected under passthrough", func(t *testing.T) {
		upstreamHit = false
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

		request := newNativeRequest(t, `{"requests":[{"contents":[{"role":"user","parts":[{"text":"a"}]}]}]}`)
		_, err := adaptor.DoRequest(c, passthroughInfo(request), strings.NewReader(`{}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not support batch requests")
		assert.False(t, upstreamHit, "rejected request must not reach upstream")
	})

	t.Run("oversized inline image is rejected under passthrough", func(t *testing.T) {
		upstreamHit = false
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

		huge := strings.Repeat("A", base64.StdEncoding.EncodedLen(maxInputImageBytes)+4)
		request := newNativeRequest(t, `{"contents":[{"role":"user","parts":[
			{"text":"edit"},{"inlineData":{"mimeType":"image/png","data":"`+huge+`"}}
		]}]}`)
		_, err := adaptor.DoRequest(c, passthroughInfo(request), strings.NewReader(`{}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is too large")
		assert.False(t, upstreamHit, "rejected request must not reach upstream")
	})
}

// 向量端点与 RelayModeGemini 共用路由与 RelayMode，但 GeminiEmbeddingHandler 会跳过
// ConvertGeminiRequest；不拦的话 GetRequestURL 会把它改写成 :generateContent 发给图片模型。
func TestDoRequestRejectsUnsupportedGeminiActions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}
	service.InitHttpClient()

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	// 这些动作都会被 Path2RelayMode 归到 RelayModeGemini，但 GetRequestURL 只会拼
	// :generateContent。不拦就会把 embed / 计数请求改写成一次真实的图片生成并计费。
	paths := []string{
		"/v1beta/models/gemini-embedding-001:embedContent",
		"/v1beta/models/gemini-embedding-001:batchEmbedContents",
		"/v1beta/models/nano-banana-2-t2i:countTokens",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			upstreamHit = false
			info := &relaycommon.RelayInfo{
				RelayMode:      relayconstant.RelayModeGemini,
				RequestURLPath: path,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelBaseUrl:    upstream.URL,
					UpstreamModelName: "gemini-embedding-001",
					ApiKey:            "test-key",
				},
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)

			_, err := adaptor.DoRequest(c, info, strings.NewReader(`{"content":{"parts":[{"text":"a"}]}}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "only supports :generateContent")
			// GeminiEmbeddingHandler 会用 NewOpenAIError 包一层；400 必须穿透到这层，
			// 否则下游拿到的是 500「网关故障」，会去重试。
			wrapped := types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
			assert.Equal(t, http.StatusBadRequest, wrapped.StatusCode)
			assert.False(t, upstreamHit, "rejected request must not reach upstream")
		})
	}

	// 普通 generateContent / 流式路径不受影响。
	for _, path := range []string{
		"/v1beta/models/nano-banana-2-t2i:generateContent",
		"/v1/models/nano-banana-2-t2i:generateContent",
		"/v1beta/models/nano-banana-2-t2i:streamGenerateContent",
		"/v1beta/models/nano-banana-2-t2i:streamGenerateContent?alt=sse",
	} {
		t.Run("allowed "+path, func(t *testing.T) {
			upstreamHit = false
			info := &relaycommon.RelayInfo{
				RelayMode:      relayconstant.RelayModeGemini,
				RequestURLPath: path,
				Request: newNativeRequest(t, `{
					"contents":[{"role":"user","parts":[{"text":"a cute cat"}]}],
					"generationConfig":{"responseModalities":["IMAGE"]}
				}`),
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelSetting:    dto.ChannelSettings{PassThroughBodyEnabled: true},
					ChannelBaseUrl:    upstream.URL,
					UpstreamModelName: "nano-banana-2-t2i",
					ApiKey:            "test-key",
				},
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, info.RequestURLPath, nil)
			_, err := adaptor.DoRequest(c, info, strings.NewReader(`{"contents":[]}`))
			require.NoError(t, err)
			assert.True(t, upstreamHit, "generateContent must still reach upstream")
		})
	}
}

func TestDoResponseNativeRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nano-banana-2.1:generateContent", nil)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeGemini,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-nano-banana-2.1"},
	}

	// 超过 maxUpstreamResponseBytes 的响应不能被整包读进内存，也不能写回下游。
	oversized := io.MultiReader(
		strings.NewReader(strings.Repeat("A", 1024)),
		io.LimitReader(neverEndingReader{}, maxUpstreamResponseBytes),
	)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(oversized),
	}

	_, apiErr := adaptor.DoResponse(c, resp, info)
	require.NotNil(t, apiErr)
	assert.Empty(t, recorder.Body.String())
}

type neverEndingReader struct{}

func (neverEndingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'A'
	}
	return len(p), nil
}

// 估算计费必须按张数封顶：异常上游塞进大量 inlineData part 时，每张兜底 token 会线性放大。
func TestDoResponseNativeClampsEstimatedImageBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	parts := make([]string, 0, maxResponseImages+4)
	for i := 0; i < maxResponseImages+4; i++ {
		parts = append(parts, `{"inlineData":{"mimeType":"image/png","data":"QUJD"}}`)
	}
	// 上游只回了 prompt 侧 metadata（completion 为 0），处理器按每张兜底 token 估算，
	// 张数必须封顶。
	upstreamBody := `{"candidates":[{"content":{"parts":[` + strings.Join(parts, ",") + `]}}],` +
		`"usageMetadata":{"promptTokenCount":11,"totalTokenCount":11}}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nano-banana-2.1:generateContent", nil)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeGemini,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-nano-banana-2.1"},
	}

	usage, apiErr := adaptor.DoResponse(c, newGeminiResponse(t, upstreamBody), info)
	require.Nil(t, apiErr)

	billingUsage, ok := usage.(*dto.Usage)
	require.True(t, ok)
	require.NotNil(t, billingUsage.BillingUsage)
	assert.True(t, billingUsage.BillingUsage.Estimated)
	assert.Equal(t, maxResponseImages*estimatedTokensPerImage, billingUsage.CompletionTokens)
}

// 流式分支同样走按张兜底计费，张数上限必须一并生效。上游只在单个事件里
// 回图片、不给 usageMetadata，处理器会按每张兜底 token 估算。
func TestDoResponseNativeStreamClampsEstimatedImageBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	parts := make([]string, 0, maxResponseImages+4)
	for i := 0; i < maxResponseImages+4; i++ {
		parts = append(parts, `{"inlineData":{"mimeType":"image/png","data":"QUJD"}}`)
	}
	event := `{"candidates":[{"content":{"parts":[` + strings.Join(parts, ",") + `]}}]}`
	upstreamBody := "data: " + event + "\n\n"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nano-banana-2.1:streamGenerateContent", nil)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeGemini,
		IsStream:    true,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-nano-banana-2.1"},
	}

	usage, apiErr := adaptor.DoResponse(c, newGeminiResponse(t, upstreamBody), info)
	require.Nil(t, apiErr)

	billingUsage, ok := usage.(*dto.Usage)
	require.True(t, ok)
	require.NotNil(t, billingUsage.BillingUsage)
	assert.True(t, billingUsage.BillingUsage.Estimated)
	assert.Equal(t, maxResponseImages*estimatedTokensPerImage, billingUsage.CompletionTokens)
}

func newGeminiResponse(t *testing.T, body string) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// 原生链路的响应必须逐字节回给下游：旁路解析只用于计费，不能把响应重新序列化。
// 用一个未建模字段（modelVersion）+ 非常规缩进来锁定这一点，重新序列化必然改变字节。
func TestDoResponseNativeReturnsRawUpstreamBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	upstreamBody := "{\n  \"modelVersion\": \"gemini-nano-banana-2.1-001\",\n  " +
		"\"candidates\": [{\"content\": {\"role\": \"model\", \"parts\": [{\"text\": \"here you go\"}]}}],\n  " +
		"\"usageMetadata\": {\"promptTokenCount\": 6, \"candidatesTokenCount\": 1913, \"totalTokenCount\": 1919}\n}"

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nano-banana-2.1:generateContent", nil)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeGemini,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-nano-banana-2.1"},
	}

	usage, apiErr := adaptor.DoResponse(c, newGeminiResponse(t, upstreamBody), info)
	require.Nil(t, apiErr)
	assert.Equal(t, upstreamBody, recorder.Body.String(), "native response must not be re-serialized")

	billingUsage, ok := usage.(*dto.Usage)
	require.True(t, ok, "DoResponse must hand billing a *dto.Usage")
	assert.Equal(t, 6, billingUsage.PromptTokens)
	assert.Equal(t, 1913, billingUsage.CompletionTokens)
	assert.Equal(t, 1919, billingUsage.TotalTokens)
	require.NotNil(t, billingUsage.BillingUsage)
}

// usageMetadata 缺失（或 completion 为 0）但有内联图片时，必须按每张兜底 token 计费，
// 否则图片请求会以 0 completion 结算。
func TestDoResponseNativeFallsBackToPerImageBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	upstreamBody := `{"candidates":[{"content":{"parts":[
		{"inlineData":{"mimeType":"image/png","data":"QUJD"}},
		{"inlineData":{"mimeType":"image/png","data":"REVG"}}
	]}}],"usageMetadata":{"promptTokenCount":11,"totalTokenCount":11}}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/nano-banana-2.1:generateContent", nil)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeGemini,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-nano-banana-2.1"},
	}

	usage, apiErr := adaptor.DoResponse(c, newGeminiResponse(t, upstreamBody), info)
	require.Nil(t, apiErr)
	assert.Equal(t, upstreamBody, recorder.Body.String())

	billingUsage, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 11, billingUsage.PromptTokens)
	assert.Equal(t, 2*estimatedTokensPerImage, billingUsage.CompletionTokens, "two inline images bill the fallback rate each")
	assert.Equal(t, 11+2*estimatedTokensPerImage, billingUsage.TotalTokens)
	require.NotNil(t, billingUsage.BillingUsage)
}

// 图片翻译链路的响应形状不能被原生链路改动：仍必须是 OpenAI 的 images 结构。
func TestDoResponseDispatchesImageModeToOpenAIShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	upstreamBody := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"QUJD"}}]}}]}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	usage, apiErr := adaptor.DoResponse(c, newGeminiResponse(t, upstreamBody), info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "QUJD", imageResponse.Data[0].B64Json)
	assert.NotEqual(t, upstreamBody, recorder.Body.String(), "image mode must keep the OpenAI response shape")
}

func TestDoResponseRejectsUnsupportedRelayMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &Adaptor{}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}

	_, apiErr := adaptor.DoResponse(c, newGeminiResponse(t, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"QUJD"}}]}}]}`), info)
	require.NotNil(t, apiErr)
	// 未支持的模式不能悄悄落到图片处理器：下游不应收到任何响应体。
	assert.Empty(t, recorder.Body.String())
}
