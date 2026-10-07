package ratio_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

// defaultGroupModelRatio 分组模型折扣没有出厂值——它是站点自定的业务策略。
// 对照 defaultGroupGroupRatio 里的 "edit_this" 占位符，这里留空以免留下同样
// 永不命中的哑配置。
var defaultGroupModelRatio = map[string]map[string]float64{}

// groupModelRatioMap 是「使用组 × 模型」的两层表：同一分组内不同模型可以给不同折扣。
var groupModelRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupModelRatio         *types.RWMap[string, map[string]float64] `json:"group_model_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)
	groupModelRatioMap.AddAll(defaultGroupModelRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		GroupModelRatio:         groupModelRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.GroupModelRatio == nil {
		groupRatioSetting.GroupModelRatio = types.NewRWMap[string, map[string]float64]()
		groupRatioSetting.GroupModelRatio.AddAll(defaultGroupModelRatio)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

// GetGroupModelRatio 查询「使用组 × 模型」折扣。模型名先过 FormatMatchingModelName
// 归一化，与 GetModelRatio 用同一把键。未命中返回 -1, false。
func GetGroupModelRatio(usingGroup, modelName string) (float64, bool) {
	modelRatios, ok := groupModelRatioMap.Get(usingGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := modelRatios[FormatMatchingModelName(modelName)]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GetGroupModelRatioCopy() map[string]map[string]float64 {
	return groupModelRatioMap.ReadAll()
}

func GroupModelRatio2JSONString() string {
	return groupModelRatioMap.MarshalJSONString()
}

func UpdateGroupModelRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupModelRatioMap, jsonStr)
}

// CheckGroupModelRatio 校验「使用组 × 模型」折扣表。允许 0（该模型在该组下免费），
// 但拒绝负数与非有限值：它们会成为计费乘数，必须在入口挡住。
func CheckGroupModelRatio(jsonStr string) error {
	checkGroupModelRatio := make(map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &checkGroupModelRatio); err != nil {
		return err
	}
	for group, modelRatios := range checkGroupModelRatio {
		for modelName, ratio := range modelRatios {
			if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
				return fmt.Errorf("group model ratio must be a finite number not less than 0: %s / %s", group, modelName)
			}
		}
	}
	return nil
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
