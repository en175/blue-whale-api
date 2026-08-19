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

// The bound that protects the User's account is the token budget, so the derivation used for callers
// that predate it must never silently authorize more than the old bounds already implied.
func TestResolveTotalTokenBudgetPrefersExplicitRequest(t *testing.T) {
	explicit := resolveTotalTokenBudget(RunCapabilityRequest{
		MaxTotalTokens:  50_000,
		MaxModelCalls:   8,
		MaxInputTokens:  40_000,
		MaxOutputTokens: 8_000,
	})
	if explicit != 50_000 {
		t.Fatalf("explicit budget should win, got %d", explicit)
	}
}

func TestResolveTotalTokenBudgetDerivesLegacyBounds(t *testing.T) {
	// 8 calls that could each spend 40k in and 8k out is what the old bounds already permitted.
	derived := resolveTotalTokenBudget(RunCapabilityRequest{
		MaxModelCalls:   8,
		MaxInputTokens:  40_000,
		MaxOutputTokens: 8_000,
	})
	if derived != 8*(40_000+8_000) {
		t.Fatalf("derived budget should match what the legacy bounds implied, got %d", derived)
	}
}

func TestResolveTotalTokenBudgetIsAlwaysCapped(t *testing.T) {
	// A caller sending no usable bounds must not end up unbounded.
	for _, request := range []RunCapabilityRequest{
		{},
		{MaxModelCalls: 64, MaxInputTokens: 1_000_000, MaxOutputTokens: 1_000_000},
	} {
		if budget := resolveTotalTokenBudget(request); budget != runCapabilityMaxTotalTokens {
			t.Fatalf("budget should clamp to the ceiling, got %d", budget)
		}
	}
}

func TestRecordRunCapabilityUsageIsInertWithoutRedis(t *testing.T) {
	// Charging usage must never fail a response the User already received.
	RecordRunCapabilityUsage(context.Background(), "", 100)
	RecordRunCapabilityUsage(context.Background(), "deadbeef", 0)
	RecordRunCapabilityUsage(context.Background(), "deadbeef", 100)
}
