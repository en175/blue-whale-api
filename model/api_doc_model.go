package model

import (
	"github.com/QuantumNous/new-api/common"
)

// ApiDocModel 对外文档模型：对外模型清单 + 参数模板（单一数据源）
type ApiDocModel struct {
	Id          int    `json:"id"`
	ModelName   string `json:"model_name" gorm:"size:128;not null;uniqueIndex"`
	DisplayName string `json:"display_name" gorm:"size:255"`
	Description string `json:"description" gorm:"type:text"`
	Tag         string `json:"tag" gorm:"size:64"`
	ParamSchema string `json:"param_schema" gorm:"type:text"` // JSON 字符串，见契约
	SortOrder   int    `json:"sort_order" gorm:"default:0"`
	Enabled     bool   `json:"enabled" gorm:"default:false"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
}

func (m *ApiDocModel) Insert() error {
	m.CreatedTime = common.GetTimestamp()
	m.UpdatedTime = m.CreatedTime
	return DB.Create(m).Error
}

func (m *ApiDocModel) Update() error {
	m.UpdatedTime = common.GetTimestamp()
	return DB.Model(&ApiDocModel{}).Where("id = ?", m.Id).Updates(map[string]any{
		"model_name":   m.ModelName,
		"display_name": m.DisplayName,
		"description":  m.Description,
		"tag":          m.Tag,
		"param_schema": m.ParamSchema,
		"sort_order":   m.SortOrder,
		"enabled":      m.Enabled,
		"updated_time": m.UpdatedTime,
	}).Error
}

func (m *ApiDocModel) Delete() error {
	return DB.Delete(&ApiDocModel{}, "id = ?", m.Id).Error
}

func GetApiDocModel(id int) (*ApiDocModel, error) {
	var m ApiDocModel
	if err := DB.First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func GetAllApiDocModels() ([]*ApiDocModel, error) {
	var list []*ApiDocModel
	err := DB.Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}

func GetEnabledApiDocModels() ([]*ApiDocModel, error) {
	var list []*ApiDocModel
	err := DB.Where("enabled = ?", true).Order("sort_order asc, id asc").Find(&list).Error
	return list, err
}
