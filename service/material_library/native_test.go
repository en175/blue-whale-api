package material_library

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newNativeServer(t *testing.T, upstream http.HandlerFunc) (*httptest.Server, *gin.Engine) {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	setConfig(t, `{"enabled":true,"ak":"pak","sk":"psk","provider":"tokenhub","base_url":"`+server.URL+`"}`)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/ark", HandleNativeRequest)
	return server, router
}

func doNativeRequest(t *testing.T, router *gin.Engine, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func TestHandleNativeRequest_requiresAction(t *testing.T) {
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be called for invalid requests")
	})

	w := doNativeRequest(t, router, "/api/ark?Version=2024-01-01", `{}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var envelope struct {
		ResponseMetadata struct {
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, "InvalidParameter", envelope.ResponseMetadata.Error.Code)
	assert.NotEmpty(t, envelope.ResponseMetadata.Error.Message)
}

// TestHandleNativeRequest_passesVersionThrough 验证 Version 完全透传：
// 下游传什么上游就收到什么，上游的拒绝原样返回。
func TestHandleNativeRequest_passesVersionThrough(t *testing.T) {
	var gotVersion string
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotVersion = r.URL.Query().Get("Version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"InvalidParameter","Message":"unknown version"}}}`))
	})

	w := doNativeRequest(t, router, "/api/ark?Action=SomeFutureAction&Version=2023-01-01", `{}`)

	assert.Equal(t, "2023-01-01", gotVersion, "下游 Version 原样透传到上游")
	assert.JSONEq(t, `{"ResponseMetadata":{"Error":{"Code":"InvalidParameter","Message":"unknown version"}}}`, w.Body.String())
}

func TestHandleNativeRequest_passesUnknownActionThrough(t *testing.T) {
	var gotAction, gotPath string
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAction = r.URL.Query().Get("Action")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"up-1"},"Result":{"Future":"value"}}`))
	})

	w := doNativeRequest(t, router, "/api/ark?Action=SomeFutureAction&Version=2024-01-01", `{"A":1}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "SomeFutureAction", gotAction)
	assert.Equal(t, "/ark", gotPath)
	// 火山信封原样保留，不做 tokenhub 归一化
	assert.JSONEq(t, `{"ResponseMetadata":{"RequestId":"up-1"},"Result":{"Future":"value"}}`, w.Body.String())
}

func TestHandleNativeRequest_keepsUpstreamErrorEnvelope(t *testing.T) {
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"NotFound","Message":"no such group"}}}`))
	})

	w := doNativeRequest(t, router, "/api/ark?Action=GetAsset&Version=2024-01-01", `{"Id":"asset-1"}`)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{"Error":{"Code":"NotFound","Message":"no such group"}}}`, w.Body.String())
}

// TestHandleNativeRequest_assetMutationFailsClosed 验证单素材 Update/Delete 的
// 归属校验：GetAsset 不返回 GroupId 时按无权处理，且绝不把写请求发给上游。
func TestHandleNativeRequest_assetMutationFailsClosed(t *testing.T) {
	for _, action := range []string{"UpdateAsset", "DeleteAsset"} {
		t.Run(action, func(t *testing.T) {
			var writeCalled bool
			_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("Action") == "GetAsset" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"asset-1"}}`))
					return
				}
				writeCalled = true
				w.WriteHeader(http.StatusOK)
			})

			w := doNativeRequest(t, router, "/api/ark?Action="+action+"&Version=2024-01-01",
				`{"Id":"asset-1","Name":"renamed"}`)

			require.Equal(t, http.StatusNotFound, w.Code)
			assert.False(t, writeCalled, "写操作必须在归属校验通过后才透传")
		})
	}
}

func TestHandleNativeRequest_listAssetsRequiresGroupIds(t *testing.T) {
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("空 GroupIds 不得发往上游")
	})

	w := doNativeRequest(t, router, "/api/ark?Action=ListAssets&Version=2024-01-01", `{"Filter":{}}`)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Filter.GroupIds")
}

// TestHandleNativeRequest_restoresClientCallback 验证真人会话的回调改写闭环：
// 上游收到网关回调，客户端看到的仍是自己传入的地址。
func TestHandleNativeRequest_restoresClientCallback(t *testing.T) {
	var gotCallback string
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			CallbackURL string `json:"CallbackURL"`
		}
		_ = common.Unmarshal(body, &req)
		gotCallback = req.CallbackURL
		w.Header().Set("Content-Type", "application/json")
		// 华鲲会话类接口返回裸字段，无信封
		_, _ = w.Write([]byte(`{"BytedToken":"t1","CallbackURL":"` + req.CallbackURL + `"}`))
	})

	w := doNativeRequest(t, router, "/api/ark?Action=CreateVisualValidateSession&Version=2024-01-01",
		`{"CallbackURL":"https://client.example.com/done"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, gotCallback, "/v1/material/h5-callback", "上游必须收到网关回调")
	assert.NotContains(t, w.Body.String(), "/v1/material/h5-callback", "客户端必须看到自己的回调")
	assert.Contains(t, w.Body.String(), "https://client.example.com/done")
}

// TestHandleNativeRequest_movesResponseErrorIntoMetadata 验证华鲲的顶层
// ResponseError 被搬迁成火山的 ResponseMetadata.Error，且 Result 原样保留。
func TestHandleNativeRequest_movesResponseErrorIntoMetadata(t *testing.T) {
	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"up-1","Action":"GetAsset"},` +
			`"ResponseError":{"Code":"NotFound","Message":"resource not found"}}`))
	})

	w := doNativeRequest(t, router, "/api/ark?Action=GetAsset&Version=2024-01-01", `{"Id":"asset-1"}`)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{"RequestId":"up-1","Action":"GetAsset",`+
		`"Error":{"Code":"NotFound","Message":"resource not found"}}}`, w.Body.String())
}

// TestHandleNativeRequest_wrapsBareSessionResponse 验证会话类裸字段被补成火山信封。
func TestHandleNativeRequest_wrapsBareSessionResponse(t *testing.T) {
	for _, action := range []string{"CreateVisualValidateSession", "GetVisualValidateResult"} {
		t.Run(action, func(t *testing.T) {
			_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"BytedToken":"t1","GroupId":"group-1"}`))
			})

			w := doNativeRequest(t, router, "/api/ark?Action="+action+"&Version=2024-01-01", `{}`)

			require.Equal(t, http.StatusOK, w.Code)
			var envelope struct {
				ResponseMetadata struct {
					Action  string `json:"Action"`
					Version string `json:"Version"`
					Service string `json:"Service"`
				} `json:"ResponseMetadata"`
				Result map[string]any `json:"Result"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &envelope))
			assert.Equal(t, action, envelope.ResponseMetadata.Action)
			assert.Equal(t, "2024-01-01", envelope.ResponseMetadata.Version)
			assert.Equal(t, "ark", envelope.ResponseMetadata.Service)
			assert.Equal(t, "t1", envelope.Result["BytedToken"])
			assert.Equal(t, "group-1", envelope.Result["GroupId"])
		})
	}
}
