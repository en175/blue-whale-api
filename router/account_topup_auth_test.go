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

func openAccountTopUpTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.TopUp{}))
	return db
}

var accountTopUpFixtureSeq int

func insertAccountTopUpUser(t *testing.T, db *gorm.DB, tokenStatus int) (userId int, tokenKey string) {
	t.Helper()

	accountTopUpFixtureSeq++
	user := model.User{
		Username:    fmt.Sprintf("topup-user-%d", accountTopUpFixtureSeq),
		Password:    "password123",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		DisplayName: "topup-user",
		AffCode:     fmt.Sprintf("topaff%07d", accountTopUpFixtureSeq),
	}
	require.NoError(t, db.Create(&user).Error)

	token := model.Token{
		UserId:         user.Id,
		Key:            fmt.Sprintf("topuptokenkey%040d", accountTopUpFixtureSeq),
		Name:           "topup-token",
		Status:         tokenStatus,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    -1,
		RemainQuota:    500000,
		UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&token).Error)
	return user.Id, "sk-" + token.Key
}

func insertTopUpOrder(t *testing.T, db *gorm.DB, userId int, tradeNo string) {
	t.Helper()

	topup := model.TopUp{
		UserId:          userId,
		Amount:          10,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodBlueWhalePay,
		PaymentProvider: "bluewhalepay",
		CreateTime:      common.GetTimestamp(),
		Status:          "success",
	}
	require.NoError(t, db.Create(&topup).Error)
}

// setupAccountTopUpRouter mirrors the production wiring: the three BlueWhale
// topup endpoints are protected by AccountTokenOrUserAuth instead of the
// session/access-token-only UserAuth.
func setupAccountTopUpRouter() *gin.Engine {
	router := gin.New()
	router.Use(sessions.Sessions("account-topup", cookie.NewStore([]byte("test-secret"))))
	accountTopUp := router.Group("/api/user")
	accountTopUp.Use(middleware.AccountTokenOrUserAuth())
	{
		accountTopUp.GET("/topup/self", controller.GetUserTopUps)
		accountTopUp.POST("/amount", controller.RequestAmount)
		accountTopUp.POST("/bluewhalepay/pay", middleware.CriticalRateLimit(), controller.RequestBlueWhalePayPay)
	}
	return router
}

func performAccountTopUpRequest(t *testing.T, router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// 契约：充值路由以 Token 属主建立 Context，两个 User 的订单互不串号。
func TestAccountTopUpAuthIsolatesOrdersByTokenOwner(t *testing.T) {
	db := openAccountTopUpTestDB(t)
	router := setupAccountTopUpRouter()

	userAId, userAToken := insertAccountTopUpUser(t, db, common.TokenStatusEnabled)
	userBId, userBToken := insertAccountTopUpUser(t, db, common.TokenStatusEnabled)
	insertTopUpOrder(t, db, userAId, "TRADE-OWNER-A")
	insertTopUpOrder(t, db, userBId, "TRADE-OWNER-B")

	recorder := performAccountTopUpRequest(t, router, http.MethodGet, "/api/user/topup/self?keyword=TRADE-OWNER-A", userAToken)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "TRADE-OWNER-A")

	recorder = performAccountTopUpRequest(t, router, http.MethodGet, "/api/user/topup/self?keyword=TRADE-OWNER-A", userBToken)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "TRADE-OWNER-A")

	recorder = performAccountTopUpRequest(t, router, http.MethodGet, "/api/user/topup/self", userBToken)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "TRADE-OWNER-B")
	assert.NotContains(t, recorder.Body.String(), "TRADE-OWNER-A")
}

// 契约：额度耗尽的 Token 仍可充值（报价与查单），禁用 Token 与无凭证请求被拒绝。
func TestAccountTopUpAuthAllowsExhaustedAndRejectsDisabledTokens(t *testing.T) {
	db := openAccountTopUpTestDB(t)
	router := setupAccountTopUpRouter()

	userId, exhaustedToken := insertAccountTopUpUser(t, db, common.TokenStatusExhausted)
	insertTopUpOrder(t, db, userId, "TRADE-EXHAUSTED")
	recorder := performAccountTopUpRequest(t, router, http.MethodGet, "/api/user/topup/self?keyword=TRADE-EXHAUSTED", exhaustedToken)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "TRADE-EXHAUSTED")

	recorder = performAccountTopUpRequest(t, router, http.MethodPost, "/api/user/amount", exhaustedToken)
	assert.NotEqual(t, http.StatusUnauthorized, recorder.Code)

	_, disabledToken := insertAccountTopUpUser(t, db, common.TokenStatusDisabled)
	recorder = performAccountTopUpRequest(t, router, http.MethodGet, "/api/user/topup/self", disabledToken)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())

	recorder = performAccountTopUpRequest(t, router, http.MethodGet, "/api/user/topup/self", "")
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
}

// 契约：真实 API 路由必须挂载这三个充值端点。
func TestAccountTopUpRoutesAreMountedOnRealAPIRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	require.NotPanics(t, func() {
		SetApiRouter(engine)
	})

	expected := map[string]string{
		http.MethodGet + " /api/user/topup/self":        "GetUserTopUps",
		http.MethodPost + " /api/user/amount":           "RequestAmount",
		http.MethodPost + " /api/user/bluewhalepay/pay": "RequestBlueWhalePayPay",
	}
	mounted := map[string]bool{}
	for _, route := range engine.Routes() {
		mounted[route.Method+" "+route.Path] = strings.Contains(route.Handler, expected[route.Method+" "+route.Path]) && expected[route.Method+" "+route.Path] != ""
	}
	for key, handler := range expected {
		require.True(t, mounted[key], "route %s with handler %s is not mounted", key, handler)
	}
}
