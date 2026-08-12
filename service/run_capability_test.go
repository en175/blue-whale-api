package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueRunCapabilityFailsClosedWithoutRedis(t *testing.T) {
	oldEnabled, oldClient := common.RedisEnabled, common.RDB
	common.RedisEnabled, common.RDB = false, nil
	t.Cleanup(func() { common.RedisEnabled, common.RDB = oldEnabled, oldClient })

	grant, err := IssueRunCapability(context.Background(), RunCapabilityRequest{
		SubjectUserId: 1, Consumer: "blue-whale-agent-runtime", Purpose: "agent_run",
		ResourceId: "run_1", Models: []string{"MiniMax-M3"}, MaxModelCalls: 1,
		MaxOutputTokens: 1024, ExpiresAt: time.Now().Add(time.Minute).Unix(), IdempotencyKey: "once",
	}, time.Now())

	require.ErrorIs(t, err, ErrRunCapabilityDenied)
	assert.Nil(t, grant)
}

func TestConsumeRunCapabilityRejectsOrdinaryCredential(t *testing.T) {
	grant, err := ConsumeRunCapability(context.Background(), "sk-user-token", "MiniMax-M3", 1024)
	require.ErrorIs(t, err, ErrRunCapabilityDenied)
	assert.Nil(t, grant)
}
