package material_library

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openMaterialRecordDB 为记录层测试准备隔离的归属表与会话表。
func openMaterialRecordDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserMaterialGroup{}, &model.UserMaterialVisualSession{}))
	return db
}

func newRecordContext(t *testing.T, path, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if body == "" {
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	} else {
		c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	}
	c.Set("id", 42)
	c.Request = c.Request.WithContext(RecordContext(c))
	return c
}

// TestRecordContext_extractsOperationInfo 验证 RecordContext 提取归属信息，且请求体原样装回。
func TestRecordContext_extractsOperationInfo(t *testing.T) {
	t.Run("CreateAssetGroup 从 body 读组名与组类型", func(t *testing.T) {
		c := newRecordContext(t, "/api/material/CreateAssetGroup", `{"Name":"group-a","GroupType":"LivenessFace"}`)
		ctx := c.Request.Context()
		assert.Equal(t, 42, ctx.Value(ctxUserID))
		assert.Equal(t, "LivenessFace", ctx.Value(ctxGroupType))
		assert.Equal(t, "group-a", ctx.Value(ctxGroupName))

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"Name":"group-a","GroupType":"LivenessFace"}`, string(body))
	})

	t.Run("DeleteAssetGroup 从 body 读组 id", func(t *testing.T) {
		c := newRecordContext(t, "/api/material/DeleteAssetGroup", `{"Id":"group-del"}`)
		ctx := c.Request.Context()
		assert.Equal(t, "group-del", ctx.Value(ctxGroupID))

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"Id":"group-del"}`, string(body))
	})

	t.Run("无关 Action 不透传记录信息", func(t *testing.T) {
		c := newRecordContext(t, "/api/material/ListAssetGroups", `{}`)
		ctx := c.Request.Context()
		assert.Nil(t, ctx.Value(ctxGroupType))
		assert.Nil(t, ctx.Value(ctxGroupID))
	})
}

// TestProxy_recordsGroupAssociation 端到端验证：CreateAssetGroup 成功后，归属表落库 Result.Id。
func TestProxy_recordsGroupAssociation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"group-recorded"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	c := newRecordContext(t, "/api/material/CreateAssetGroup", `{"Name":"group-a","GroupType":"AIGC"}`)
	// 直接传原始 recorder：gin 的 writer 会把 httptest.ResponseRecorder 包一层，
	// ReverseProxy 的 CloseNotify 断言会在测试里 panic（生产环境底层是真实连接，无此问题）。
	Proxy().ServeHTTP(httptest.NewRecorder(), c.Request)

	groups, err := model.GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "group-recorded", groups[0].SeedanceAssetGroupId)
	assert.Equal(t, "AIGC", groups[0].GroupType)
	assert.Equal(t, "group-a", groups[0].Name)
}

// TestProxy_removesGroupAssociationOnDelete 端到端验证：DeleteAssetGroup 成功后，归属表同步清理。
func TestProxy_removesGroupAssociationOnDelete(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "group-del", "g", "AIGC"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	c := newRecordContext(t, "/api/material/DeleteAssetGroup", `{"Id":"group-del"}`)
	Proxy().ServeHTTP(httptest.NewRecorder(), c.Request)

	groups, err := model.GetUserMaterialGroups(42)
	require.NoError(t, err)
	assert.Len(t, groups, 0)
}

// TestProxy_recordsVisualValidateSession 端到端验证：CreateVisualValidateSession 成功后，
// 会话表记录 BytedToken/H5Link/CallbackURL 及用户，供认证回调反查。
func TestProxy_recordsVisualValidateSession(t *testing.T) {
	model.DB = openMaterialRecordDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"BytedToken":"bt-1","H5Link":"https://h5.example/1"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"callback_url":"https://gw.example/cb"}`, upstream.URL))

	c := newRecordContext(t, "/api/material/CreateVisualValidateSession", `{"CallbackURL":"https://gw.example/cb","ProjectName":"default"}`)
	Proxy().ServeHTTP(httptest.NewRecorder(), c.Request)

	s, err := model.GetVisualValidateSessionByToken("bt-1")
	require.NoError(t, err)
	assert.Equal(t, 42, s.UserId)
	assert.Equal(t, "tokenhub", s.Provider)
	assert.Equal(t, "https://h5.example/1", s.H5Link)
	assert.Equal(t, "https://gw.example/cb", s.CallbackURL)
	assert.Equal(t, model.VisualSessionProcessing, s.Status)
}

// TestProxy_rewritesClientCallbackToGateway 端到端验证对外 API 的回调中转：
// 客户传自己的 CallbackURL → 发给上游的是网关回调（会话表 callback_url），
// 客户原回调存入 custom_callback（认证后 302 到它）；响应 Result.CallbackURL 替换回客户原值。
func TestProxy_rewritesClientCallbackToGateway(t *testing.T) {
	model.DB = openMaterialRecordDB(t)

	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		// 火山会 echo 请求里的 CallbackURL（网关回调）
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"BytedToken":"bt-rw","H5Link":"https://h5.example/rw","CallbackURL":"https://gw.example.com/v1/material/h5-callback"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"callback_url":"https://gw.example.com/v1/material/h5-callback"}`, upstream.URL))

	c := newRecordContext(t, "/api/material/CreateVisualValidateSession", `{"CallbackURL":"https://client.example.com/cb","ProjectName":"default"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	// 上游收到的是网关回调
	assert.Contains(t, gotBody, `"CallbackURL":"https://gw.example.com/v1/material/h5-callback"`)

	// 响应给客户的是其原回调（替换回），而非网关内置
	assert.Contains(t, rec.Body.String(), `"CallbackURL":"https://client.example.com/cb"`)
	assert.NotContains(t, rec.Body.String(), `"CallbackURL":"https://gw.example.com/v1/material/h5-callback"`)

	// 会话类响应为直接格式（无 Result 信封）：顶层 BytedToken/H5Link/CallbackURL。
	var direct struct {
		BytedToken  string `json:"BytedToken"`
		H5Link      string `json:"H5Link"`
		CallbackURL string `json:"CallbackURL"`
		Result      any    `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "bt-rw", direct.BytedToken)
	assert.Equal(t, "https://h5.example/rw", direct.H5Link)
	assert.Equal(t, "https://client.example.com/cb", direct.CallbackURL)
	assert.Nil(t, direct.Result) // 无信封，证明会话类直接格式

	s, err := model.GetVisualValidateSessionByToken("bt-rw")
	require.NoError(t, err)
	assert.Equal(t, "https://client.example.com/cb", s.CustomCallback)
	assert.Equal(t, "https://gw.example.com/v1/material/h5-callback", s.CallbackURL)
}

// TestProxy_listAssetGroupsLocalIsolation 验证对外 ListAssetGroups 查本地归属表并按用户隔离：
// 只返回当前 API key 用户的组，不查上游、不越权看到其他用户的组。
func TestProxy_listAssetGroupsLocalIsolation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "g-user42", "组A", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(7, "tokenhub", "g-user7", "组B", "AIGC"))
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"","callback_url":"https://gw.example/cb"}`)

	c := newRecordContext(t, "/api/material/ListAssetGroups", `{}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		Result struct {
			TotalCount int `json:"TotalCount"`
			Items      []struct {
				Id        string `json:"Id"`
				Name      string `json:"Name"`
				GroupType string `json:"GroupType"`
			} `json:"Items"`
		} `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 1, out.Result.TotalCount)
	require.Len(t, out.Result.Items, 1)
	assert.Equal(t, "g-user42", out.Result.Items[0].Id)
	assert.Equal(t, "组A", out.Result.Items[0].Name)
	assert.Equal(t, "AIGC", out.Result.Items[0].GroupType)
}

// TestProxy_getVisualValidateResult_localCacheHit 验证对外查询认证结果：本地会话已结算
// （group_id 非空且 active）时直接返回，不查上游。
func TestProxy_getVisualValidateResult_localCacheHit(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "volcengine", "真人组1", "bt-cache", "https://h5/1", "https://gw/cb", "default", ""))
	require.NoError(t, model.CompleteVisualValidateSession("bt-cache", "group-cached", model.VisualSessionActive))
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"","provider":"volcengine"}`)

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"bt-cache"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	// 统一 tokenhub 直接格式
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "group-cached", direct.GroupId)
}

// TestProxy_getVisualValidateResult_backfillsSession 验证对外查询认证结果：本地未结算时
// 查上游拿到 GroupId → 回填会话（active + group_id）并记录归属（幂等）。
func TestProxy_getVisualValidateResult_backfillsSession(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "tokenhub", "真人组1", "bt-q", "https://h5/1", "https://gw/cb", "default", ""))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"GroupId":"group-backfill"}`)) // tokenhub 直接格式
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"provider":"tokenhub"}`, upstream.URL))

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"bt-q"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "group-backfill", direct.GroupId)

	// 回填：会话 active + 归属记录
	s, err := model.GetVisualValidateSessionByToken("bt-q")
	require.NoError(t, err)
	assert.Equal(t, model.VisualSessionActive, s.Status)
	assert.Equal(t, "group-backfill", s.GroupId)
	groups, err := model.GetMaterialGroups(0, "tokenhub", "LivenessFace")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "group-backfill", groups[0].SeedanceAssetGroupId)
}

// TestProxy_getVisualValidateResult_userIsolation 验证对外查询认证结果跨租户隔离：
// 会话属于其他用户（user 7）时，当前用户（user 42）查询视为未完成（返回空 GroupId），不泄露。
func TestProxy_getVisualValidateResult_userIsolation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(7, "volcengine", "真人组1", "bt-iso", "https://h5/1", "https://gw/cb", "default", ""))
	require.NoError(t, model.CompleteVisualValidateSession("bt-iso", "group-iso", model.VisualSessionActive))
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"","provider":"volcengine"}`)

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"bt-iso"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "", direct.GroupId) // 不泄露他人组
}

// TestProxy_getVisualValidateResult_validatePending 验证火山认证未完成（ValidatePending）时
// 对外查询返回 200 + 空 GroupId（与 tokenhub 200+空契约一致），而非 502，客户端继续轮询。
func TestProxy_getVisualValidateResult_validatePending(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "volcengine", "真人组1", "bt-pending", "https://h5/1", "https://gw/cb", "default", ""))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"ValidatePending","Message":"not yet"}}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"provider":"volcengine","volcengine_access_key":"ak","volcengine_secret_key":"sk","volcengine_base_url":%q}`, upstream.URL, upstream.URL))

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"bt-pending"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code) // 200 + 空 GroupId（继续轮询）
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "", direct.GroupId)
}

// TestProxy_getVisualValidateResult_noSessionFailsClosed 验证会话不存在的 token fail-closed：
// 不查上游（隔离缺口），返回空 GroupId。
func TestProxy_getVisualValidateResult_noSessionFailsClosed(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"","provider":"tokenhub"}`)

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"ghost-token"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "", direct.GroupId)
}

// TestProxy_recordsVisualValidateSessionName 验证对外 API 代理路径带 Name 时，会话表记录该组名
// （上游忽略 Name 但不报错，本地存作认证后归属组名）。
func TestProxy_recordsVisualValidateSessionName(t *testing.T) {
	model.DB = openMaterialRecordDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"BytedToken":"bt-n","H5Link":"https://h5.example/n"}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"callback_url":"https://gw.example/cb"}`, upstream.URL))

	c := newRecordContext(t, "/api/material/CreateVisualValidateSession", `{"Name":"我的真人组","CallbackURL":"https://gw.example/cb","ProjectName":"default"}`)
	Proxy().ServeHTTP(httptest.NewRecorder(), c.Request)

	s, err := model.GetVisualValidateSessionByToken("bt-n")
	require.NoError(t, err)
	assert.Equal(t, "我的真人组", s.Name)
}

// TestProxy_groupOperationOwnershipIsolation 验证对外组操作（Get/Update/DeleteAssetGroup）与
// ListAssets 的归属校验（防 IDOR）：他人组返回 404（不查上游），本人组正常放行。
func TestProxy_groupOperationOwnershipIsolation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(7, "tokenhub", "group-other", "他人组", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "group-mine", "我的组", "AIGC"))
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"https://upstream.invalid"}`)

	// 他人组 → 404（校验拦截，不触发上游）
	c := newRecordContext(t, "/api/material/GetAssetGroup", `{"Id":"group-other"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// 本人组 → 校验通过放行（上游不可达 → 502 而非 404，证明未拦截）
	c2 := newRecordContext(t, "/api/material/GetAssetGroup", `{"Id":"group-mine"}`)
	rec2 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec2, c2.Request)
	assert.NotEqual(t, http.StatusNotFound, rec2.Code)

	// DeleteAssetGroup 他人组 → 404
	c3 := newRecordContext(t, "/api/material/DeleteAssetGroup", `{"Id":"group-other"}`)
	rec3 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec3, c3.Request)
	require.Equal(t, http.StatusNotFound, rec3.Code)

	// ListAssets 含他人组 → 404
	c4 := newRecordContext(t, "/api/material/ListAssets", `{"Filter":{"GroupIds":["group-other"]},"PageNumber":1,"PageSize":10}`)
	rec4 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec4, c4.Request)
	require.Equal(t, http.StatusNotFound, rec4.Code)
}

// TestProxy_assetWriteOwnership 验证 CreateAsset 目标组归属校验与 ListAssets 空 GroupIds 拒绝。
func TestProxy_assetWriteOwnership(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(7, "tokenhub", "group-other", "他人组", "AIGC"))
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"https://upstream.invalid"}`)

	// CreateAsset 到他人组 → 404
	c := newRecordContext(t, "/api/material/CreateAsset", `{"GroupId":"group-other","URL":"https://x","Name":"a","AssetType":"Image"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// ListAssets 空 GroupIds → 400
	c2 := newRecordContext(t, "/api/material/ListAssets", `{"Filter":{},"PageNumber":1,"PageSize":10}`)
	rec2 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec2, c2.Request)
	require.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestProxy_nameXSSRejected 验证建组/真人会话名称含 HTML 危险字符（<>）被拒绝（防存储型 XSS）。
func TestProxy_nameXSSRejected(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":"https://upstream.invalid"}`)

	// 代理 CreateAssetGroup 含 <> → 400
	c := newRecordContext(t, "/api/material/CreateAssetGroup", `{"Name":"<script>alert(1)</script>","GroupType":"AIGC"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// 代理 CreateVisualValidateSession 含 <> → 400
	c2 := newRecordContext(t, "/api/material/CreateVisualValidateSession", `{"Name":"<script>alert(1)</script>","CallbackURL":"https://gw.example/cb"}`)
	rec2 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec2, c2.Request)
	require.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestProxy_nameTooLongRejected 验证建组/真人会话名称超过 255 rune 被拒绝（防镜像表列宽溢出），
// 且恰好 255 rune 的边界名放行（按 rune 计数而非 byte，多字节 UTF-8 不误伤）。
func TestProxy_nameTooLongRejected(t *testing.T) {
	model.DB = openMaterialRecordDB(t)

	// 256 个中文字符 = 256 rune（每字 3 byte）
	longName := strings.Repeat("中", 256)

	// 代理 CreateAssetGroup 超长名 → 400
	c := newRecordContext(t, "/api/material/CreateAssetGroup", fmt.Sprintf(`{"Name":%q,"GroupType":"AIGC"}`, longName))
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// 代理 CreateVisualValidateSession 超长名 → 400
	c2 := newRecordContext(t, "/api/material/CreateVisualValidateSession", fmt.Sprintf(`{"Name":%q,"CallbackURL":"https://gw.example/cb"}`, longName))
	rec2 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec2, c2.Request)
	require.Equal(t, http.StatusBadRequest, rec2.Code)

	// 边界：恰好 255 rune 放行（转上游成功 → 200）
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"group-255"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	boundaryName := strings.Repeat("中", 255)
	c3 := newRecordContext(t, "/api/material/CreateAssetGroup", fmt.Sprintf(`{"Name":%q,"GroupType":"AIGC"}`, boundaryName))
	rec3 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec3, c3.Request)
	require.Equal(t, http.StatusOK, rec3.Code)

	// 边界名确实以完整 255 rune 落库（镜像表列宽安全的直接证据，非仅请求放行）
	groups, err := model.GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, boundaryName, groups[0].Name)
}

// TestProxy_materialNameTooLongActions 验证代理层对超长名称（>255 rune）在全部 Name 动作上
// 统一 400 拒绝（防镜像表列宽溢出，不把超长名透传上游触发 tokenhub 500）。
// SQL 注入负载按产品决策不在网关拦截（注入防御在上游后端），故不在此断言。
func TestProxy_materialNameTooLongActions(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "group-own", "g", "AIGC"))
	setConfig(t, `{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":""}`)

	longName := strings.Repeat("中", 256)

	// CreateAssetGroup：超长名 → 400（校验在透传上游前），锁定 tokenhub 格式错误信封
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, newRecordContext(t, "/api/material/CreateAssetGroup", fmt.Sprintf(`{"Name":%q,"GroupType":"AIGC"}`, longName)).Request)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{},"ResponseError":{"Code":"InvalidParameter","Message":"invalid parameter"}}`, rec.Body.String())

	// CreateVisualValidateSession：超长名 → 400
	rec2 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec2, newRecordContext(t, "/api/material/CreateVisualValidateSession", fmt.Sprintf(`{"Name":%q}`, longName)).Request)
	require.Equal(t, http.StatusBadRequest, rec2.Code)

	// CreateAsset：超长名 → 400（归属组有效，先过归属校验再拒绝名称）
	rec3 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec3, newRecordContext(t, "/api/material/CreateAsset", fmt.Sprintf(`{"GroupId":"group-own","Name":%q,"URL":"https://e.com/a.jpg","AssetType":"Image"}`, longName)).Request)
	require.Equal(t, http.StatusBadRequest, rec3.Code)

	// UpdateAssetGroup：超长名 → 400（归属组有效）
	rec5 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec5, newRecordContext(t, "/api/material/UpdateAssetGroup", fmt.Sprintf(`{"Id":"group-own","Name":%q}`, longName)).Request)
	require.Equal(t, http.StatusBadRequest, rec5.Code)

	// UpdateAsset：超长名 → 400
	rec6 := httptest.NewRecorder()
	Proxy().ServeHTTP(rec6, newRecordContext(t, "/api/material/UpdateAsset", fmt.Sprintf(`{"Id":"Asset-x","Name":%q}`, longName)).Request)
	require.Equal(t, http.StatusBadRequest, rec6.Code)
}

// TestProxy_getVisualValidateResult_tokenhubInternalError 验证 tokenhub 未认证（上游 500 InternalError，
// 有本地会话）→ 返回 200 + 空 GroupId 继续轮询（非 502）。
func TestProxy_getVisualValidateResult_tokenhubInternalError(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "tokenhub", "真人组1", "bt-ie", "https://h5/1", "https://gw/cb", "default", ""))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ResponseError":{"Code":"InternalError","Message":"internal error"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"bt-ie"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "", direct.GroupId)
}

// TestProxy_getVisualValidateResult_volcengineNotFound 验证火山未认证（上游 NotFound，有本地会话）
// → 返回 200 + 空 GroupId（统一 tokenhub 直接格式，继续轮询），非 502。
func TestProxy_getVisualValidateResult_volcengineNotFound(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddVisualValidateSession(42, "volcengine", "真人组1", "bt-nf", "https://h5/1", "https://gw/cb", "default", ""))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"NotFound","Message":"token not found"}}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q,"provider":"volcengine","volcengine_access_key":"ak","volcengine_secret_key":"sk","volcengine_base_url":%q}`, upstream.URL, upstream.URL))
	defer setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y","base_url":""}`)

	c := newRecordContext(t, "/api/material/GetVisualValidateResult", `{"BytedToken":"bt-nf"}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusOK, rec.Code)
	var direct struct {
		GroupId string `json:"GroupId"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &direct))
	assert.Equal(t, "", direct.GroupId)
}

// TestProxy_getAssetRequiresOwnership 验证路径入口的 GetAsset 与 Update/Delete
// 同口径：经上游 GetAsset 反查 GroupId 校验归属，非本人素材 404 且不再读取。
func TestProxy_getAssetRequiresOwnership(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "group-owned", "我的组", "AIGC"))

	for _, tc := range []struct {
		name       string
		body       string
		groupID    string
		wantStatus int
		wantCalls  int
	}{
		{name: "foreign asset", body: `{"Id":"asset-x"}`, groupID: "group-foreign", wantStatus: http.StatusNotFound, wantCalls: 1},
		{name: "foreign asset lowercase key", body: `{"id":"asset-x"}`, groupID: "group-foreign", wantStatus: http.StatusNotFound, wantCalls: 1},
		{name: "conflicting keys", body: `{"Id":"asset-x","id":"asset-y"}`, wantStatus: http.StatusNotFound, wantCalls: 0},
		{name: "owned asset", body: `{"Id":"asset-x"}`, groupID: "group-owned", wantStatus: http.StatusOK, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"asset-x","GroupId":"` + tc.groupID + `"}}`))
			}))
			defer upstream.Close()
			setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

			c := newRecordContext(t, "/api/material/GetAsset", tc.body)
			rec := httptest.NewRecorder()
			Proxy().ServeHTTP(rec, c.Request)

			require.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantCalls, calls, "归属校验通过后直接复用探测响应，不再读取")
		})
	}
}

// TestProxy_getAssetOwnershipVolcengine 验证火山直连 provider 的素材归属探测
// 同样读取 Result.GroupId（ADR 0024 要求两个 provider 各验证一次）。
func TestProxy_getAssetOwnershipVolcengine(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "volcengine", "group-volc", "组", "AIGC"))
	defer setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y","base_url":""}`)

	for _, tc := range []struct {
		name       string
		groupID    string
		wantStatus int
	}{
		{name: "foreign asset", groupID: "group-foreign", wantStatus: http.StatusNotFound},
		{name: "owned asset", groupID: "group-volc", wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.NotEmpty(t, r.Header.Get("Authorization"), "火山请求必须带 SigV4 签名")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"req-1"},"Result":{"Id":"asset-x","GroupId":"` + tc.groupID + `"}}`))
			}))
			defer upstream.Close()
			setConfig(t, fmt.Sprintf(`{"enabled":true,"provider":"volcengine","volcengine_access_key":"ak","volcengine_secret_key":"sk","volcengine_base_url":%q}`, upstream.URL))

			c := newRecordContext(t, "/api/material/GetAsset", `{"Id":"asset-x"}`)
			rec := httptest.NewRecorder()
			Proxy().ServeHTTP(rec, c.Request)

			require.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, 1, calls)
			if tc.wantStatus == http.StatusOK {
				assert.JSONEq(t, `{"ResponseMetadata":{"RequestId":"req-1"},"Result":{"Id":"asset-x","GroupId":"`+tc.groupID+`"}}`, rec.Body.String())
			}
		})
	}
}

// TestBuildLocalAssetGroupsResponse_failsClosedWithoutUser 验证缺少用户上下文时绝不按
// 「不过滤 user_id」查全表：模型层把 userID<=0 当作无过滤条件，这里必须先拦住。
func TestBuildLocalAssetGroupsResponse_failsClosedWithoutUser(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "tokenhub", "group-42", "组", "AIGC"))
	setConfig(t, `{"enabled":true,"provider":"tokenhub","ak":"pak_x","sk":"psk_y"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/material/ListAssetGroups", strings.NewReader(`{}`))
	data, err := buildLocalAssetGroupsResponse(req, []byte(`{}`), arkVersion, "")
	require.NoError(t, err)

	var out struct {
		Result struct {
			TotalCount int              `json:"TotalCount"`
			Items      []AssetGroupItem `json:"Items"`
		} `json:"Result"`
	}
	require.NoError(t, common.Unmarshal(data, &out))
	assert.Zero(t, out.Result.TotalCount)
	assert.Empty(t, out.Result.Items)
}

// TestProxy_listAssetGroupsDBErrorUsesEnvelope 验证本地组列表查询失败时仍按旧
// 接口契约返回 tokenhub 错误信封，而不是纯文本 500。
func TestProxy_listAssetGroupsDBErrorUsesEnvelope(t *testing.T) {
	db := openMaterialRecordDB(t)
	model.DB = db
	require.NoError(t, db.Migrator().DropTable(&model.UserMaterialGroup{}))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("ListAssetGroups 不得查上游")
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	c := newRecordContext(t, "/api/material/ListAssetGroups", `{}`)
	rec := httptest.NewRecorder()
	Proxy().ServeHTTP(rec, c.Request)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"ResponseMetadata":{},"ResponseError":{"Code":"InternalError","Message":"list asset groups failed"}}`, rec.Body.String())
}
