package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// docModelCard 对外模型清单的统一返回形状（不含内部字段）
func docModelCard(d *model.ApiDocModel) gin.H {
	return gin.H{
		"model_name":   d.ModelName,
		"display_name": d.DisplayName,
		"description":  d.Description,
		"param_schema": d.ParamSchema,
		"sort_order":   d.SortOrder,
	}
}

// GetDocsConfig 构建时文档站拉取：返回对外模型清单 + 文档域名（公开只读）
func GetDocsConfig(c *gin.Context) {
	models, err := model.GetEnabledApiDocModels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	cards := make([]gin.H, 0, len(models))
	for _, m := range models {
		cards = append(cards, docModelCard(m))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"docs_domain": common.OptionMap["DocsDomain"],
			"models":      cards,
		},
	})
}

// GetUserDocsModels 控制台：对外清单 ∩ 用户能接的模型（含参数模板）
func GetUserDocsModels(c *gin.Context) {
	id := c.GetInt("id")
	user, err := model.GetUserCache(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	groups := service.GetUserUsableGroups(user.Group)
	usable := make(map[string]bool)
	for group := range groups {
		for _, m := range model.GetGroupEnabledModels(group) {
			usable[m] = true
		}
	}
	docs, err := model.GetEnabledApiDocModels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
		return
	}
	out := make([]gin.H, 0, len(docs))
	for _, d := range docs {
		if usable[d.ModelName] {
			out = append(out, docModelCard(d))
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": out})
}
