package material_library

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// ListAssets 走 service.GetHttpClient() 直连上游，测试前需初始化。
	service.InitHttpClient()
	os.Exit(m.Run())
}

// TestListAssets_parsesResult 验证 ListAssets 直连上游并解析 Result。
func TestListAssets_parsesResult(t *testing.T) {
	var gotAction, gotAuth, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAction = r.URL.Query().Get("Action")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"TotalCount":1,"Items":[{"Id":"asset-1","Name":"a","URL":"https://x","GroupId":"group-1","AssetType":"Image","Status":"Active","CreateTime":"2026-01-01T00:00:00Z","UpdateTime":"2026-01-01T00:00:00Z"}],"PageNumber":1,"PageSize":10}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	result, err := ListAssets(context.Background(), []string{"group-1"}, 1, 10, "")
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "asset-1", result.Items[0].Id)
	assert.Equal(t, "Active", result.Items[0].Status)

	// 直连构造与代理一致：/ark?Action=ListAssets&Version=...
	assert.Equal(t, "/ark", gotPath)
	assert.Equal(t, "ListAssets", gotAction)
	assert.Equal(t, "Bearer pak_x:psk_y", gotAuth)
}

// TestListAssets_returnsArkError 验证上游非 2xx 返回解析后的 ArkError。
func TestListAssets_returnsArkError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"no permission"}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	_, err := ListAssets(context.Background(), []string{"group-1"}, 1, 10, "")
	require.Error(t, err)
	var arkErr *ArkError
	require.ErrorAs(t, err, &arkErr)
	assert.Equal(t, "Forbidden", arkErr.Code)
	assert.Equal(t, "no permission", arkErr.Message)
}

// TestListAssets_emptyResult 验证空素材列表解析正常。
func TestListAssets_emptyResult(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"TotalCount":0,"Items":[],"PageNumber":1,"PageSize":10}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	result, err := ListAssets(context.Background(), []string{"group-1"}, 1, 10, "")
	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.Equal(t, 0, result.TotalCount)
}

// TestListAssets_forwardsNameFilter 验证名称搜索透传给上游 Filter.Name。
func TestListAssets_forwardsNameFilter(t *testing.T) {
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{"TotalCount":0,"Items":[],"PageNumber":1,"PageSize":10}}`))
	}))
	defer upstream.Close()

	setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

	_, err := ListAssets(context.Background(), []string{"group-1"}, 1, 10, "search-term")
	require.NoError(t, err)
	assert.JSONEq(t, `{"Filter":{"GroupIds":["group-1"],"Name":"search-term"},"PageNumber":1,"PageSize":10}`, gotBody)
}

// TestArkWriteActions 覆盖建组/删组/建素材/改素材/删素材的直连与 Result.Id 解析。
func TestArkWriteActions(t *testing.T) {
	tests := []struct {
		name   string
		action string
		body   string
		resp   string
		wantID string
		call   func(ctx context.Context) (string, error)
	}{
		{
			name: "CreateAssetGroup", action: "CreateAssetGroup", wantID: "group-1",
			body: `{"Name":"g","Description":"d","GroupType":"AIGC"}`,
			resp: `{"ResponseMetadata":{},"Result":{"Id":"group-1"}}`,
			call: func(ctx context.Context) (string, error) {
				return CreateAssetGroup(ctx, "g", "d", "AIGC")
			},
		},
		{
			name: "CreateAsset", action: "CreateAsset", wantID: "asset-1",
			body: `{"GroupId":"group-1","URL":"https://x","Name":"a","AssetType":"Image"}`,
			resp: `{"ResponseMetadata":{},"Result":{"Id":"asset-1"}}`,
			call: func(ctx context.Context) (string, error) {
				return CreateAsset(ctx, "group-1", "https://x", "a", "Image")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.action, r.URL.Query().Get("Action"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.resp))
			}))
			defer upstream.Close()
			setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

			id, err := tt.call(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
		})
	}

	t.Run("DeleteAssetGroup/UpdateAsset/DeleteAsset", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ResponseMetadata":{},"Result":{}}`))
		}))
		defer upstream.Close()
		setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

		require.NoError(t, DeleteAssetGroup(context.Background(), "group-1"))
		require.NoError(t, UpdateAsset(context.Background(), "asset-1", "new-name"))
		require.NoError(t, DeleteAsset(context.Background(), "asset-1"))
	})

	t.Run("写操作上游错误透传 ArkError", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"ResponseMetadata":{},"ResponseError":{"Code":"Forbidden","Message":"no"}}`))
		}))
		defer upstream.Close()
		setConfig(t, fmt.Sprintf(`{"enabled":true,"ak":"pak_x","sk":"psk_y","base_url":%q}`, upstream.URL))

		_, err := CreateAssetGroup(context.Background(), "g", "", "AIGC")
		require.Error(t, err)
		var arkErr *ArkError
		require.ErrorAs(t, err, &arkErr)
		assert.Equal(t, "Forbidden", arkErr.Code)
	})
}
