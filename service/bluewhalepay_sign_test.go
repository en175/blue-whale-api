//go:build manual

// 集成测试：需本地 pay（:8083）运行。go test -tags manual -run TestBlueWhalePaySignAlignment -v ./service/

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestBlueWhalePaySignAlignment 验证 Go BlueWhalePaySign 与 pay BlueWhalePayKit.getSign 一致：
// 用固定参数 + appSecret 算 sign，POST pay /api/pay/unifiedOrder，pay 验签通过（code=0）即对齐。
// 同时打印 signStr/sign 供对照 pay 日志（BlueWhalePayKit:82-84 打印）。
func TestBlueWhalePaySignAlignment(t *testing.T) {
	appSecret := "BLUEWHALE_APP_SECRET_2026"
	reqTime := time.Now().UnixMilli()
	mchOrderNo := fmt.Sprintf("SIGNTEST%d", reqTime)

	params := map[string]string{
		"mchNo":      "BLUEWHALE",
		"appId":      "BLUEWHALE_APP",
		"mchOrderNo": mchOrderNo,
		"wayCode":    "ALI_WAP",
		"amount":     "100",
		"currency":   "CNY",
		"subject":    "签名测试",
		"body":       "签名测试",
		"clientIp":   "127.0.0.1",
		"notifyUrl":  "http://localhost:3000/api/bluewhalepay/notify",
		"reqTime":    fmt.Sprintf("%d", reqTime),
		"version":    "1.0",
		"signType":   "MD5",
	}

	// 打印 signStr（对照 pay BlueWhalePayKit:82 signStr）
	pairs := make([]string, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, k+"="+v+"&")
	}
	sort.Slice(pairs, func(i, j int) bool { return strings.ToLower(pairs[i]) < strings.ToLower(pairs[j]) })
	signStr := strings.Join(pairs, "") + "key=" + appSecret
	sign := BlueWhalePaySign(params, appSecret)
	t.Logf("signStr=%s", signStr)
	t.Logf("sign=%s", sign)

	// 构造 JSON body（数字字段用数字类型）POST pay 验证签名
	body := map[string]interface{}{
		"mchNo": "BLUEWHALE", "appId": "BLUEWHALE_APP",
		"mchOrderNo": mchOrderNo, "wayCode": "ALI_WAP",
		"amount": int64(100), "currency": "CNY",
		"subject": "签名测试", "body": "签名测试",
		"clientIp":  "127.0.0.1",
		"notifyUrl": "http://localhost:3000/api/bluewhalepay/notify",
		"reqTime":   reqTime, "version": "1.0", "signType": "MD5", "sign": sign,
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post("http://localhost:8083/api/pay/unifiedOrder", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("POST pay 失败: %v", err)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	t.Logf("pay 响应 status=%d body=%s", resp.StatusCode, string(rb))

	// 解析 code
	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(rb, &res)
	if res.Code != 0 {
		t.Fatalf("签名未通过：pay code=%d msg=%s（看 pay 日志 BlueWhalePayKit signStr/sign 对照）", res.Code, res.Msg)
	}
	t.Logf("✅ 签名对齐：pay 验签通过 code=0")
}
