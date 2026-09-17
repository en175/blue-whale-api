package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseLogTypes(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected []int
	}{
		{name: "缺省为空串", raw: "", expected: nil},
		{name: "纯空白等同缺省", raw: "   ", expected: nil},
		{name: "0 为不过滤哨兵", raw: "0", expected: nil},
		{name: "带空白的 0", raw: " 0 ", expected: nil},
		{name: "单值", raw: "2", expected: []int{2}},
		{name: "多值取并集", raw: "2,6", expected: []int{2, 6}},
		{name: "逐项去两侧空白", raw: " 2 , 6 ", expected: []int{2, 6}},
		{name: "重复值不去重", raw: "2,2", expected: []int{2, 2}},
		{name: "全部类型", raw: "1,2,3,4,5,6,7", expected: []int{1, 2, 3, 4, 5, 6, 7}},
		{name: "越界值照旧下发", raw: "999", expected: []int{999}},
		{name: "负值照旧下发", raw: "-1", expected: []int{-1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogTypes(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestParseLogTypesRejectsSilentDegrade 保护 IKGCWR 的核心约束：这些输入若不被拒绝，
// 会落进「不过滤」分支静默返回全部类型，而调用方以为拿到的是过滤结果。
//
// 注意「分号分隔」一行只覆盖解析器契约。HTTP 层上 `?type=1;2` 到不了这里——Go 的
// url.ParseQuery 会把整对键值丢弃，gin 的 c.Query 吞掉该 error，于是它看起来像「没传」
// 而静默返回全部类型，与改动前一致。详见 docs/adr/0018 的「已知边界」。
func TestParseLogTypesRejectsSilentDegrade(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "非数字", raw: "abc"},
		{name: "尾随逗号", raw: "2,"},
		{name: "前导逗号", raw: ",2"},
		{name: "仅逗号", raw: ","},
		{name: "分号分隔", raw: "1;2"},
		{name: "空格分隔", raw: "2 6"},
		{name: "小数点", raw: "2.6"},
		{name: "0 在前与其他值混用", raw: "0,2"},
		{name: "0 在后与其他值混用", raw: "2,0"},
		{name: "0 重复出现", raw: "0,0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogTypes(tt.raw)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

type userLogsEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Total int         `json:"total"`
		Items []model.Log `json:"items"`
	} `json:"data"`
}

// openLogControllerTestDB 起一个内存 SQLite 作为 DB / LOG_DB，只迁移 logs 表。
func openLogControllerTestDB(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

// TestGetUserLogsHandlerTypeFilter 走一遍真实的 handler 装配：查询串 → parseLogTypes →
// 过滤 → PageInfo 信封。非法值必须整体失败，不能落到「不过滤」返回全部类型。
func TestGetUserLogsHandlerTypeFilter(t *testing.T) {
	openLogControllerTestDB(t)

	const owner = 9003
	seeds := []model.Log{
		{UserId: owner, Type: model.LogTypeConsume, CreatedAt: 100},
		{UserId: owner, Type: model.LogTypeRefund, CreatedAt: 200},
		{UserId: owner, Type: model.LogTypeTopup, CreatedAt: 300},
	}
	for i := range seeds {
		require.NoError(t, model.LOG_DB.Create(&seeds[i]).Error)
	}

	router := gin.New()
	router.GET("/api/log/self", func(c *gin.Context) {
		c.Set("id", owner)
		GetUserLogs(c)
	})

	tests := []struct {
		name        string
		query       string
		wantSuccess bool
		wantTotal   int
	}{
		{name: "多值取并集", query: "type=2,6&p=1&page_size=20", wantSuccess: true, wantTotal: 2},
		{name: "多值带空白", query: "type=2,%206&p=1&page_size=20", wantSuccess: true, wantTotal: 2},
		{name: "单值", query: "type=2&p=1&page_size=20", wantSuccess: true, wantTotal: 1},
		{name: "0 不过滤", query: "type=0&p=1&page_size=20", wantSuccess: true, wantTotal: 3},
		{name: "不传 type 不过滤", query: "p=1&page_size=20", wantSuccess: true, wantTotal: 3},
		{name: "非法值不得降级为全部", query: "type=abc&p=1&page_size=20", wantSuccess: false},
		{name: "0 与其他值混用非法", query: "type=0,2&p=1&page_size=20", wantSuccess: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/log/self?"+tt.query, nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			var envelope userLogsEnvelope
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &envelope))

			require.Equal(t, tt.wantSuccess, envelope.Success, "message=%s", envelope.Message)
			if !tt.wantSuccess {
				assert.NotEmpty(t, envelope.Message)
				return
			}
			assert.Equal(t, tt.wantTotal, envelope.Data.Total)
			assert.Len(t, envelope.Data.Items, tt.wantTotal)
		})
	}
}
