package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserMaterialGroupCRUD 覆盖归属表的 upsert / 按用户查询 / 删除。
func TestUserMaterialGroupCRUD(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))

	// 新增
	require.NoError(t, AddUserMaterialGroup(42, "ag_1", "group-a", "AIGC"))

	groups, err := GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "ag_1", groups[0].SeedanceAssetGroupId)

	// upsert 同组更新元数据，不产生重复行
	require.NoError(t, AddUserMaterialGroup(42, "ag_1", "group-a-renamed", "AIGC"))
	groups, err = GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "group-a-renamed", groups[0].Name)

	// 用户间隔离：其他用户查不到
	other, err := GetUserMaterialGroups(7)
	require.NoError(t, err)
	assert.Len(t, other, 0)

	// 删除
	require.NoError(t, DeleteUserMaterialGroup("ag_1"))
	groups, err = GetUserMaterialGroups(42)
	require.NoError(t, err)
	assert.Len(t, groups, 0)
}

// TestAddUserMaterialGroup_keepsOwnership 验证归属不可转移：同组 id 被另一用户重复创建时，
// 不覆盖既有归属（UserId 保持不变）。
func TestAddUserMaterialGroup_keepsOwnership(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))
	require.NoError(t, AddUserMaterialGroup(1, "g_owner", "g", "AIGC"))

	// 用户 2 重复创建同一组 id → 归属仍归用户 1，仅元数据更新
	require.NoError(t, AddUserMaterialGroup(2, "g_owner", "renamed", "AIGC"))

	groups, err := GetMaterialGroups(0, "")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, 1, groups[0].UserId)
	assert.Equal(t, "renamed", groups[0].Name)

	require.NoError(t, DeleteUserMaterialGroup("g_owner"))
}

// TestGetMaterialGroups_queryFilters 覆盖管理界面的隔离与类型过滤：
// userId<=0 查全部（admin），否则按用户隔离；groupType 可选过滤。
func TestGetMaterialGroups_queryFilters(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))
	require.NoError(t, AddUserMaterialGroup(1, "g_admin_1", "admin-aigc", "AIGC"))
	require.NoError(t, AddUserMaterialGroup(2, "g_user_1", "user-aigc", "AIGC"))
	require.NoError(t, AddUserMaterialGroup(2, "g_user_2", "user-real", "LivenessFace"))

	t.Run("按用户隔离", func(t *testing.T) {
		groups, err := GetMaterialGroups(2, "")
		require.NoError(t, err)
		require.Len(t, groups, 2)
	})

	t.Run("admin 查全部（userId=0）", func(t *testing.T) {
		groups, err := GetMaterialGroups(0, "")
		require.NoError(t, err)
		require.Len(t, groups, 3)
	})

	t.Run("按类型过滤", func(t *testing.T) {
		groups, err := GetMaterialGroups(2, "LivenessFace")
		require.NoError(t, err)
		require.Len(t, groups, 1)
		assert.Equal(t, "g_user_2", groups[0].SeedanceAssetGroupId)
	})

	require.NoError(t, DeleteUserMaterialGroup("g_admin_1"))
	require.NoError(t, DeleteUserMaterialGroup("g_user_1"))
	require.NoError(t, DeleteUserMaterialGroup("g_user_2"))
}
