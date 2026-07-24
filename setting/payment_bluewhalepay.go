package setting

// blue-whale-pay 支付渠道配置（独立网关）。
// BlueWhalePayAppSecret = 商户应用私钥（t_mch_app.app_secret），下单签名 + 回调验签共用。
// 付款方式（支付宝/微信）由通用 PayMethods 管理（type=bluewhalepay_alipay/wxpay）。
var BlueWhalePayEnabled = false
var BlueWhalePayPayUrl = ""         // pay 服务地址，如 http://localhost:8083
var BlueWhalePayMchNo = "BLUEWHALE" // 单运营主体固定商户号
var BlueWhalePayAppId = "BLUEWHALE_APP"
var BlueWhalePayAppSecret = ""      // 商户应用私钥（签名/验签）
var BlueWhalePayServerBaseUrl = ""  // blue_whale_api 公网基址，拼 /api/bluewhalepay/notify 作 notifyUrl 回调入口
var BlueWhalePayWayCode = "ALI_QR"  // 默认支付方式（fallback）
