package router

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInternalCollectPayRouteIsMounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetInternalRouter(engine)

	found := false
	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/internal/v1/topup/pay" && strings.Contains(route.Handler, "CollectBlueWhalePay") {
			found = true
			break
		}
	}
	require.True(t, found, "POST /internal/v1/topup/pay must mount CollectBlueWhalePay")
}
