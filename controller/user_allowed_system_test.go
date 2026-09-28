package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// performJSONWithHeader 在 performJSON 基础上允许注入额外请求头（如 X-Client-System）。
func performJSONWithHeader(router *gin.Engine, method, path string, body any, extraHeader map[string]string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeader {
		req.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

type systemAllowedResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		AllowedSystems string `json:"allowed_systems"`
	} `json:"data"`
}

// setupSystemAllowedTest 建立带白名单场景的路由：注册、登录、身份查询。
func setupSystemAllowedTest(t *testing.T) *gin.Engine {
	t.Helper()
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.PasswordLoginEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0
	constant.GenerateDefaultToken = true
	t.Cleanup(func() {
		constant.GenerateDefaultToken = false
	})
	router := gin.New()
	router.Use(sessions.Sessions("system-allowed", cookie.NewStore([]byte("test-secret"))))
	router.POST("/api/user/register", Register)
	router.POST("/api/user/login", Login)
	router.GET("/api/token/identity", GetTokenIdentity)
	return router
}

func decodeSystemAllowed(t *testing.T, recorder *httptest.ResponseRecorder) systemAllowedResponse {
	t.Helper()
	var response systemAllowedResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

func TestLoginEnforcesAllowedSystemsWhitelist(t *testing.T) {
	router := setupSystemAllowedTest(t)

	// 注册（默认 api）+ 显式把白名单收紧为仅 bluewhale
	recorder := performJSONWithHeader(router, http.MethodPost, "/api/user/register",
		map[string]string{"username": "bw-user", "password": "password123"},
		map[string]string{"X-Client-System": "bluewhale"})
	require.True(t, decodeSystemAllowed(t, recorder).Success)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "bw-user").First(&user).Error)
	user.AllowedSystems = "bluewhale"
	require.NoError(t, model.DB.Model(&user).Update("allowed_systems", "bluewhale").Error)

	t.Run("bluewhale header allowed", func(t *testing.T) {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/login",
			map[string]string{"username": "bw-user", "password": "password123"},
			map[string]string{"X-Client-System": "bluewhale"})
		decoded := decodeSystemAllowed(t, resp)
		require.Equal(t, http.StatusOK, resp.Code)
		require.True(t, decoded.Success, decoded.Message)
	})

	t.Run("missing header defaults to api and is rejected", func(t *testing.T) {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/login",
			map[string]string{"username": "bw-user", "password": "password123"}, nil)
		decoded := decodeSystemAllowed(t, resp)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.False(t, decoded.Success)
		// 测试环境 i18n bundle 未初始化，ApiErrorI18n 回退返回原始 key；校验 key 而非译文
		assert.Equal(t, i18n.MsgUserSystemNotAllowed, decoded.Message)
	})

	t.Run("unknown header defaults to api and is rejected", func(t *testing.T) {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/login",
			map[string]string{"username": "bw-user", "password": "password123"},
			map[string]string{"X-Client-System": "foobar"})
		decoded := decodeSystemAllowed(t, resp)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.False(t, decoded.Success)
	})

	t.Run("case-insensitive header accepted", func(t *testing.T) {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/login",
			map[string]string{"username": "bw-user", "password": "password123"},
			map[string]string{"X-Client-System": "BlueWhale"})
		decoded := decodeSystemAllowed(t, resp)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.True(t, decoded.Success, decoded.Message)
	})
}

func TestLoginEmptyAllowedSystemsAllowsAll(t *testing.T) {
	router := setupSystemAllowedTest(t)

	// 存量账号（allowed_systems 空串 = 全放行）：注册时不带头 → 默认 api，再显式清空模拟存量数据
	recorder := performJSONWithHeader(router, http.MethodPost, "/api/user/register",
		map[string]string{"username": "open-user", "password": "password123"}, nil)
	require.True(t, decodeSystemAllowed(t, recorder).Success)
	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "open-user").First(&user).Error)
	require.NoError(t, model.DB.Model(&user).Update("allowed_systems", "").Error)

	for _, header := range []map[string]string{
		{"X-Client-System": "bluewhale"},
		nil,
		{"X-Client-System": "api"},
	} {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/login",
			map[string]string{"username": "open-user", "password": "password123"}, header)
		decoded := decodeSystemAllowed(t, resp)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.True(t, decoded.Success, decoded.Message)
	}
}

func TestRegisterDefaultsAllowedSystemsFromSource(t *testing.T) {
	router := setupSystemAllowedTest(t)

	t.Run("bluewhale header sets bluewhale", func(t *testing.T) {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/register",
			map[string]string{"username": "src-bw", "password": "password123"},
			map[string]string{"X-Client-System": "bluewhale"})
		require.True(t, decodeSystemAllowed(t, resp).Success)

		var user model.User
		require.NoError(t, model.DB.Where("username = ?", "src-bw").First(&user).Error)
		assert.Equal(t, "bluewhale", user.AllowedSystems)
	})

	t.Run("missing header defaults to api", func(t *testing.T) {
		resp := performJSONWithHeader(router, http.MethodPost, "/api/user/register",
			map[string]string{"username": "src-api", "password": "password123"}, nil)
		require.True(t, decodeSystemAllowed(t, resp).Success)

		var user model.User
		require.NoError(t, model.DB.Where("username = ?", "src-api").First(&user).Error)
		assert.Equal(t, "api", user.AllowedSystems)
	})
}

func TestRegisterIgnoresClientInjectedAllowedSystems(t *testing.T) {
	router := setupSystemAllowedTest(t)

	// 注册请求体里塞 allowed_systems，必须被忽略：仍按来源头落库
	resp := performJSONWithHeader(router, http.MethodPost, "/api/user/register",
		map[string]string{
			"username":        "inject-user",
			"password":        "password123",
			"allowed_systems": "api", // 攻击者想自授控制台登录权
		},
		map[string]string{"X-Client-System": "bluewhale"})
	require.True(t, decodeSystemAllowed(t, resp).Success)

	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "inject-user").First(&user).Error)
	assert.Equal(t, "bluewhale", user.AllowedSystems)
}

func TestTokenIdentityReturnsAllowedSystems(t *testing.T) {
	router := setupSystemAllowedTest(t)

	// 注册拿 sk token
	resp := performJSONWithHeader(router, http.MethodPost, "/api/user/register",
		map[string]string{"username": "ident-user", "password": "password123"},
		map[string]string{"X-Client-System": "bluewhale"})
	registered := decodeSystemAllowed(t, resp)
	require.True(t, registered.Success)

	// 需要真实 sk 才能走 ResolveUserByTokenKey
	var token model.Token
	require.NoError(t, model.DB.Where("user_id = ?", userIDFromRegistration(t, resp)).First(&token).Error)

	identResp := performJSONWithHeader(router, http.MethodGet, "/api/token/identity", nil,
		map[string]string{"Authorization": "Bearer sk-" + token.Key})
	require.Equal(t, http.StatusOK, identResp.Code)
	var identity struct {
		Success bool `json:"success"`
		Data    struct {
			AllowedSystems string `json:"allowed_systems"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(identResp.Body.Bytes(), &identity))
	assert.Equal(t, "bluewhale", identity.Data.AllowedSystems)
}

func userIDFromRegistration(t *testing.T, recorder *httptest.ResponseRecorder) int {
	t.Helper()
	var response struct {
		Data struct {
			UserID int `json:"user_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response.Data.UserID
}
