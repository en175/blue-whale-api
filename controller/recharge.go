package controller

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
	"gorm.io/gorm"
)

// ---------- 用户端 ----------

// GetRechargeTiers 返回当前有效的充值档位列表（无需鉴权）。
func GetRechargeTiers(c *gin.Context) {
	tiers := model.GetActiveRechargeTiers()
	items := make([]dto.RechargeTierItem, 0, len(tiers))
	for _, t := range tiers {
		items = append(items, rechargeTierToItem(t))
	}
	common.ApiSuccess(c, dto.GetRechargeTiersResponse{Tiers: items})
}

// CreateRechargeOrder 选定档位 + 支付方式，创建 TopUp 记录并返回支付二维码。
// 档位金额为 raw tokens，不走 QuotaPerUnit 的乘法——回调时直接加 raw tokens。
func CreateRechargeOrder(c *gin.Context) {
	var req dto.CreateRechargeOrderRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if req.TierId == "" {
		common.ApiErrorMsg(c, "请选择充值档位")
		return
	}
	if req.PaymentMethod != "wechat" && req.PaymentMethod != "alipay" {
		common.ApiErrorMsg(c, "支付方式仅支持 wechat / alipay")
		return
	}

	tier := model.GetRechargeTierById(req.TierId)
	if tier == nil {
		common.ApiErrorMsg(c, "档位不存在")
		return
	}
	if !tier.IsActive {
		common.ApiErrorMsg(c, "该档位已下架")
		return
	}
	now := common.GetTimestamp()
	if tier.StartTime != nil && *tier.StartTime > now {
		common.ApiErrorMsg(c, "该档位尚未生效")
		return
	}
	if tier.EndTime != nil && *tier.EndTime < now {
		common.ApiErrorMsg(c, "该档位已过期")
		return
	}

	userId := c.GetInt("id")

	// 档位积分 = raw tokens，不做 QuotaPerUnit 转换
	totalTokens := tier.Amount + tier.Bonus

	// wayCode：前端 wechat/alipay → WX_NATIVE(扫码) / ALI_QR
	wayCode := "WX_NATIVE"
	if req.PaymentMethod == "alipay" {
		wayCode = "ALI_QR"
	}

	tradeNo := fmt.Sprintf("RCH%d%d%s", userId, time.Now().UnixMilli(), randstr.String(4))
	topUp := &model.TopUp{
		UserId:          userId,
		Amount:          totalTokens, // raw tokens，回调直接加
		Money:           tier.Price,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderBlueWhalePay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("recharge 创建订单失败 user_id=%d tier=%s trade_no=%s error=%q", userId, req.TierId, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	// notifyUrl 指向 /api/recharge/notify（与余额充值回调区分）
	payOrderId, payData, err := service.BlueWhalePayUnifiedOrder(c.Request.Context(), topUp, c.ClientIP(), wayCode, "/api/recharge/notify")
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("recharge 下单失败 user_id=%d tier=%s trade_no=%s error=%q", userId, req.TierId, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("recharge 下单成功 user_id=%d tier=%s trade_no=%s pay_order_id=%s tokens=%d price=%.2f", userId, req.TierId, tradeNo, payOrderId, totalTokens, tier.Price))

	// 过期时间：15 分钟
	expireTime := common.GetTimestamp() + 15*60

	common.ApiSuccess(c, dto.CreateRechargeOrderResponse{
		TradeNo:    tradeNo,
		QrCodeData: payData,
		ExpireTime: expireTime,
		Amount:     tier.Price,
		Points:     totalTokens,
	})
}

// GetRechargeOrderStatus 轮询订单支付状态。
func GetRechargeOrderStatus(c *gin.Context) {
	tradeNo := c.Param("tradeNo")
	if tradeNo == "" {
		common.ApiErrorMsg(c, "缺少订单号")
		return
	}

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}

	userId := c.GetInt("id")
	if topUp.UserId != userId {
		common.ApiErrorMsg(c, "无权查看该订单")
		return
	}

	resp := dto.GetOrderStatusResponse{
		TradeNo: tradeNo,
		Status:  topUp.Status,
	}

	if topUp.Status == common.TopUpStatusSuccess {
		user, err := model.GetUserById(userId, false)
		if err == nil {
			q := int64(user.Quota)
			resp.CreditsAfter = &q
		}
	}

	common.ApiSuccess(c, resp)
}

// CancelRechargeOrder 取消未支付订单。
func CancelRechargeOrder(c *gin.Context) {
	tradeNo := c.Param("tradeNo")
	if tradeNo == "" {
		common.ApiErrorMsg(c, "缺少订单号")
		return
	}

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}

	userId := c.GetInt("id")
	if topUp.UserId != userId {
		common.ApiErrorMsg(c, "无权操作该订单")
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		common.ApiErrorMsg(c, "订单状态不允许取消")
		return
	}

	if err := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderBlueWhalePay, common.TopUpStatusExpired); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("recharge 取消订单失败 user_id=%d trade_no=%s error=%q", userId, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "取消订单失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("recharge 取消订单成功 user_id=%d trade_no=%s", userId, tradeNo))
	common.ApiSuccess(c, nil)
}

// GetRechargeAgreement 返回充值协议（无需鉴权）。
func GetRechargeAgreement(c *gin.Context) {
	common.ApiSuccess(c, dto.GetRechargeAgreementResponse{
		Title:   "充值协议",
		Content: "## 充值协议\n\n充值即视为同意本协议。\n\n- 充值金额不支持退款\n- 暂不支持开具发票\n- 如有疑问请联系客服",
	})
}

// ---------- 支付回调 ----------

// RechargeBlueWhalePayNotify 档位充值回调（notifyUrl = /api/recharge/notify）。
// raw tokens 直接加，不走 QuotaPerUnit 乘法。
func RechargeBlueWhalePayNotify(c *gin.Context) {
	if !isBlueWhalePayEnabled() {
		common.SysError("recharge notify 被拒绝 reason=disabled path=" + c.Request.RequestURI)
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	if strings.TrimSpace(setting.BlueWhalePayAppSecret) == "" {
		common.SysError("recharge notify 被拒绝 reason=appSecret 未配置")
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
		common.SysError("recharge notify 验签失败 mchOrderNo=" + params["mchOrderNo"])
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 仅支付成功(state=2)才加额度
	if params["state"] != "2" {
		common.SysError("recharge notify state 非 SUCCESS，忽略 mchOrderNo=" + params["mchOrderNo"] + " state=" + params["state"])
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

	var (
		topupUserId      int
		topupMoney       float64
		topupPayment     string
		topupTokensAdded int64
	)

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		topUp := &model.TopUp{}
		if err := model.LockForUpdate(tx).Where("trade_no = ?", mchOrderNo).First(topUp).Error; err != nil {
			return fmt.Errorf("充值订单不存在")
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等
		}
		if topUp.Status != common.TopUpStatusPending {
			return fmt.Errorf("充值订单状态错误")
		}

		// raw tokens 直接加，不走 QuotaPerUnit
		tokensToAdd := topUp.Amount
		if tokensToAdd <= 0 {
			return fmt.Errorf("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", tokensToAdd)).Error; err != nil {
			return err
		}

		// 捕获值，事务成功后写日志
		topupUserId = topUp.UserId
		topupMoney = topUp.Money
		topupPayment = topUp.PaymentMethod
		topupTokensAdded = tokensToAdd

		return nil
	}); err != nil {
		common.SysError("recharge notify 加额度失败 mchOrderNo=" + mchOrderNo + " err=" + err.Error())
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// 记录充值日志
	model.RecordTopupLog(topupUserId,
		fmt.Sprintf("档位充值回调成功，充值额度：%v，支付金额：%.2f", logger.FormatQuota(int(topupTokensAdded)), topupMoney),
		c.ClientIP(), topupPayment, model.PaymentMethodBlueWhalePay)

	common.SysLog("recharge notify 充值成功 mchOrderNo=" + mchOrderNo)
	c.String(http.StatusOK, "success")
}

// ---------- 管理端 ----------

// AdminGetAllTiers 返回全部档位（含已下架/过期）。
func AdminGetAllTiers(c *gin.Context) {
	tiers := model.GetAllRechargeTiers()
	items := make([]dto.AdminTierItem, 0, len(tiers))
	for _, t := range tiers {
		items = append(items, rechargeTierToAdminItem(t))
	}
	common.ApiSuccess(c, items)
}

// AdminCreateTier 创建新档位。
func AdminCreateTier(c *gin.Context) {
	var req dto.CreateTierRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if req.Id == "" || req.Amount <= 0 || req.Price <= 0 {
		common.ApiErrorMsg(c, "id/amount/price 为必填且必须大于0")
		return
	}
	if model.GetRechargeTierById(req.Id) != nil {
		common.ApiErrorMsg(c, "档位 ID 已存在")
		return
	}

	tier := &model.RechargeTier{
		Id:            req.Id,
		Name:          req.Name,
		Amount:        req.Amount,
		Price:         req.Price,
		OriginalPrice: req.OriginalPrice,
		Bonus:         req.Bonus,
		IsRecommended: req.IsRecommended,
		IsActive:      req.IsActive,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		SortOrder:     req.SortOrder,
	}
	if err := model.InsertRechargeTier(tier); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("recharge admin 创建档位失败 id=%s error=%q", req.Id, err.Error()))
		common.ApiErrorMsg(c, "创建档位失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("recharge admin 创建档位成功 id=%s name=%s amount=%d price=%.2f", req.Id, req.Name, req.Amount, req.Price))
	common.ApiSuccess(c, rechargeTierToAdminItem(tier))
}

// AdminUpdateTier 编辑档位（部分更新）。
func AdminUpdateTier(c *gin.Context) {
	id := c.Param("id")
	tier := model.GetRechargeTierById(id)
	if tier == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "data": "档位不存在"})
		return
	}

	var req dto.UpdateTierRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	if req.Name != nil {
		tier.Name = *req.Name
	}
	if req.Amount != nil {
		tier.Amount = *req.Amount
	}
	if req.Price != nil {
		tier.Price = *req.Price
	}
	if req.OriginalPrice != nil {
		tier.OriginalPrice = req.OriginalPrice
	}
	if req.Bonus != nil {
		tier.Bonus = *req.Bonus
	}
	if req.IsRecommended != nil {
		tier.IsRecommended = *req.IsRecommended
	}
	if req.IsActive != nil {
		tier.IsActive = *req.IsActive
	}
	if req.StartTime != nil {
		tier.StartTime = req.StartTime
	}
	if req.EndTime != nil {
		tier.EndTime = req.EndTime
	}
	if req.SortOrder != nil {
		tier.SortOrder = *req.SortOrder
	}

	if err := model.UpdateRechargeTier(tier); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("recharge admin 更新档位失败 id=%s error=%q", id, err.Error()))
		common.ApiErrorMsg(c, "更新档位失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("recharge admin 更新档位成功 id=%s", id))
	common.ApiSuccess(c, rechargeTierToAdminItem(tier))
}

// AdminDeleteTier 删除档位。
func AdminDeleteTier(c *gin.Context) {
	id := c.Param("id")
	if model.GetRechargeTierById(id) == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "data": "档位不存在"})
		return
	}
	if err := model.DeleteRechargeTierById(id); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("recharge admin 删除档位失败 id=%s error=%q", id, err.Error()))
		common.ApiErrorMsg(c, "删除档位失败")
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("recharge admin 删除档位成功 id=%s", id))
	common.ApiSuccess(c, nil)
}

// ---------- helpers ----------

func rechargeTierToItem(t *model.RechargeTier) dto.RechargeTierItem {
	return dto.RechargeTierItem{
		Id:            t.Id,
		Name:          t.Name,
		Amount:        t.Amount,
		Price:         t.Price,
		OriginalPrice: t.OriginalPrice,
		Bonus:         t.Bonus,
		IsRecommended: t.IsRecommended,
		SortOrder:     t.SortOrder,
	}
}

func rechargeTierToAdminItem(t *model.RechargeTier) dto.AdminTierItem {
	return dto.AdminTierItem{
		Id:            t.Id,
		Name:          t.Name,
		Amount:        t.Amount,
		Price:         t.Price,
		OriginalPrice: t.OriginalPrice,
		Bonus:         t.Bonus,
		IsRecommended: t.IsRecommended,
		IsActive:      t.IsActive,
		StartTime:     t.StartTime,
		EndTime:       t.EndTime,
		SortOrder:     t.SortOrder,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}
