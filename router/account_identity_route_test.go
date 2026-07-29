package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountIdentityRouteIsMountedOnRealAPIRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	require.NotPanics(t, func() {
		SetApiRouter(engine)
	})

	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/token/identity" {
			assert.Contains(t, route.Handler, "GetTokenIdentity")
			return
		}
	}
	t.Fatal("GET /api/token/identity route is not mounted")
}
