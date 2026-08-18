package material_library

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openMaterialRecordDB 为记录层测试准备隔离的归属表。
func openMaterialRecordDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserMaterialGroup{}))
	return db
}

func newRecordContext(t *testing.T, path, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if body == "" {
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	} else {
		c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	}
	c.Set("id", 42)
	c.Request = c.Request.WithContext(RecordContext(c))
	return c
}

// TestRecordContext_extractsOperationInfo 验证 RecordContext 提取归属信息，且请求体原样装回。
func TestRecordContext_extractsOperationInfo(t *testing.T) {
	t.Run("CreateAssetGroup 从 body 读组名与组类型", func(t *testing.T) {
		c := newRecordContext(t, "/api/material/CreateAssetGroup", `{"Name":"group-a","GroupType":"LivenessFace"}`)
		ctx := c.Request.Context()
		assert.Equal(t, 42, ctx.Value(ctxUserID))
		assert.Equal(t, "LivenessFace", ctx.Value(ctxGroupType))
		assert.Equal(t, "group-a", ctx.Value(ctxGroupName))

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"Name":"group-a","GroupType":"LivenessFace"}`, string(body))
	})

	t.Run("DeleteAssetGroup 从 body 读组 id", func(t *testing.T) {
		c := newRecordContext(t, "/api/material/DeleteAssetGroup", `{"Id":"group-del"}`)
		ctx := c.Request.Context()
		assert.Equal(t, "group-del", ctx.Value(ctxGroupID))

		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"Id":"group-del"}`, string(body))
	})

	t.Run("无关 Action 不透传记录信息", func(t *testing.T) {
		c := newRecordContext(t, "/api/material/ListAssetGroups", `{}`)
		ctx := c.Request.Context()
		assert.Nil(t, ctx.Value(ctxGroupType))
		assert.Nil(t, ctx.Value(ctxGroupID))
	})
}

// TestProxy_recordsGroupAssociation 端到端验证：CreateAssetGroup 成功后，归属表落库 Result.Id。
func TestProxy_recordsGroupAssociation(t *testing.T) {
	model.DB = openMaterialRecordDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"group-recorded"}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	c := newRecordContext(t, "/api/material/CreateAssetGroup", `{"Name":"group-a","GroupType":"AIGC"}`)
	// 直接传原始 recorder：gin 的 writer 会把 httptest.ResponseRecorder 包一层，
	// ReverseProxy 的 CloseNotify 断言会在测试里 panic（生产环境底层是真实连接，无此问题）。
	Proxy().ServeHTTP(httptest.NewRecorder(), c.Request)

	groups, err := model.GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "group-recorded", groups[0].SeedanceAssetGroupId)
	assert.Equal(t, "AIGC", groups[0].GroupType)
	assert.Equal(t, "group-a", groups[0].Name)
}

// TestProxy_removesGroupAssociationOnDelete 端到端验证：DeleteAssetGroup 成功后，归属表同步清理。
func TestProxy_removesGroupAssociationOnDelete(t *testing.T) {
	model.DB = openMaterialRecordDB(t)
	require.NoError(t, model.AddUserMaterialGroup(42, "group-del", "g", "AIGC"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{}}`))
	}))
	defer upstream.Close()
	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	c := newRecordContext(t, "/api/material/DeleteAssetGroup", `{"Id":"group-del"}`)
	Proxy().ServeHTTP(httptest.NewRecorder(), c.Request)

	groups, err := model.GetUserMaterialGroups(42)
	require.NoError(t, err)
	assert.Len(t, groups, 0)
}
