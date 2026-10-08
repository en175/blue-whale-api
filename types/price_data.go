package types

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// GroupRatioSource 标识最终收费倍率由哪一层分组规则解析而来。
// 三层是替换关系，只有一层生效，日志据此决定记哪个字段。
type GroupRatioSource int

const (
	// GroupRatioSourceGroup 分组倍率 GroupRatio[使用组]，兜底层。
	GroupRatioSourceGroup GroupRatioSource = iota
	// GroupRatioSourceUserGroup 用户组特价 GroupGroupRatio[用户组][使用组]。
	GroupRatioSourceUserGroup
	// GroupRatioSourceGroupModel 分组模型折扣 GroupModelRatio[使用组][模型]，优先级最高。
	GroupRatioSourceGroupModel
)

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
	// GroupModelRatio 命中分组模型折扣时的原始倍率；未命中为 -1，与
	// GroupSpecialRatio 的「-1 表示未设置」约定一致。
	GroupModelRatio    float64
	HasGroupModelRatio bool
}

// NewGroupRatioInfo 由解析出的倍率与来源构造计费信息。只填充真正生效的那一层，
// 其余保持 -1/ false，这样日志里出现的字段一定是当次实际使用的规则。
func NewGroupRatioInfo(ratio float64, source GroupRatioSource) GroupRatioInfo {
	info := GroupRatioInfo{
		GroupRatio:        ratio,
		GroupSpecialRatio: -1,
		GroupModelRatio:   -1,
	}
	switch source {
	case GroupRatioSourceGroupModel:
		info.GroupModelRatio = ratio
		info.HasGroupModelRatio = true
	case GroupRatioSourceUserGroup:
		info.GroupSpecialRatio = ratio
		info.HasSpecialRatio = true
	}
	return info
}

type PriceData struct {
	FreeModel            bool
	ModelPrice           float64
	ModelRatio           float64
	CompletionRatio      float64
	CacheRatio           float64
	CacheCreationRatio   float64
	CacheCreation5mRatio float64
	CacheCreation1hRatio float64
	ImageRatio           float64
	AudioRatio           float64
	AudioCompletionRatio float64
	otherRatios          map[string]float64
	UsePrice             bool
	Quota                int // 按次计费的最终额度（MJ / Task）
	QuotaToPreConsume    int // 按量计费的预消耗额度
	GroupRatioInfo       GroupRatioInfo
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if !isValidOtherRatio(ratio) {
		return
	}
	if p.otherRatios == nil {
		p.otherRatios = make(map[string]float64)
	}
	p.otherRatios[key] = ratio
}

func (p *PriceData) ReplaceOtherRatios(ratios map[string]float64) bool {
	p.otherRatios = nil
	for key, ratio := range ratios {
		p.AddOtherRatio(key, ratio)
	}
	return len(p.otherRatios) > 0
}

func (p *PriceData) HasOtherRatio(key string) bool {
	ratio, ok := p.otherRatios[key]
	return ok && isValidOtherRatio(ratio)
}

func (p *PriceData) OtherRatios() map[string]float64 {
	if len(p.otherRatios) == 0 {
		return nil
	}
	ratios := make(map[string]float64, len(p.otherRatios))
	for key, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) {
			ratios[key] = ratio
		}
	}
	if len(ratios) == 0 {
		return nil
	}
	return ratios
}

func (p *PriceData) OtherRatioMultiplier() float64 {
	multiplier := 1.0
	for _, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) && ratio != 1.0 {
			multiplier *= ratio
		}
	}
	return multiplier
}

func (p *PriceData) ApplyOtherRatiosToFloat(value float64) float64 {
	return value * p.OtherRatioMultiplier()
}

func (p *PriceData) ApplyOtherRatiosToDecimal(value decimal.Decimal) decimal.Decimal {
	for _, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) && ratio != 1.0 {
			value = value.Mul(decimal.NewFromFloat(ratio))
		}
	}
	return value
}

func (p *PriceData) RemoveOtherRatiosFromFloat(value float64) float64 {
	for _, ratio := range p.otherRatios {
		if isValidOtherRatio(ratio) && ratio != 1.0 {
			value /= ratio
		}
	}
	return value
}

func isValidOtherRatio(ratio float64) bool {
	// NaN/Inf would poison every downstream quota multiplication
	// (int(NaN * quota) wraps to a negative charge).
	return ratio > 0 && !math.IsInf(ratio, 1)
}

func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
