package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 平铺键与分层键写的是同一份内存配置：model.handleConfigUpdate 把
// "<模块>.<字段>" 拆开直接灌进注册的配置结构体，不经过任何校验。
// 因此校验必须对两种拼写同时生效——只挂在平铺键上时，换个 key 名就能把
// null 送进去被 encoding/json 解成 0（该模型在该组下免费），或把负倍率
// 送进计费乘数。
func TestUpdateOptionRejectsInvalidGroupRatioUnderBothKeySpellings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/api/option", UpdateOption)

	cases := []struct {
		name  string
		key   string
		value string
	}{
		{"平铺键拒绝负数", "GroupRatio", `{"default":-1}`},
		{"分层键拒绝负数", "group_ratio_setting.group_ratio", `{"default":-1}`},
		{"平铺键拒绝 null 倍率", "GroupModelRatio", `{"vip":{"m":null}}`},
		{"分层键拒绝 null 倍率", "group_ratio_setting.group_model_ratio", `{"vip":{"m":null}}`},
		{"平铺键拒绝负数倍率", "GroupModelRatio", `{"vip":{"m":-5}}`},
		{"分层键拒绝负数倍率", "group_ratio_setting.group_model_ratio", `{"vip":{"m":-5}}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 校验在 model.UpdateOption（落库）之前短路返回，正常路径不碰数据库。
			// 万一某个拼写漏了校验，请求会继续走到 model.UpdateOption，而 controller
			// 测试进程没有数据库连接，nil DB 会 panic。接住它，把「校验没拦住」变成
			// 一条明确的失败信息，而不是让整个测试进程崩掉、连带吞掉后面的子用例。
			var recorder *httptest.ResponseRecorder
			var panicked any
			func() {
				defer func() { panicked = recover() }()
				recorder = performJSON(router, http.MethodPut, "/api/option",
					map[string]any{"key": tc.key, "value": tc.value}, "")
			}()
			require.Nil(t, panicked, "校验未拦截 %s，请求落到了 model.UpdateOption", tc.key)

			var resp struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
			require.False(t, resp.Success, "平铺键与分层键都必须被拒：%s", tc.key)
			require.NotEmpty(t, resp.Message)
		})
	}
}
