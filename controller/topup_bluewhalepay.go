package controller

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

// BlueWhalePayPayRequest 前端下单请求。
type BlueWhalePayPayRequest struct {
	Amount  int64   `json:"amount"`   // 充值美元额度数量（N，加 quota = N × QuotaPerUnit）
	WayCode string  `json:"way_code"` // 支付方式 ALI_QR/WX_JSAPI，空用默认 setting.BlueWhalePayWayCode
}

// RequestBlueWhalePayPay 建本地 TopUp（pending）→ HTTP 调 pay /api/pay/unifiedOrder（带 notifyUrl）→ 返 payData。
func RequestBlueWhalePayPay(c *gin.Context) {
	var req BlueWhalePayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额必须大于0"})
		return
	}

	// wayCode：空用默认 setting.BlueWhalePayWayCode
	wayCode := req.WayCode
	if wayCode == "" {
		wayCode = setting.BlueWhalePayWayCode
	}

	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	// Amount 转换：TOKENS 显示模式下 req.Amount 是 quota，需 ÷ QuotaPerUnit 转美元额度（对齐 RequestEpay，防超额充值）
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = decimal.NewFromInt(req.Amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	tradeNo := fmt.Sprintf("BWP%d%d%s", userId, time.Now().UnixMilli(), randstr.String(4))
	topUp := &model.TopUp{
		UserId:          userId,
		Amount:          amount,   // 美元额度数量（N，加 quota = N × QuotaPerUnit）
		Money:           payMoney, // 实付金额（元，getPayMoney 算）
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodBlueWhalePay,
		PaymentProvider: model.PaymentProviderBlueWhalePay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		common.SysError("bluewhalepay 创建订单失败 trade_no=" + tradeNo + " err=" + err.Error())
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	_, payData, err := service.BlueWhalePayRequestUnifiedOrder(topUp, c.ClientIP(), wayCode)
	if err != nil {
		common.SysError("bluewhalepay 下单失败 trade_no=" + tradeNo + " err=" + err.Error())
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

// BlueWhalePayNotify pay 商户通知回调入口（pay confirmSuccess → PayMchNotifyMQ → GET notifyUrl）。
// pay createNotifyUrl 把订单字段 + reqTime + sign 拼 URL query 发 GET；这里验签后加额度。
// 路由 GET+POST 都挂（兼容）；返回字符串 "success" 通知 pay 停止重试。
func BlueWhalePayNotify(c *gin.Context) {
	if !isBlueWhalePayEnabled() {
		common.SysError("bluewhalepay notify 被拒绝 reason=disabled path=" + c.Request.RequestURI)
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// 合并 query + form 参数（pay createNotifyUrl 走 GET query，兼容 POST form）
	params := map[string]string{}
	for k, vs := range c.Request.URL.Query() {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	_ = c.Request.ParseForm()
	for k, vs := range c.Request.PostForm {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}

	sign := params["sign"]
	delete(params, "sign")
	expectSign := service.BlueWhalePaySign(params, setting.BlueWhalePayAppSecret)
	if len(sign) == 0 || subtle.ConstantTimeCompare([]byte(sign), []byte(expectSign)) != 1 {
		common.SysError("bluewhalepay notify 验签失败 mchOrderNo=" + params["mchOrderNo"])
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 防御：仅支付成功(state=2)才加额度，忽略失败/关闭通知（pay 当前只发成功通知，防未来扩展）
	if params["state"] != "2" {
		common.SysError("bluewhalepay notify state 非 SUCCESS，忽略 mchOrderNo=" + params["mchOrderNo"] + " state=" + params["state"])
		c.String(http.StatusOK, "success")
		return
	}

	mchOrderNo := params["mchOrderNo"]
	if mchOrderNo == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	LockOrder(mchOrderNo)
	defer UnlockOrder(mchOrderNo)
	if err := model.RechargeBlueWhalePay(mchOrderNo, c.ClientIP()); err != nil {
		common.SysError("bluewhalepay notify 加额度失败 mchOrderNo=" + mchOrderNo + " err=" + err.Error())
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	common.SysLog("bluewhalepay notify 充值成功 mchOrderNo=" + mchOrderNo)
	c.String(http.StatusOK, "success")
}
