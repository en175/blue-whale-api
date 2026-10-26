package material_library

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

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
	router.Use(func(c *gin.Context) { c.Set("id", 42) })
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

	w := doNativeRequest(t, router, "/api/ark?Action=SomeFutureAction&Version=2024-01-01", `{"Id":"asset-1"}`)

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

// TestHandleNativeRequest_listAssetGroupsLocalIsolation 验证兼容入口的组列表
// 与路径入口同源：只查本地归属表、按当前 API key 过滤，不触达上游。
func TestHandleNativeRequest_listAssetGroupsLocalIsolation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "g-user42-a", "A组", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "g-user42-b", "B组", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "g-user42-live", "真人组", "LivenessFace"))
	require.NoError(t, model.AddUserMaterialGroup(7, "tokenhub", "g-user7", "他人组", "AIGC"))

	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("ListAssetGroups 不得查上游")
	})

	w := doNativeRequest(t, router,
		"/api/ark?Action=ListAssetGroups&Version=2023-01-01",
		`{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":10}`)

	require.Equal(t, http.StatusOK, w.Code)
	var out struct {
		ResponseMetadata struct {
			RequestId string `json:"RequestId"`
			Action    string `json:"Action"`
			Version   string `json:"Version"`
		} `json:"ResponseMetadata"`
		Result struct {
			TotalCount int `json:"TotalCount"`
			Items      []struct {
				Id string `json:"Id"`
			} `json:"Items"`
		} `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "ListAssetGroups", out.ResponseMetadata.Action)
	assert.Equal(t, "2023-01-01", out.ResponseMetadata.Version)
	assert.NotEmpty(t, out.ResponseMetadata.RequestId)
	assert.Equal(t, 2, out.Result.TotalCount)
	require.Len(t, out.Result.Items, 2)
	for _, item := range out.Result.Items {
		assert.NotEqual(t, "g-user7", item.Id, "不得返回其他用户的组")
	}
}

// TestHandleNativeRequest_listAssetGroupsHugePage 验证超大 PageNumber 不会因
// (page-1)*pageSize 溢出而切片越界 panic，超出末页时返回空 Items。
func TestHandleNativeRequest_listAssetGroupsHugePage(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "g-user42", "组", "AIGC"))

	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("ListAssetGroups 不得查上游")
	})

	w := doNativeRequest(t, router,
		"/api/ark?Action=ListAssetGroups&Version=2024-01-01",
		`{"Filter":{"GroupType":"AIGC"},"PageNumber":922337203685477580,"PageSize":100}`)

	require.Equal(t, http.StatusOK, w.Code)
	var out struct {
		Result struct {
			TotalCount int   `json:"TotalCount"`
			Items      []any `json:"Items"`
		} `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, 1, out.Result.TotalCount)
	assert.Empty(t, out.Result.Items)
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
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"up-1","Action":"SomeFutureAction"},` +
			`"ResponseError":{"Code":"NotFound","Message":"resource not found"}}`))
	})

	w := doNativeRequest(t, router, "/api/ark?Action=SomeFutureAction&Version=2024-01-01", `{"Id":"asset-1"}`)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{"RequestId":"up-1","Action":"SomeFutureAction",`+
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

// TestHandleNativeRequest_rejectsMalformedIdentifiers 验证白名单动作对格式非法的
// 请求体 fail-closed：body 不是 JSON 对象、或标识字段不是字符串时，下游 helper
// 会把标识当成缺省值放行，必须在归属校验之前就拒绝。
func TestHandleNativeRequest_rejectsMalformedIdentifiers(t *testing.T) {
	tests := []struct {
		action string
		body   string
	}{
		{"GetAssetGroup", `{"Id":123}`},
		{"UpdateAssetGroup", `{"Id":123,"Description":"x"}`},
		{"DeleteAssetGroup", `not-json`},
		{"GetAsset", `{"Id":123}`},
		{"GetAsset", `{"id":123}`},
		{"UpdateAsset", `{"Id":123}`},
		{"DeleteAsset", `[]`},
		{"CreateAsset", `{"GroupId":123,"URL":"https://x/y.jpg","Name":"a","AssetType":"Image"}`},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("格式非法的请求不得发往上游")
			})

			w := doNativeRequest(t, router, "/api/ark?Action="+tt.action+"&Version=2024-01-01", tt.body)

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), "InvalidParameter")
		})
	}
}

// TestHandleNativeRequest_visualResultRequiresSessionOwner 验证兼容入口的认证结果
// 查询与路径入口同口径：token 不存在或不属于当前 API key 时返回空 GroupId，
// 且不向上游发起查询。
func TestHandleNativeRequest_visualResultRequiresSessionOwner(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "tokenhub", "mine", "bt-mine", "h5", "cb", "proj", ""))
	require.NoError(t, model.AddVisualValidateSession(7, "tokenhub", "other", "bt-other", "h5", "cb", "proj", ""))

	for _, tc := range []struct {
		name          string
		body          string
		upstreamCalls int
		wantGroup     string
	}{
		{name: "owner", body: `{"BytedToken":"bt-mine"}`, upstreamCalls: 1, wantGroup: "group-mine"},
		{name: "other user", body: `{"BytedToken":"bt-other"}`, upstreamCalls: 0},
		{name: "unknown token", body: `{"BytedToken":"bt-unknown"}`, upstreamCalls: 0},
		// 上游按大小写不敏感解析键名，小写字段同样必须走归属校验。
		{name: "other user lowercase key", body: `{"bytedtoken":"bt-other"}`, upstreamCalls: 0},
		{name: "conflicting keys", body: `{"BytedToken":"bt-mine","bytedtoken":"bt-other"}`, upstreamCalls: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"GroupId":"group-mine"}`))
			})

			w := doNativeRequest(t, router,
				"/api/ark?Action=GetVisualValidateResult&Version=2024-01-01", tc.body)

			require.Equal(t, http.StatusOK, w.Code)
			var out struct {
				Result struct {
					GroupId string `json:"GroupId"`
				} `json:"Result"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
			assert.Equal(t, tc.wantGroup, out.Result.GroupId)
			assert.Equal(t, tc.upstreamCalls, calls)
		})
	}
}

// TestWithProjectName 验证查询体里的 ProjectName 以会话记录为准，并兼容大小写键名。
func TestWithProjectName(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "missing", body: `{"BytedToken":"t"}`, want: `{"BytedToken":"t","ProjectName":"p"}`},
		{name: "same", body: `{"BytedToken":"t","ProjectName":"p"}`, want: `{"BytedToken":"t","ProjectName":"p"}`},
		{name: "mismatch", body: `{"BytedToken":"t","ProjectName":"other"}`, want: `{"BytedToken":"t","ProjectName":"p"}`},
		{name: "lowercase key", body: `{"bytedToken":"t","projectName":"other"}`, want: `{"bytedToken":"t","ProjectName":"p"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.JSONEq(t, tc.want, string(withProjectName([]byte(tc.body), "p")))
		})
	}
}

// TestHandleNativeRequest_visualResultUsesSessionProvider 验证兼容入口按会话记录
// 的 provider 查询认证结果，并补上创建会话时的 ProjectName。
func TestHandleNativeRequest_visualResultUsesSessionProvider(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "volcengine", "mine", "bt-mine", "h5", "cb", "proj-x", ""))

	tokenhubCalls := 0
	tokenhub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenhubCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"GroupId":"group-wrong"}`))
	}))
	defer tokenhub.Close()

	var got struct {
		BytedToken  string `json:"BytedToken"`
		ProjectName string `json:"ProjectName"`
	}
	volcCalls := 0
	volc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		volcCalls++
		body, _ := io.ReadAll(r.Body)
		_ = common.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"req-volc"},"Result":{"GroupId":"group-mine"}}`))
	}))
	defer volc.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"provider":"tokenhub","ak":"pak","sk":"psk","base_url":%q,`+
		`"volcengine_access_key":"ak","volcengine_secret_key":"sk","volcengine_base_url":%q}`,
		tokenhub.URL, volc.URL))
	defer setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak","sk":"psk","base_url":""}`)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 42) })
	router.POST("/api/ark", HandleNativeRequest)

	w := doNativeRequest(t, router, "/api/ark?Action=GetVisualValidateResult&Version=2024-01-01", `{"BytedToken":"bt-mine"}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Zero(t, tokenhubCalls, "必须按会话记录的 provider 查询")
	assert.Equal(t, 1, volcCalls)
	assert.Equal(t, "bt-mine", got.BytedToken)
	assert.Equal(t, "proj-x", got.ProjectName, "客户端漏传时补会话记录的 ProjectName")
	assert.Contains(t, w.Body.String(), "group-mine")
}

// TestHandleNativeRequest_listAssetGroupsDBErrorUsesArkEnvelope 验证本地组列表
// 查询失败时仍返回火山信封，而不是纯文本 500。
func TestHandleNativeRequest_listAssetGroupsDBErrorUsesArkEnvelope(t *testing.T) {
	db := openMaterialRecordDB(t)
	model.DB = db
	require.NoError(t, db.Migrator().DropTable(&model.UserMaterialGroup{}))

	_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("ListAssetGroups 不得查上游")
	})

	w := doNativeRequest(t, router, "/api/ark?Action=ListAssetGroups&Version=2024-01-01", `{}`)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var out struct {
		ResponseMetadata struct {
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "InternalError", out.ResponseMetadata.Error.Code)
}

// TestHandleNativeRequest_getAssetRequiresOwnership 验证兼容入口的 GetAsset
// 与 Update/Delete 同口径：先反查 GroupId 校验归属，非本人素材 404 且不再读取。
func TestHandleNativeRequest_getAssetRequiresOwnership(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "group-owned", "我的组", "AIGC"))

	for _, tc := range []struct {
		name         string
		body         string
		groupID      string
		wantStatus   int
		upstreamCall int
	}{
		{name: "foreign asset", body: `{"Id":"asset-x"}`, groupID: "group-foreign", wantStatus: http.StatusNotFound, upstreamCall: 1},
		{name: "foreign asset lowercase key", body: `{"id":"asset-x"}`, groupID: "group-foreign", wantStatus: http.StatusNotFound, upstreamCall: 1},
		{name: "conflicting keys", body: `{"Id":"asset-x","id":"asset-y"}`, wantStatus: http.StatusBadRequest, upstreamCall: 0},
		{name: "owned asset", body: `{"Id":"asset-x"}`, groupID: "group-owned", wantStatus: http.StatusOK, upstreamCall: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, router := newNativeServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"asset-x","GroupId":"` + tc.groupID + `"}}`))
			})

			w := doNativeRequest(t, router, "/api/ark?Action=GetAsset&Version=2024-01-01", tc.body)

			require.Equal(t, tc.wantStatus, w.Code)
			assert.Equal(t, tc.upstreamCall, calls, "归属校验通过后直接复用探测响应，不再读取")
		})
	}
}
