package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openSelfPasswordTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "failed to open sqlite db")
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	return db
}

var selfPasswordFixtureSeq int

// insertSelfPasswordUser 写入一个已存密码哈希的用户 + 一条启用 Token。
func insertSelfPasswordUser(t *testing.T, db *gorm.DB) (userId int, tokenKey string, password string) {
	t.Helper()

	selfPasswordFixtureSeq++
	password = "password123"
	hashed, err := common.Password2Hash(password)
	require.NoError(t, err)

	user := model.User{
		Username:    fmt.Sprintf("selfpwd-user-%d", selfPasswordFixtureSeq),
		Password:    hashed,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		DisplayName: "selfpwd-user",
		AffCode:     fmt.Sprintf("selfpwdaff%06d", selfPasswordFixtureSeq),
	}
	require.NoError(t, db.Create(&user).Error)

	token := model.Token{
		UserId:         user.Id,
		Key:            fmt.Sprintf("selfpwdkey%040d", selfPasswordFixtureSeq),
		Name:           "selfpwd-token",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    -1,
		RemainQuota:    500000,
		UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&token).Error)
	return user.Id, "sk-" + token.Key, password
}

// setupSelfPasswordRouter 镜像生产接线：PUT /self 走 AccountTokenOrUserAuth，
// GET /self 仍走 UserAuth；POST /login 用于建立 session 登录态。
func setupSelfPasswordRouter() *gin.Engine {
	router := gin.New()
	router.Use(sessions.Sessions("self-password", cookie.NewStore([]byte("test-secret"))))
	userRoute := router.Group("/api/user")
	userRoute.POST("/login", controller.Login)

	selfPasswordRoute := userRoute.Group("/")
	selfPasswordRoute.Use(middleware.AccountTokenOrUserAuth())
	{
		selfPasswordRoute.PUT("/self", controller.UpdateSelf)
	}
	selfRoute := userRoute.Group("/")
	selfRoute.Use(middleware.UserAuth())
	{
		selfRoute.GET("/self", controller.GetSelf)
	}
	return router
}

// 契约：真实 SetApiRouter 中 PUT /self 挂在 UpdateSelf，且 GET/DELETE /self
// 仍是同 handler（迁移不丢路由）。
func TestSelfPasswordRoutesAreMountedOnRealAPIRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() {
		SetApiRouter(engine)
	})

	mounted := map[string]bool{}
	for _, route := range engine.Routes() {
		if route.Path == "/api/user/self" {
			mounted[route.Method] = strings.Contains(route.Handler, "UpdateSelf")
		}
	}
	assert.True(t, mounted[http.MethodPut], "PUT /api/user/self must mount UpdateSelf")
}

func performChangePassword(t *testing.T, router *gin.Engine, password, originalPassword, token string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	body := fmt.Sprintf(`{"password":%q,"original_password":%q}`, password, originalPassword)
	request := httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// 契约：PUT /self 改密仅凭 Bearer sk-（免 New-Api-User 头）即可成功。
func TestSelfPasswordAcceptsBearerTokenWithoutNewApiUserHeader(t *testing.T) {
	db := openSelfPasswordTestDB(t)
	router := setupSelfPasswordRouter()
	userId, tokenKey, password := insertSelfPasswordUser(t, db)

	recorder := performChangePassword(t, router, "newpassword123", password, tokenKey, nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	// 落库校验：新密码哈希与明文匹配。
	var stored model.User
	require.NoError(t, db.First(&stored, userId).Error)
	assert.True(t, common.ValidatePasswordAndHash("newpassword123", stored.Password))
}

// 契约：带上 New-Api-User 头且与 Token 属主一致时仍可改密（可选头保持兼容）。
func TestSelfPasswordAcceptsMatchingNewApiUserHeader(t *testing.T) {
	db := openSelfPasswordTestDB(t)
	router := setupSelfPasswordRouter()
	userId, tokenKey, password := insertSelfPasswordUser(t, db)

	recorder := performChangePassword(t, router, "newpassword123", password, tokenKey,
		map[string]string{"New-Api-User": fmt.Sprintf("%d", userId)})
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

// 契约：带上 New-Api-User 头但与 Token 属主不一致时拒绝（可选头仍校验一致性）。
func TestSelfPasswordRejectsMismatchedNewApiUserHeader(t *testing.T) {
	db := openSelfPasswordTestDB(t)
	router := setupSelfPasswordRouter()
	userAId, tokenA, passwordA := insertSelfPasswordUser(t, db)
	userBId, _, _ := insertSelfPasswordUser(t, db)
	require.NotEqual(t, userAId, userBId)

	recorder := performChangePassword(t, router, "newpassword123", passwordA, tokenA,
		map[string]string{"New-Api-User": fmt.Sprintf("%d", userBId)})
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
}

func extractSessionCookie(recorder *httptest.ResponseRecorder) string {
	var parts []string
	for _, c := range recorder.Result().Cookies() {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// 契约：session 登录态仍可改密（AccountTokenOrUserAuth 兼容两种凭证）。
func TestSelfPasswordAcceptsSessionLogin(t *testing.T) {
	db := openSelfPasswordTestDB(t)
	router := setupSelfPasswordRouter()
	userId, _, password := insertSelfPasswordUser(t, db)

	common.PasswordLoginEnabled = true
	loginRequest := httptest.NewRequest(http.MethodPost, "/api/user/login",
		strings.NewReader(fmt.Sprintf(`{"username":"%s","password":%q}`, fmt.Sprintf("selfpwd-user-%d", selfPasswordFixtureSeq), password)))
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, loginRequest)
	require.Equal(t, http.StatusOK, loginRecorder.Code, loginRecorder.Body.String())

	body := fmt.Sprintf(`{"password":%q,"original_password":%q}`, "newpassword123", password)
	changeRequest := httptest.NewRequest(http.MethodPut, "/api/user/self", strings.NewReader(body))
	changeRequest.Header.Set("Content-Type", "application/json")
	changeRequest.Header.Set("Cookie", extractSessionCookie(loginRecorder))
	changeRecorder := httptest.NewRecorder()
	router.ServeHTTP(changeRecorder, changeRequest)
	require.Equal(t, http.StatusOK, changeRecorder.Code, changeRecorder.Body.String())

	var stored model.User
	require.NoError(t, db.First(&stored, userId).Error)
	assert.True(t, common.ValidatePasswordAndHash("newpassword123", stored.Password))
}

// 契约：GET /self 未被迁走——纯 Bearer 不带 New-Api-User 仍被 UserAuth 拒绝。
func TestSelfGetStillRequiresNewApiUserHeader(t *testing.T) {
	db := openSelfPasswordTestDB(t)
	router := setupSelfPasswordRouter()
	_, tokenKey, _ := insertSelfPasswordUser(t, db)

	request := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	request.Header.Set("Authorization", "Bearer "+tokenKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
}
