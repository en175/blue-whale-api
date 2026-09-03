package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openMaterialConsoleDB 为管理端点测试准备隔离的归属表。
func openMaterialConsoleDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitCol()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.UserMaterialGroup{}))
	return db
}

func performGetMaterialGroups(t *testing.T, role, userID int, groupType string) *httptest.ResponseRecorder {
	t.Helper()
	params := map[string]string{}
	if groupType != "" {
		params["type"] = groupType
	}
	return performGetMaterialGroupsParams(t, role, userID, params)
}

// performGetMaterialGroupsParams 带完整查询参数（p/page_size/type/name）的版本，空值键跳过。
func performGetMaterialGroupsParams(t *testing.T, role, userID int, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	// 模拟 UserAuth 注入的 role/id，再进入真实 handler。
	r.GET("/materials/groups", func(c *gin.Context) {
		c.Set("role", role)
		c.Set("id", userID)
		GetMaterialGroups(c)
	})

	urlPath := "/materials/groups"
	sep := "?"
	for k, v := range params {
		if v == "" {
			continue
		}
		urlPath += sep + url.QueryEscape(k) + "=" + url.QueryEscape(v)
		sep = "&"
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, urlPath, nil))
	return w
}

// TestGetMaterialGroups_isolation 验证数据隔离：普通用户只见自己，admin 看全部，类型可过滤。
func TestGetMaterialGroups_isolation(t *testing.T) {
	openMaterialConsoleDB(t)
	require.NoError(t, model.AddUserMaterialGroup(1, "tokenhub", "g_admin", "admin-aigc", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(2, "tokenhub", "g_user", "user-aigc", "AIGC"))
	require.NoError(t, model.AddUserMaterialGroup(2, "tokenhub", "g_real", "user-real", "LivenessFace"))
	t.Cleanup(func() {
		_ = model.DeleteUserMaterialGroup("tokenhub", "g_admin")
		_ = model.DeleteUserMaterialGroup("tokenhub", "g_user")
		_ = model.DeleteUserMaterialGroup("tokenhub", "g_real")
	})

	t.Run("普通用户只见自己的组", func(t *testing.T) {
		w := performGetMaterialGroups(t, common.RoleCommonUser, 2, "")
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Data struct {
				Items []model.UserMaterialGroup `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Data.Items, 2)
		for _, g := range resp.Data.Items {
			assert.Equal(t, 2, g.UserId)
		}
	})

	t.Run("admin 看全部", func(t *testing.T) {
		w := performGetMaterialGroups(t, common.RoleAdminUser, 1, "")
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Data struct {
				Items []model.UserMaterialGroup `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Data.Items, 3)
	})

	t.Run("按类型过滤", func(t *testing.T) {
		w := performGetMaterialGroups(t, common.RoleAdminUser, 1, "LivenessFace")
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Data struct {
				Items []model.UserMaterialGroup `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Data.Items, 1)
		assert.Equal(t, "LivenessFace", resp.Data.Items[0].GroupType)
	})
}

// TestGetMaterialGroups_pagination 覆盖分页返回与参数解析：
// total_count/page_number/page_size 字段、默认值、page_size/page 上限钳制、name 过滤、越界页空 items。
func TestGetMaterialGroups_pagination(t *testing.T) {
	openMaterialConsoleDB(t)
	for i := 1; i <= 5; i++ {
		require.NoError(t, model.AddUserMaterialGroup(2, "tokenhub", fmt.Sprintf("pg_g%d", i), fmt.Sprintf("pg-name-%d", i), "AIGC"))
	}
	t.Cleanup(func() {
		for i := 1; i <= 5; i++ {
			_ = model.DeleteUserMaterialGroup("tokenhub", fmt.Sprintf("pg_g%d", i))
		}
	})

	type respShape struct {
		Data struct {
			TotalCount int                       `json:"total_count"`
			PageNumber int                       `json:"page_number"`
			PageSize   int                       `json:"page_size"`
			Items      []model.UserMaterialGroup `json:"items"`
		} `json:"data"`
	}
	parse := func(w *httptest.ResponseRecorder) respShape {
		t.Helper()
		require.Equal(t, http.StatusOK, w.Code)
		var resp respShape
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	t.Run("默认分页", func(t *testing.T) {
		resp := parse(performGetMaterialGroupsParams(t, common.RoleCommonUser, 2, nil))
		assert.EqualValues(t, 5, resp.Data.TotalCount)
		assert.EqualValues(t, 1, resp.Data.PageNumber)
		assert.EqualValues(t, 20, resp.Data.PageSize)
		assert.Len(t, resp.Data.Items, 5)
	})

	t.Run("翻页", func(t *testing.T) {
		resp := parse(performGetMaterialGroupsParams(t, common.RoleCommonUser, 2, map[string]string{"p": "2", "page_size": "2"}))
		assert.EqualValues(t, 5, resp.Data.TotalCount)
		assert.EqualValues(t, 2, resp.Data.PageNumber)
		assert.EqualValues(t, 2, resp.Data.PageSize)
		assert.Len(t, resp.Data.Items, 2)
	})

	t.Run("page_size 超上限钳制到 100", func(t *testing.T) {
		resp := parse(performGetMaterialGroupsParams(t, common.RoleCommonUser, 2, map[string]string{"page_size": "500"}))
		assert.EqualValues(t, 100, resp.Data.PageSize)
		assert.Len(t, resp.Data.Items, 5)
	})

	t.Run("page 超上限钳制到 100000", func(t *testing.T) {
		resp := parse(performGetMaterialGroupsParams(t, common.RoleCommonUser, 2, map[string]string{"p": "999999", "page_size": "2"}))
		assert.EqualValues(t, 100000, resp.Data.PageNumber)
	})

	t.Run("page 越界返回空 items 不报错", func(t *testing.T) {
		resp := parse(performGetMaterialGroupsParams(t, common.RoleCommonUser, 2, map[string]string{"p": "99", "page_size": "2"}))
		assert.EqualValues(t, 5, resp.Data.TotalCount)
		assert.Len(t, resp.Data.Items, 0)
	})

	t.Run("name 过滤", func(t *testing.T) {
		resp := parse(performGetMaterialGroupsParams(t, common.RoleCommonUser, 2, map[string]string{"name": "pg-name-3"}))
		assert.EqualValues(t, 1, resp.Data.TotalCount)
		require.Len(t, resp.Data.Items, 1)
		assert.Equal(t, "pg_g3", resp.Data.Items[0].SeedanceAssetGroupId)
	})
}

// TestListMaterialGroupAssets_callsUpstream 验证管理端点按组 id 直连上游并返回素材（admin 放行）。
func TestListMaterialGroupAssets_callsUpstream(t *testing.T) {
	openMaterialConsoleDB(t)
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "ListAssets", r.URL.Query().Get("Action"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"TotalCount":1,"Items":[{"Id":"asset-9","Name":"n","Status":"Active"}],"PageNumber":1,"PageSize":10}}`))
	}))
	defer upstream.Close()

	setMaterialConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	r := gin.New()
	r.GET("/materials/groups/:id/assets", func(c *gin.Context) {
		c.Set("role", common.RoleAdminUser)
		c.Set("id", 1)
		ListMaterialGroupAssets(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/materials/groups/group-1/assets?p=1&page_size=10", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			TotalCount int `json:"TotalCount"`
			Items      []struct {
				Id string `json:"Id"`
			} `json:"Items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Data.TotalCount)
	assert.Len(t, resp.Data.Items, 1)
	assert.Equal(t, "asset-9", resp.Data.Items[0].Id)
}

// TestMaterialConsole_rejectsUnownedGroup 验证数据隔离：非 admin 用户操作无归属的组被拒。
func TestMaterialConsole_rejectsUnownedGroup(t *testing.T) {
	openMaterialConsoleDB(t)
	require.NoError(t, model.AddUserMaterialGroup(1, "tokenhub", "g_owned", "mine", "AIGC"))
	t.Cleanup(func() { _ = model.DeleteUserMaterialGroup("tokenhub", "g_owned") })

	r := gin.New()
	// 普通用户（role=1）id=2，只拥有 g_owned 由 user 1 持有 → 操作 g_owned 应被拒
	r.DELETE("/materials/groups/:id", func(c *gin.Context) {
		c.Set("role", common.RoleCommonUser)
		c.Set("id", 2)
		DeleteMaterialGroup(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/materials/groups/g_owned", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "material group not found or not owned")
}

// TestMaterialConsole_rejectsOverlongGroupName 验证管理端建组名超过 255 rune 被拒
// （与代理侧一致的组名校验：≤255 rune、不含 <>、管理端额外要求非空）。
func TestMaterialConsole_rejectsOverlongGroupName(t *testing.T) {
	openMaterialConsoleDB(t)

	r := gin.New()
	r.POST("/materials/groups", func(c *gin.Context) {
		c.Set("role", common.RoleAdminUser)
		c.Set("id", 1)
		CreateMaterialGroup(c)
	})

	longName := strings.Repeat("中", 256)
	body := fmt.Sprintf(`{"name":%q,"group_type":"AIGC"}`, longName)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/materials/groups", strings.NewReader(body)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "name is too long")
}
