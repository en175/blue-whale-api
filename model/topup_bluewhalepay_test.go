package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentMethodFromWayCode(t *testing.T) {
	tests := []struct {
		name    string
		wayCode string
		want    string
	}{
		{name: "WX_NATIVE", wayCode: "WX_NATIVE", want: PaymentMethodWxPay},
		{name: "WX_JSAPI", wayCode: "WX_JSAPI", want: PaymentMethodWxPay},
		{name: "ALI_QR", wayCode: "ALI_QR", want: PaymentMethodAlipay},
		{name: "ALI_WAP", wayCode: "ALI_WAP", want: PaymentMethodAlipay},
		{name: "empty", wayCode: "", want: PaymentMethodBlueWhalePay},
		{name: "unknown", wayCode: "FOO", want: PaymentMethodBlueWhalePay},
		{name: "wechat alias not stored", wayCode: "wechat", want: PaymentMethodBlueWhalePay},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, PaymentMethodFromWayCode(tc.wayCode))
		})
	}
}

func insertBlueWhalePayTopUp(t *testing.T, tradeNo string, userID int, paymentMethod string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentMethod,
		PaymentProvider: PaymentProviderBlueWhalePay,
		Status:          common.TopUpStatusPending,
		CreateTime:      common.GetTimestamp(),
	}
	require.NoError(t, topUp.Insert())
}

func TestRechargeBlueWhalePay_OverlaysIfCodeOnPending(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 501, 0)
	insertBlueWhalePayTopUp(t, "bwp-ifcode-overlay", 501, PaymentMethodBlueWhalePay)

	err := RechargeBlueWhalePay(context.Background(), "bwp-ifcode-overlay", "127.0.0.1", PaymentMethodWxPay)
	require.NoError(t, err)

	topUp := GetTopUpByTradeNo("bwp-ifcode-overlay")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, PaymentMethodWxPay, topUp.PaymentMethod)
	assert.Equal(t, PaymentProviderBlueWhalePay, topUp.PaymentProvider)
	assert.Equal(t, int(2*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 501))
}

func TestRechargeBlueWhalePay_IgnoresUnknownOrEmptyIfCode(t *testing.T) {
	tests := []struct {
		name          string
		tradeNo       string
		ifCode        string
		createdMethod string
		wantMethod    string
	}{
		{
			name:          "empty ifCode",
			tradeNo:       "bwp-ifcode-empty",
			ifCode:        "",
			createdMethod: PaymentMethodAlipay,
			wantMethod:    PaymentMethodAlipay,
		},
		{
			name:          "unknown ifCode",
			tradeNo:       "bwp-ifcode-unknown",
			ifCode:        "unionpay",
			createdMethod: PaymentMethodWxPay,
			wantMethod:    PaymentMethodWxPay,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 502, 0)
			insertBlueWhalePayTopUp(t, tc.tradeNo, 502, tc.createdMethod)

			err := RechargeBlueWhalePay(context.Background(), tc.tradeNo, "127.0.0.1", tc.ifCode)
			require.NoError(t, err)

			topUp := GetTopUpByTradeNo(tc.tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
			assert.Equal(t, tc.wantMethod, topUp.PaymentMethod)
		})
	}
}

func TestRechargeBlueWhalePay_DoesNotOverlayAfterSuccess(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 503, 0)
	insertBlueWhalePayTopUp(t, "bwp-ifcode-idempotent", 503, PaymentMethodAlipay)

	require.NoError(t, RechargeBlueWhalePay(context.Background(), "bwp-ifcode-idempotent", "127.0.0.1", ""))
	require.NoError(t, RechargeBlueWhalePay(context.Background(), "bwp-ifcode-idempotent", "127.0.0.1", PaymentMethodWxPay))

	topUp := GetTopUpByTradeNo("bwp-ifcode-idempotent")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, PaymentMethodAlipay, topUp.PaymentMethod)
	assert.Equal(t, int(2*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 503))
}
