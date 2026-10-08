package ratio_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

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

func GroupModelRatio2JSONString() string {
	return groupModelRatioMap.MarshalJSONString()
}

// UpdateGroupModelRatioByJSONString 写入折扣表。空串按「清空」处理：管理员在设置页把
// 原始 JSON 清空时前端会发 value=""，而 encoding/json 对空输入报错，不归一化会让配置
// 既改不掉也留不下来。
func UpdateGroupModelRatioByJSONString(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		jsonStr = "{}"
	}
	return types.LoadFromJsonString(groupModelRatioMap, jsonStr)
}

// CheckGroupModelRatio 校验「使用组 × 模型」折扣表。允许 0（该模型在该组下免费），
// 但拒绝负数：它会成为计费乘数，必须在入口挡住。
// 空串与 UpdateGroupModelRatioByJSONString 一样表示「清空配置」。
//
// IsNaN/IsInf 是纵深防御，不是已验证的防护：当前唯一入口是选项 API 传入的 JSON 字符串，
// 而 JSON 没有 NaN 字面量、1e400 这类溢出会被 encoding/json 直接拒掉，所以这两个条件
// 实际走不到。保留是防校验函数将来被非 JSON 来源复用，删掉不划算。
func CheckGroupModelRatio(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}
	// 值解到 *float64 而非 float64：encoding/json 把 null 解进非指针标量是 no-op 且不报错，
	// 键照样建出来、值取零值，而 0 在这里的含义是「该模型在该组免费」——一条 `"m": null`
	// 就成了静默免费。指针能区分「写了数字」与「写了 null」，后者一律拒。
	checkGroupModelRatio := make(map[string]map[string]*float64)
	if err := common.Unmarshal([]byte(jsonStr), &checkGroupModelRatio); err != nil {
		return err
	}
	for group, modelRatios := range checkGroupModelRatio {
		// `"vip": null` 同样落成 nil map，不会报错也不会命中任何模型，属于垃圾配置
		if modelRatios == nil {
			return fmt.Errorf("group model ratio must be an object of models, got null: %s", group)
		}
		for modelName, ratio := range modelRatios {
			if ratio == nil {
				return fmt.Errorf("group model ratio must be a number, got null: %s / %s", group, modelName)
			}
			if math.IsNaN(*ratio) || math.IsInf(*ratio, 0) || *ratio < 0 {
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
