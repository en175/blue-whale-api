package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 契约：报价接口 RequestAmount 对「恰好 0.01 元」的档位应放行（返回 success 报价），
// 而非判定「充值金额过低」拒绝。回归 IK72JT：0.01 元档位充值被报价接口拒绝。
//
// 复现条件（测试站实测）：Price=1、AmountDiscount[1]=0.01、MinTopUp=1、分组倍率=1
// → getPayMoney(1) = 1 × 1 × 1 × 0.01 = 0.01 元，恰好命中 0.01 边界。
func TestRequestAmount_AllowsExactlyZeroPointZeroOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openAccountTopUpTestDB(t)

	// 设置报价全局状态：Price=1、折扣[1]=0.01、MinTopUp=1、分组倍率=1
	originalPrice := operation_setting.Price
	originalMinTopUp := operation_setting.MinTopUp
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for k, v := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[k] = v
	}
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()

	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.MinTopUp = originalMinTopUp
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	operation_setting.Price = 1
	operation_setting.MinTopUp = 1
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{1: 0.01}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	// 构造用户 + token（报价接口走 AccountTokenOrUserAuth，Token 属主即订单归属）
	_, tokenKey := insertAccountTopUpUser(t, db, common.TokenStatusEnabled)

	router := setupAccountTopUpRouter()
	request := httptest.NewRequest(http.MethodPost, "/api/user/amount", strings.NewReader(`{"amount":1}`))
	request.Header.Set("Authorization", "Bearer "+tokenKey)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var payload struct {
		Message string `json:"message"`
	}
	require.NoError(t, common.UnmarshalJsonStr(recorder.Body.String(), &payload))
	assert.Equal(t, "success", payload.Message, "报价 0.01 元不应被判「充值金额过低」")
	assert.Contains(t, recorder.Body.String(), "0.01")
}
