package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeAllowedSystems(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"spaces", "  ", ""},
		{"single", "bluewhale", "bluewhale"},
		{"both", "bluewhale,api", "bluewhale,api"},
		{"trim+dedupe", " bluewhale , api, bluewhale ", "bluewhale,api"},
		{"unknown filtered", "bluewhale,garbage", "bluewhale"},
		{"all unknown -> empty (all systems allowed)", "garbage,other", ""},
		{"reversed order preserved", "api,bluewhale", "api,bluewhale"},
		{"reversed with spaces", " api , bluewhale ", "api,bluewhale"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeAllowedSystems(tt.in))
		})
	}
}

func TestValidateAllowedSystemsInput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty allowed (all systems)", "", true},
		{"spaces only allowed", "  ", true},
		{"single valid", "bluewhale", true},
		{"both valid", "bluewhale,api", true},
		{"with spaces valid", " api , bluewhale ", true},
		{"unknown rejected", "garbage", false},
		{"mixed with unknown rejected", "bluewhale,garbage", false},
		{"all unknown rejected", "garbage,other", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidateAllowedSystemsInput(tt.in))
		})
	}
}

func TestForceSystem(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		system string
		want   string
	}{
		{"empty raw", "", "api", "api"},
		{"already contains", "bluewhale,api", "api", "bluewhale,api"},
		{"append", "bluewhale", "api", "bluewhale,api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ForceSystem(tt.raw, tt.system))
		})
	}
}

func TestUserCanLoginTo(t *testing.T) {
	tests := []struct {
		name    string
		allowed string
		system  string
		want    bool
	}{
		{"empty means all", "", "bluewhale", true},
		{"empty means all (api)", "", "api", true},
		{"contains", "bluewhale,api", "bluewhale", true},
		{"contains api", "bluewhale,api", "api", true},
		{"not contains", "bluewhale", "api", false},
		{"not contains reverse", "api", "bluewhale", false},
		{"nil user", "", "api", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{AllowedSystems: tt.allowed}
			if tt.name == "nil user" {
				user = nil
			}
			assert.Equal(t, tt.want, user.CanLoginTo(tt.system))
		})
	}
}

func TestEditWithTxNormalizesAndProtectsRoot(t *testing.T) {
	setupUserUpdateTestState(t)

	tests := []struct {
		name       string
		role       int
		submitted  string
		wantStored string
	}{
		{"common user normalized", common.RoleCommonUser, " bluewhale , api, garbage ", "bluewhale,api"},
		{"common user empty keeps empty", common.RoleCommonUser, "", ""},
		{"root forces api", common.RoleRootUser, "bluewhale", "bluewhale,api"},
		{"root empty becomes api only", common.RoleRootUser, "", "api"},
		{"root api stays", common.RoleRootUser, "api", "api"},
		{"root cannot drop api", common.RoleRootUser, "bluewhale,api", "bluewhale,api"},
		{"root all-garbage falls back to api", common.RoleRootUser, "garbage,other", "api"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := User{
				Username:       tt.name + "-user",
				Password:       "password",
				DisplayName:    "before",
				Role:           tt.role,
				Status:         common.UserStatusEnabled,
				AffCode:        fmt.Sprintf("aff%d", i),
				AllowedSystems: tt.submitted,
			}
			require.NoError(t, DB.Create(&user).Error)

			// 模拟后台编辑提交：只改 remark，allowed_systems 从请求体带入
			edited := user
			edited.Remark = "edited"
			edited.AllowedSystems = tt.submitted
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
				return edited.EditWithTx(tx, false)
			}))

			var got User
			require.NoError(t, DB.First(&got, user.Id).Error)
			assert.Equal(t, tt.wantStored, got.AllowedSystems)
			assert.Equal(t, "edited", got.Remark)
		})
	}
}

func TestEditWithTxNoChangeDoesNotTouchAllowedSystems(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Username:       "guard-user",
		Password:       "password",
		DisplayName:    "before",
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		AllowedSystems: "api", // 已收紧
	}
	require.NoError(t, DB.Create(&user).Error)

	// 调用方只发 username/display_name（AllowedSystems 零值），不得把已收紧账号扩成全放行
	edited := user
	edited.DisplayName = "after"
	edited.AllowedSystems = "" // 模拟未感知该字段的调用方
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return edited.EditWithTx(tx, false)
	}))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "api", got.AllowedSystems)
}
