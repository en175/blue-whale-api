package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// NormalizeAllowedSystems 规范化白名单：trim、去重、只保留已知系统（bluewhale/api）、过滤垃圾值。
// 返回 "" 表示「全放行」（向后兼容，存量数据零迁移）。
func NormalizeAllowedSystems(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	seen := map[string]bool{}
	parts := make([]string, 0, 2)
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == common.AllowedSystemBlueWhale || p == common.AllowedSystemApi {
			if !seen[p] {
				seen[p] = true
				parts = append(parts, p)
			}
		}
	}
	return strings.Join(parts, common.AllowedSystemSeparator)
}

// ValidateAllowedSystemsInput 校验管理员提交的白名单输入。
// 空串 = 全放行（合法）；非空时每个逗号段必须是已知系统（bluewhale/api），
// 存在未知段视为非法 —— 否则「提交垃圾值=想收紧」会被 Normalize 静默变成空串=全放行。
func ValidateAllowedSystemsInput(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != common.AllowedSystemBlueWhale && p != common.AllowedSystemApi {
			return false
		}
	}
	return true
}

// ForceSystem 保证白名单含 system；空串 → 恰好该系统；已含 → 原样。
// 用于 root 账号强制保留控制台登录权（"api" 不可移除）。
func ForceSystem(raw string, system string) string {
	if raw == "" {
		return system
	}
	for _, p := range strings.Split(raw, ",") {
		if p == system {
			return raw
		}
	}
	return raw + common.AllowedSystemSeparator + system
}

// CanLoginTo 判断账号是否允许登录指定系统：白名单为空 = 全放行；
// 否则逗号逐段 trim 后 contains 判断。
func (user *User) CanLoginTo(system string) bool {
	if user == nil || strings.TrimSpace(user.AllowedSystems) == "" {
		return true
	}
	for _, p := range strings.Split(user.AllowedSystems, ",") {
		if strings.TrimSpace(p) == system {
			return true
		}
	}
	return false
}
