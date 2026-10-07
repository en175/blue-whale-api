package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率（不含模型维度），供分组定价预览使用。
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, _ := ResolveGroupRatio(userGroup, group, "")
	return ratio
}

// ResolveGroupRatio 解析一次请求最终适用的收费倍率，优先级从高到低：
//
//  1. 分组模型折扣 GroupModelRatio[使用组][模型]
//  2. 用户组特价   GroupGroupRatio[用户组][使用组]
//  3. 分组倍率     GroupRatio[使用组]
//
// 三层都是替换语义，命中即整体替换，绝不叠乘。modelName 传空串时跳过第 1 层。
//
// 预扣与差额结算必须走同一个函数：两边各自手写解析曾有结算漏掉用户组特价的
// 事故（IKJUBL），把规则收在这里是为了不再重演。
func ResolveGroupRatio(userGroup, usingGroup, modelName string) (float64, types.GroupRatioSource) {
	if modelName != "" {
		if ratio, ok := ratio_setting.GetGroupModelRatio(usingGroup, modelName); ok {
			return ratio, types.GroupRatioSourceGroupModel
		}
	}
	if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, usingGroup); ok {
		return ratio, types.GroupRatioSourceUserGroup
	}
	return ratio_setting.GetGroupRatio(usingGroup), types.GroupRatioSourceGroup
}
