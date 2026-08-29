package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openCollectTopUpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
	return db
}

func collectPayRouter() *gin.Engine {
	r := gin.New()
	r.POST("/internal/v1/topup/pay", CollectBlueWhalePay)
	return r
}

func TestCollectBlueWhalePayRejectsMissingToken(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/topup/pay", bytes.NewBufferString(
		`{"user_id":1,"money":33.3,"amount":37,"way_code":"ALI_QR"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	collectPayRouter().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"error"`)

	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestCollectBlueWhalePayRejectsWrongToken(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/topup/pay", bytes.NewBufferString(
		`{"user_id":1,"money":33.3,"amount":37,"way_code":"ALI_QR"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Blue-Whale-Collect-Token", "not-the-secret")
	rec := httptest.NewRecorder()
	collectPayRouter().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"error"`)

	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestCollectBlueWhalePayRejectsWhenEnvUnset(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	require.NoError(t, os.Unsetenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN"))

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/topup/pay", bytes.NewBufferString(
		`{"user_id":1,"money":33.3,"amount":37,"way_code":"ALI_QR"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Blue-Whale-Collect-Token", "anything")
	rec := httptest.NewRecorder()
	collectPayRouter().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"error"`)

	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestCollectBlueWhalePayRejectsBearerInsteadOfCollectHeader(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/topup/pay", bytes.NewBufferString(
		`{"user_id":1,"money":33.3,"amount":37,"way_code":"ALI_QR"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer collect-secret")
	rec := httptest.NewRecorder()
	collectPayRouter().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"message":"error"`)
	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

var collectUserSeq int

func insertCollectUser(t *testing.T, db *gorm.DB, status int) int {
	t.Helper()
	collectUserSeq++
	user := model.User{
		Username:    fmt.Sprintf("collect-user-%d", collectUserSeq),
		Password:    "password123",
		Role:        common.RoleCommonUser,
		Status:      status,
		DisplayName: "collect-user",
		AffCode:     fmt.Sprintf("colaff%07d", collectUserSeq),
	}
	require.NoError(t, db.Create(&user).Error)
	return user.Id
}

func startFakeBlueWhalePay(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pay/unifiedOrder" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"payData":"weixin://pay/collect-test","payOrderId":"P1"}}`))
	}))
	t.Cleanup(srv.Close)
	prevURL, prevSecret, prevBase := setting.BlueWhalePayPayUrl, setting.BlueWhalePayAppSecret, setting.BlueWhalePayServerBaseUrl
	setting.BlueWhalePayPayUrl = srv.URL
	setting.BlueWhalePayAppSecret = "test-secret"
	setting.BlueWhalePayServerBaseUrl = "http://notify.test"
	t.Cleanup(func() {
		setting.BlueWhalePayPayUrl = prevURL
		setting.BlueWhalePayAppSecret = prevSecret
		setting.BlueWhalePayServerBaseUrl = prevBase
	})
}

func postCollectPay(t *testing.T, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/topup/pay", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Blue-Whale-Collect-Token", token)
	}
	rec := httptest.NewRecorder()
	collectPayRouter().ServeHTTP(rec, req)
	return rec
}

func TestCollectBlueWhalePayStoresDownstreamMoneyNotGoDiscount(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")
	startFakeBlueWhalePay(t)
	userId := insertCollectUser(t, db, common.UserStatusEnabled)

	ps := operation_setting.GetPaymentSetting()
	prevDiscount := ps.AmountDiscount
	ps.AmountDiscount = map[int]float64{37: 0.5}
	t.Cleanup(func() { ps.AmountDiscount = prevDiscount })

	body := fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37,"way_code":"WX_NATIVE"}`, userId)
	rec := postCollectPay(t, "collect-secret", body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"message":"success"`)
	assert.Contains(t, rec.Body.String(), `"pay_data":"weixin://pay/collect-test"`)
	assert.Contains(t, rec.Body.String(), `"order_id"`)

	var topUp model.TopUp
	require.NoError(t, db.First(&topUp).Error)
	assert.Equal(t, userId, topUp.UserId)
	assert.Equal(t, int64(37), topUp.Amount)
	assert.InDelta(t, 33.3, topUp.Money, 0.001)
	assert.Equal(t, model.PaymentMethodWxPay, topUp.PaymentMethod)
	assert.Equal(t, model.PaymentProviderBlueWhalePay, topUp.PaymentProvider)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestCollectBlueWhalePayRejectsMissingAndDisabledUser(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")
	startFakeBlueWhalePay(t)
	disabledId := insertCollectUser(t, db, common.UserStatusDisabled)

	rec := postCollectPay(t, "collect-secret", `{"user_id":999999,"money":33.3,"amount":37,"way_code":"ALI_QR"}`)
	assert.Contains(t, rec.Body.String(), `"message":"error"`)

	rec = postCollectPay(t, "collect-secret", fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37,"way_code":"ALI_QR"}`, disabledId))
	assert.Contains(t, rec.Body.String(), `"message":"error"`)

	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestCollectBlueWhalePayRejectsOutOfRangeMoneyAndAmount(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")
	startFakeBlueWhalePay(t)
	userId := insertCollectUser(t, db, common.UserStatusEnabled)

	cases := []string{
		fmt.Sprintf(`{"user_id":%d,"money":0,"amount":37,"way_code":"ALI_QR"}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":0.001,"amount":37,"way_code":"ALI_QR"}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":100000.01,"amount":37,"way_code":"ALI_QR"}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":0,"way_code":"ALI_QR"}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":-1,"way_code":"ALI_QR"}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":100000,"way_code":"ALI_QR"}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37.5,"way_code":"ALI_QR"}`, userId),
	}
	for _, body := range cases {
		rec := postCollectPay(t, "collect-secret", body)
		assert.Contains(t, rec.Body.String(), `"message":"error"`, body)
	}
	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestCollectBlueWhalePayRejectsEmptyWayCode(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")
	startFakeBlueWhalePay(t)
	userId := insertCollectUser(t, db, common.UserStatusEnabled)

	bodies := []string{
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37,"way_code":""}`, userId),
		fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37,"way_code":"  "}`, userId),
	}
	for _, body := range bodies {
		rec := postCollectPay(t, "collect-secret", body)
		assert.Equal(t, http.StatusOK, rec.Code, body)
		assert.Contains(t, rec.Body.String(), `"message":"error"`, body)
	}
	var n int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&n).Error)
	assert.Equal(t, int64(0), n)
}

func TestCollectBlueWhalePayKeepsPendingWhenPayFails(t *testing.T) {
	db := openCollectTopUpTestDB(t)
	t.Setenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN", "collect-secret")
	userId := insertCollectUser(t, db, common.UserStatusEnabled)
	prevURL, prevSecret := setting.BlueWhalePayPayUrl, setting.BlueWhalePayAppSecret
	setting.BlueWhalePayPayUrl = ""
	setting.BlueWhalePayAppSecret = "test-secret"
	t.Cleanup(func() {
		setting.BlueWhalePayPayUrl = prevURL
		setting.BlueWhalePayAppSecret = prevSecret
	})

	rec := postCollectPay(t, "collect-secret", fmt.Sprintf(`{"user_id":%d,"money":33.3,"amount":37,"way_code":"ALI_QR"}`, userId))
	assert.Contains(t, rec.Body.String(), `"message":"error"`)

	var topUp model.TopUp
	require.NoError(t, db.First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, userId, topUp.UserId)
}
