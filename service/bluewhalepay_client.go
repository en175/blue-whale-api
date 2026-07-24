package service

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

// BlueWhalePaySign 实现 bluewhalepay 商户签名（对照 pay BlueWhalePayKit.getSign）：
// 每个参数拼成 "k=v&"（value 非空）→ 按"整串"不区分大小写排序 → 末尾 "key=<appSecret>" → MD5(UTF-8) 大写。
// 下单签名（请求）+ 回调验签（响应）共用一份。
func BlueWhalePaySign(params map[string]string, appSecret string) string {
	pairs := make([]string, 0, len(params))
	for k, v := range params {
		if v == "" {
			continue
		}
		pairs = append(pairs, k+"="+v+"&")
	}
	// String.CASE_INSENSITIVE_ORDER 等价：整串 ToLowerCase 比较
	sort.Slice(pairs, func(i, j int) bool {
		return strings.ToLower(pairs[i]) < strings.ToLower(pairs[j])
	})
	var sb strings.Builder
	for _, p := range pairs {
		sb.WriteString(p)
	}
	sb.WriteString("key=" + appSecret)
	sum := md5.Sum([]byte(sb.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// bluewhalepayApiRes pay /api/pay/unifiedOrder 响应。
type bluewhalepayApiRes struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		PayOrderId  string `json:"payOrderId"`
		OrderState  int    `json:"orderState"`
		ErrCode     string `json:"errCode"`
		ErrMsg      string `json:"errMsg"`
		PayDataType string `json:"payDataType"`
		PayData     string `json:"payData"` // 支付链接 / 二维码内容
	} `json:"data"`
}

// BlueWhalePayRequestUnifiedOrder 调 pay /api/pay/unifiedOrder 下单（带 notifyUrl → HTTP 回调分支）。
// amount 传分：TopUp.Money（元）× 100。返回 payOrderId + payData（前端拉起支付）。
func BlueWhalePayRequestUnifiedOrder(topUp *model.TopUp, clientIp, wayCode string) (payOrderId, payData string, err error) {
	if strings.TrimSpace(setting.BlueWhalePayPayUrl) == "" {
		return "", "", fmt.Errorf("bluewhalepay pay 服务地址未配置")
	}
	if strings.TrimSpace(setting.BlueWhalePayAppSecret) == "" {
		return "", "", fmt.Errorf("bluewhalepay appSecret 未配置")
	}
	if wayCode == "" {
		wayCode = setting.BlueWhalePayWayCode
	}

	amountFen := int64(math.Round(topUp.Money * 100))
	notifyUrl := strings.TrimRight(setting.BlueWhalePayServerBaseUrl, "/") + "/api/bluewhalepay/notify"
	reqTime := time.Now().UnixMilli()
	subject := fmt.Sprintf("充值%.2f元", topUp.Money)

	// body 数字字段用数字类型（JSON）；签名时值统一转字符串（对照 Java getSign 的 value.toString()）
	body := map[string]interface{}{
		"mchNo":      setting.BlueWhalePayMchNo,
		"appId":      setting.BlueWhalePayAppId,
		"mchOrderNo": topUp.TradeNo,
		"wayCode":    wayCode,
		"amount":     amountFen,
		"currency":   "CNY",
		"subject":    subject,
		"body":       subject,
		"clientIp":   clientIp,
		"notifyUrl":  notifyUrl,
		"reqTime":    reqTime,
		"version":    "1.0", // AbstractRQ @NotBlank
		"signType":   "MD5", // AbstractRQ @NotBlank，参与签名
	}
	signParams := make(map[string]string, len(body))
	for k, v := range body {
		signParams[k] = fmt.Sprintf("%v", v)
	}
	body["sign"] = BlueWhalePaySign(signParams, setting.BlueWhalePayAppSecret)

	jsonBody, _ := json.Marshal(body)
	url := strings.TrimRight(setting.BlueWhalePayPayUrl, "/") + "/api/pay/unifiedOrder"
	common.SysLog(fmt.Sprintf("bluewhalepay 下单请求 trade_no=%s wayCode=%s amountFen=%d url=%s", topUp.TradeNo, wayCode, amountFen, url))

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("调用 pay 失败: %v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	common.SysLog(fmt.Sprintf("bluewhalepay 下单响应 trade_no=%s status=%d body=%s", topUp.TradeNo, resp.StatusCode, string(respBody)))
	if resp.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("pay http status %d", resp.StatusCode)
	}

	var apiRes bluewhalepayApiRes
	if err := json.Unmarshal(respBody, &apiRes); err != nil {
		return "", "", fmt.Errorf("解析 pay 响应失败: %v", err)
	}
	if apiRes.Code != 0 {
		return "", "", fmt.Errorf("pay 下单失败 code=%d msg=%s", apiRes.Code, apiRes.Msg)
	}
	return apiRes.Data.PayOrderId, apiRes.Data.PayData, nil
}
