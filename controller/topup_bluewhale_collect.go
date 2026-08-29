package controller

import (
	"crypto/subtle"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

const (
	blueWhaleCollectTokenHeader = "X-Blue-Whale-Collect-Token"
	collectMinMoney             = 0.01
	collectMaxMoney             = 100000.0
)

type blueWhaleCollectPayRequest struct {
	UserId  int     `json:"user_id"`
	Money   float64 `json:"money"`
	Amount  int64   `json:"amount"`
	WayCode string  `json:"way_code"`
}

// CollectBlueWhalePay 产品代收下单：仅 BlueWhale 后端凭代收凭证可调，实付与额度 N 由下游指定，不走 getPayMoney。
func CollectBlueWhalePay(c *gin.Context) {
	expected := strings.TrimSpace(os.Getenv("BLUE_WHALE_TOPUP_COLLECT_TOKEN"))
	got := strings.TrimSpace(c.GetHeader(blueWhaleCollectTokenHeader))
	if expected == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "未授权"})
		return
	}

	var req blueWhaleCollectPayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.UserId <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}
	user, err := model.GetUserById(req.UserId, false)
	if err != nil || user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	if math.IsNaN(req.Money) || math.IsInf(req.Money, 0) || req.Money < collectMinMoney || req.Money > collectMaxMoney {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "实付金额超出范围"})
		return
	}
	if req.Amount < 1 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值额度无效"})
		return
	}
	quota, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(req.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
	)
	if clamp != nil || quota <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值额度超出范围"})
		return
	}

	wayCode := strings.TrimSpace(req.WayCode)
	if wayCode == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式无效"})
		return
	}

	tradeNo := fmt.Sprintf("BWC%d%d%s", req.UserId, time.Now().UnixMilli(), randstr.String(4))
	topUp := &model.TopUp{
		UserId:          req.UserId,
		Amount:          req.Amount,
		Money:           req.Money,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodFromWayCode(wayCode),
		PaymentProvider: model.PaymentProviderBlueWhalePay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), "bluewhalepay collect 创建订单失败 trade_no="+tradeNo+" err="+err.Error())
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	_, payData, err := service.BlueWhalePayUnifiedOrder(c.Request.Context(), topUp, c.ClientIP(), wayCode, "/api/bluewhalepay/notify")
	if err != nil {
		logger.LogError(c.Request.Context(), "bluewhalepay collect 下单失败 trade_no="+tradeNo+" err="+err.Error())
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_data": payData,
			"order_id": tradeNo,
		},
	})
}
