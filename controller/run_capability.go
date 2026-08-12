package controller

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func IssueRunCapability(c *gin.Context) {
	expected := strings.TrimSpace(os.Getenv("BLUE_WHALE_INTERNAL_SERVICE_TOKEN"))
	if expected == "" || c.GetHeader("Authorization") != "Bearer "+expected {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "trusted service identity required"}})
		return
	}
	var request service.RunCapabilityRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_json"}})
		return
	}
	grant, err := service.IssueRunCapability(c, request, time.Now())
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "capability_denied", "message": "capability request denied"}})
		return
	}
	c.JSON(http.StatusCreated, grant)
}
