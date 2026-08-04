package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openAccountTokenAuthTestDB(t *testing.T) *gorm.DB {
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

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	return db
}

var accountTokenFixtureSeq int

func insertAccountTokenFixtures(t *testing.T, db *gorm.DB, userStatus int, tokenStatus int, expiredTime int64) (userId int, tokenKey string) {
	t.Helper()

	accountTokenFixtureSeq++
	user := model.User{
		Username:    fmt.Sprintf("account-token-user-%d", accountTokenFixtureSeq),
		Password:    "password123",
		Role:        common.RoleCommonUser,
		Status:      userStatus,
		DisplayName: "account-token-user",
		AffCode:     fmt.Sprintf("aff%08d", accountTokenFixtureSeq),
	}
	require.NoError(t, db.Create(&user).Error)

	token := model.Token{
		UserId:         user.Id,
		Key:            fmt.Sprintf("accounttokenkey%040d", accountTokenFixtureSeq),
		Name:           "account-token",
		Status:         tokenStatus,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    expiredTime,
		RemainQuota:    500000,
		UnlimitedQuota: true,
	}
	require.NoError(t, db.Create(&token).Error)
	return user.Id, "sk-" + token.Key
}

func setupAccountTokenAuthRouter() *gin.Engine {
	router := gin.New()
	router.Use(sessions.Sessions("account-token-auth", cookie.NewStore([]byte("test-secret"))))
	router.GET("/account", AccountTokenAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"id":        c.GetInt("id"),
			"token_id":  c.GetInt("token_id"),
			"token_key": c.GetString("token_key"),
		})
	})
	return router
}

func performAccountTokenAuthRequest(t *testing.T, router *gin.Engine, token string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/account", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// 账户层契约：启用与额度耗尽的 Token 都能证明账户身份。
func TestAccountTokenAuthAcceptsEnabledAndExhaustedTokens(t *testing.T) {
	db := openAccountTokenAuthTestDB(t)

	userId, enabledKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusEnabled, -1)
	recorder := performAccountTokenAuthRequest(t, setupAccountTokenAuthRouter(), enabledKey)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), fmt.Sprintf(`"id":%d`, userId))
	assert.NotContains(t, recorder.Body.String(), `"id":0`)
	assert.NotEmpty(t, userId)

	db2 := openAccountTokenAuthTestDB(t)
	_, exhaustedKey := insertAccountTokenFixtures(t, db2, common.UserStatusEnabled, common.TokenStatusExhausted, -1)
	recorder = performAccountTokenAuthRequest(t, setupAccountTokenAuthRouter(), exhaustedKey)
	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

// 账户层契约：显式禁用、到期与不存在的 Token 不得证明账户身份。
func TestAccountTokenAuthRejectsDisabledExpiredAndUnknownTokens(t *testing.T) {
	db := openAccountTokenAuthTestDB(t)
	router := setupAccountTokenAuthRouter()

	_, disabledKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusDisabled, -1)
	assert.Equal(t, http.StatusUnauthorized, performAccountTokenAuthRequest(t, router, disabledKey).Code)

	_, expiredKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusExpired, common.GetTimestamp()-3600)
	assert.Equal(t, http.StatusUnauthorized, performAccountTokenAuthRequest(t, router, expiredKey).Code)

	// 状态仍是 enabled 但实际到期时间已过，也必须拒绝
	_, wallClockExpiredKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusEnabled, common.GetTimestamp()-3600)
	assert.Equal(t, http.StatusUnauthorized, performAccountTokenAuthRequest(t, router, wallClockExpiredKey).Code)

	assert.Equal(t, http.StatusUnauthorized, performAccountTokenAuthRequest(t, router, "sk-does-not-exist").Code)
}

// 账户层契约：Token 属主 User 被禁用时拒绝。
func TestAccountTokenAuthRejectsDisabledUser(t *testing.T) {
	db := openAccountTokenAuthTestDB(t)
	_, tokenKey := insertAccountTokenFixtures(t, db, common.UserStatusDisabled, common.TokenStatusEnabled, -1)

	recorder := performAccountTokenAuthRequest(t, setupAccountTokenAuthRouter(), tokenKey)
	assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

// ValidateAccountToken 是中间件共用的模型层校验：放行 enabled/exhausted，拒绝其余。
func TestValidateAccountTokenStatusRules(t *testing.T) {
	db := openAccountTokenAuthTestDB(t)

	userId, enabledKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusEnabled, -1)
	token, err := model.ValidateAccountToken(strings.TrimPrefix(enabledKey, "sk-"))
	require.NoError(t, err)
	assert.Equal(t, userId, token.UserId)

	_, exhaustedKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusExhausted, -1)
	token, err = model.ValidateAccountToken(strings.TrimPrefix(exhaustedKey, "sk-"))
	require.NoError(t, err)
	require.NotNil(t, token)

	_, disabledKey := insertAccountTokenFixtures(t, db, common.UserStatusEnabled, common.TokenStatusDisabled, -1)
	_, err = model.ValidateAccountToken(strings.TrimPrefix(disabledKey, "sk-"))
	assert.Error(t, err)

	_, err = model.ValidateAccountToken("unknown-token-key")
	assert.Error(t, err)

	_, err = model.ValidateAccountToken("")
	assert.ErrorIs(t, err, model.ErrTokenNotProvided)
}
