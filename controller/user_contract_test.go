package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountContractResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID       int    `json:"id"`
		UserID   int    `json:"user_id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Token    string `json:"token"`
		Status   int    `json:"status"`
		Role     int    `json:"role"`
	} `json:"data"`
}

func setupAccountContractTest(t *testing.T, migrateToken bool) *gin.Engine {
	t.Helper()
	db := openTokenControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	if migrateToken {
		require.NoError(t, db.AutoMigrate(&model.Token{}))
	}
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
	router.Use(sessions.Sessions("account-contract", cookie.NewStore([]byte("test-secret"))))
	router.POST("/api/user/register", Register)
	router.POST("/api/user/login", Login)
	router.GET("/api/token/identity", GetTokenIdentity)
	return router
}

func TestLoginByUsernameOrEmailReturnsSameStableAccountContract(t *testing.T) {
	router := setupAccountContractTest(t, true)
	user := model.User{
		Username: "login-user", Email: "login@example.com", Password: "password123",
		DisplayName: "login-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	token := model.Token{
		Name: "default", Key: "login-contract-key", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true,
	}
	require.NoError(t, user.InsertWithDefaultToken(0, &token))

	for _, identifier := range []string{"login-user", "login@example.com"} {
		recorder := performJSON(router, http.MethodPost, "/api/user/login", map[string]string{
			"username": identifier,
			"password": "password123",
		}, "")
		response := decodeAccountContract(t, recorder)
		assert.Equal(t, http.StatusOK, recorder.Code, identifier)
		assert.Equal(t, user.Id, response.Data.ID, identifier)
		assert.Equal(t, user.Id, response.Data.UserID, identifier)
		assert.Equal(t, "sk-"+token.Key, response.Data.Token, identifier)
	}
}

func TestLoginWithoutDefaultTokenReturnsConfigurationError(t *testing.T) {
	router := setupAccountContractTest(t, true)
	user := model.User{
		Username: "missing-token", Password: "password123", DisplayName: "missing-token",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))
	recorder := performJSON(router, http.MethodPost, "/api/user/login", map[string]string{
		"username": "missing-token",
		"password": "password123",
	}, "")
	assert.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
}

func performJSON(router *gin.Engine, method, path string, body any, token string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func decodeAccountContract(t *testing.T, recorder *httptest.ResponseRecorder) accountContractResponse {
	t.Helper()
	var response accountContractResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

func TestRegisterCreatesDefaultTokenAndReturnsStableAccountContract(t *testing.T) {
	router := setupAccountContractTest(t, true)
	recorder := performJSON(router, http.MethodPost, "/api/user/register", map[string]string{
		"username": "alice",
		"password": "password123",
	}, "")

	response := decodeAccountContract(t, recorder)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, response.Success)
	assert.NotZero(t, response.Data.UserID)
	assert.Equal(t, "alice", response.Data.Username)
	assert.Regexp(t, `^sk-.+`, response.Data.Token)
}

func TestRegisterRollsBackUserWhenDefaultTokenCannotBeCreated(t *testing.T) {
	router := setupAccountContractTest(t, false)
	recorder := performJSON(router, http.MethodPost, "/api/user/register", map[string]string{
		"username": "rollback-user",
		"password": "password123",
	}, "")

	var count int64
	require.NoError(t, model.DB.Model(&model.User{}).Where("username = ?", "rollback-user").Count(&count).Error)
	assert.Zero(t, count, recorder.Body.String())
}

func TestTokenIdentityReturnsStableUserAndRejectsDisabledUser(t *testing.T) {
	router := setupAccountContractTest(t, true)
	register := performJSON(router, http.MethodPost, "/api/user/register", map[string]string{
		"username": "identity-user",
		"password": "password123",
	}, "")
	account := decodeAccountContract(t, register).Data

	identity := performJSON(router, http.MethodGet, "/api/token/identity", nil, account.Token)
	resolved := decodeAccountContract(t, identity)
	require.Equal(t, http.StatusOK, identity.Code, identity.Body.String())
	assert.Equal(t, account.UserID, resolved.Data.UserID)

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", account.UserID).Update("status", common.UserStatusDisabled).Error)
	disabled := performJSON(router, http.MethodGet, "/api/token/identity", nil, account.Token)
	assert.Equal(t, http.StatusForbidden, disabled.Code, disabled.Body.String())
}

func TestTokenIdentityReturnsAccountRole(t *testing.T) {
	router := setupAccountContractTest(t, true)
	register := performJSON(router, http.MethodPost, "/api/user/register", map[string]string{
		"username": "role-user",
		"password": "password123",
	}, "")
	account := decodeAccountContract(t, register).Data

	identity := performJSON(router, http.MethodGet, "/api/token/identity", nil, account.Token)
	require.Equal(t, http.StatusOK, identity.Code, identity.Body.String())
	resolved := decodeAccountContract(t, identity)
	assert.Equal(t, common.RoleCommonUser, resolved.Data.Role)

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", account.UserID).Update("role", common.RoleAdminUser).Error)
	adminIdentity := performJSON(router, http.MethodGet, "/api/token/identity", nil, account.Token)
	require.Equal(t, http.StatusOK, adminIdentity.Code, adminIdentity.Body.String())
	adminResolved := decodeAccountContract(t, adminIdentity)
	assert.Equal(t, common.RoleAdminUser, adminResolved.Data.Role)
}

func TestTokenIdentityRejectsInvalidToken(t *testing.T) {
	router := setupAccountContractTest(t, true)
	invalid := performJSON(router, http.MethodGet, "/api/token/identity", nil, "sk-does-not-exist")
	assert.Equal(t, http.StatusUnauthorized, invalid.Code, invalid.Body.String())
}
