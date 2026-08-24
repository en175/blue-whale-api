package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

// TestUserMaterialGroupCRUD 覆盖归属表的 upsert / 按用户查询 / 删除。
func TestUserMaterialGroupCRUD(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))

	// 新增
	require.NoError(t, AddUserMaterialGroup(42, "tokenhub", "ag_1", "group-a", "AIGC"))

	groups, err := GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "ag_1", groups[0].SeedanceAssetGroupId)
	assert.Equal(t, "tokenhub", groups[0].Provider)

	// upsert 同组更新元数据，不产生重复行
	require.NoError(t, AddUserMaterialGroup(42, "tokenhub", "ag_1", "group-a-renamed", "AIGC"))
	groups, err = GetUserMaterialGroups(42)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "group-a-renamed", groups[0].Name)

	// 用户间隔离：其他用户查不到
	other, err := GetUserMaterialGroups(7)
	require.NoError(t, err)
	assert.Len(t, other, 0)

	// 删除
	require.NoError(t, DeleteUserMaterialGroup("tokenhub", "ag_1"))
	groups, err = GetUserMaterialGroups(42)
	require.NoError(t, err)
	assert.Len(t, groups, 0)
}

// TestAddUserMaterialGroup_keepsOwnership 验证归属不可转移：同组 id 被另一用户重复创建时，
// 不覆盖既有归属（UserId 保持不变）。
func TestAddUserMaterialGroup_keepsOwnership(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))
	require.NoError(t, AddUserMaterialGroup(1, "tokenhub", "g_owner", "g", "AIGC"))

	// 用户 2 重复创建同一组 id → 归属仍归用户 1，仅元数据更新
	require.NoError(t, AddUserMaterialGroup(2, "tokenhub", "g_owner", "renamed", "AIGC"))

	groups, err := GetMaterialGroups(0, "", "")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, 1, groups[0].UserId)
	assert.Equal(t, "renamed", groups[0].Name)

	require.NoError(t, DeleteUserMaterialGroup("tokenhub", "g_owner"))
}

// TestGetMaterialGroups_queryFilters 覆盖管理界面的隔离与类型过滤：
// userId<=0 查全部（admin），否则按用户隔离；provider/groupType 可选过滤。
func TestGetMaterialGroups_queryFilters(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))
	require.NoError(t, AddUserMaterialGroup(1, "tokenhub", "g_admin_1", "admin-aigc", "AIGC"))
	require.NoError(t, AddUserMaterialGroup(2, "tokenhub", "g_user_1", "user-aigc", "AIGC"))
	require.NoError(t, AddUserMaterialGroup(2, "tokenhub", "g_user_2", "user-real", "LivenessFace"))

	t.Run("按用户隔离", func(t *testing.T) {
		groups, err := GetMaterialGroups(2, "", "")
		require.NoError(t, err)
		require.Len(t, groups, 2)
	})

	t.Run("admin 查全部（userId=0）", func(t *testing.T) {
		groups, err := GetMaterialGroups(0, "", "")
		require.NoError(t, err)
		require.Len(t, groups, 3)
	})

	t.Run("按 provider 过滤", func(t *testing.T) {
		groups, err := GetMaterialGroups(0, "tokenhub", "")
		require.NoError(t, err)
		require.Len(t, groups, 3)
		groups, err = GetMaterialGroups(0, "volcengine", "")
		require.NoError(t, err)
		require.Len(t, groups, 0)
	})

	t.Run("按类型过滤", func(t *testing.T) {
		groups, err := GetMaterialGroups(2, "", "LivenessFace")
		require.NoError(t, err)
		require.Len(t, groups, 1)
		assert.Equal(t, "g_user_2", groups[0].SeedanceAssetGroupId)
	})

	require.NoError(t, DeleteUserMaterialGroup("tokenhub", "g_admin_1"))
	require.NoError(t, DeleteUserMaterialGroup("tokenhub", "g_user_1"))
	require.NoError(t, DeleteUserMaterialGroup("tokenhub", "g_user_2"))
}

// TestGetMaterialGroupsPaged 覆盖分页：总条数、每页条数、id 降序、跨页不重不漏、越界页为空，
// 以及 name 过滤（组名/组 id 均可匹配、LIKE 通配符转义）。
func TestGetMaterialGroupsPaged(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialGroup{}))
	for i := 1; i <= 5; i++ {
		require.NoError(t, AddUserMaterialGroup(1, "tokenhub", fmt.Sprintf("page_g%d", i), fmt.Sprintf("page-name-%d", i), "AIGC"))
	}
	t.Cleanup(func() {
		for i := 1; i <= 5; i++ {
			_ = DeleteUserMaterialGroup("tokenhub", fmt.Sprintf("page_g%d", i))
		}
	})

	t.Run("分页不重不漏且按 id 降序", func(t *testing.T) {
		total, p1, err := GetMaterialGroupsPaged(1, "tokenhub", "", "", 1, 2)
		require.NoError(t, err)
		assert.EqualValues(t, 5, total)
		require.Len(t, p1, 2)
		assert.Greater(t, p1[0].Id, p1[1].Id) // id DESC

		_, p2, err := GetMaterialGroupsPaged(1, "tokenhub", "", "", 2, 2)
		require.NoError(t, err)
		require.Len(t, p2, 2)

		_, p3, err := GetMaterialGroupsPaged(1, "tokenhub", "", "", 3, 2)
		require.NoError(t, err)
		require.Len(t, p3, 1) // 最后一页不满

		// 越界页返回空但不报错
		_, p4, err := GetMaterialGroupsPaged(1, "tokenhub", "", "", 99, 2)
		require.NoError(t, err)
		assert.Len(t, p4, 0)

		// 三页合并是全集（不重不漏）
		all := append(append(p1, p2...), p3...)
		seen := make(map[string]bool, len(all))
		for _, g := range all {
			seen[g.SeedanceAssetGroupId] = true
		}
		assert.Len(t, seen, 5)
	})

	t.Run("name 过滤组名与组 id", func(t *testing.T) {
		total, groups, err := GetMaterialGroupsPaged(0, "", "", "page-name-3", 1, 20)
		require.NoError(t, err)
		require.Len(t, groups, 1)
		assert.EqualValues(t, 1, total)
		assert.Equal(t, "page_g3", groups[0].SeedanceAssetGroupId)

		total, groups, err = GetMaterialGroupsPaged(0, "", "", "page_g2", 1, 20)
		require.NoError(t, err)
		assert.EqualValues(t, 1, total)
		assert.Equal(t, "page_g2", groups[0].SeedanceAssetGroupId)
	})

	t.Run("name 与类型过滤叠加", func(t *testing.T) {
		total, groups, err := GetMaterialGroupsPaged(0, "", "LivenessFace", "page-name", 1, 20)
		require.NoError(t, err)
		assert.EqualValues(t, 0, total)
		assert.Len(t, groups, 0)
	})

	t.Run("LIKE 通配符被转义为字面量", func(t *testing.T) {
		// 输入 % 应只匹配字面 %（当前数据无含 % 的组），转义后 0 条；不转义会误匹配全部。
		total, groups, err := GetMaterialGroupsPaged(0, "", "", "%", 1, 20)
		require.NoError(t, err)
		assert.EqualValues(t, 0, total)
		assert.Len(t, groups, 0)
	})
}

// TestMaterialNameLikeEscape 验证组名搜索的 LIKE ESCAPE 跨库契约：用 ! 而非 \ 作 ESCAPE。
// SQLite 测试库对 ESCAPE '\' 也能跑通（单引号内反斜杠不转义），只有 MySQL 会因反斜杠
// 转义掉闭合引号而语法错误（1064），故用 DryRun 捕获 SQL 文本断言，防回归到 \。
func TestMaterialNameLikeEscape(t *testing.T) {
	dummyDB, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)

	// Find 触发 DryRun 构建 SQL（同 locking_test 范式），断言 ESCAPE 契约文本。
	var rows []UserMaterialGroup
	query := applyMaterialNameLike(dummyDB.Model(&UserMaterialGroup{}), "测试")
	sql := query.Find(&rows).Statement.SQL.String()
	assert.Contains(t, sql, "ESCAPE '!'")
	assert.NotContains(t, sql, `ESCAPE '\`)
}
