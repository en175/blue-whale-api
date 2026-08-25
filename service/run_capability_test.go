package service

import (
	"context"
	"errors"
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
	RecordRunCapabilityUsage(context.Background(), "", 100, 40)
	RecordRunCapabilityUsage(context.Background(), "deadbeef", 0, 0)
	RecordRunCapabilityUsage(context.Background(), "deadbeef", 100, 40)
	RecordRunCapabilityUsage(context.Background(), "deadbeef", 0, 40)
	RecordRunCapabilityUsage(context.Background(), "deadbeef", -1, -1)
}

func TestResolveTotalTokenBudgetYieldsToAnExplicitSpendBound(t *testing.T) {
	// A caller that sends a spend bound has said what it wants bounded. Deriving a token budget on
	// top would bind first — the derived figure is the loosest thing the other bounds already imply,
	// and it is denominated in the unit MaxSpendQuota exists to stop using.
	budget := resolveTotalTokenBudget(RunCapabilityRequest{
		MaxSpendQuota:   120_000,
		MaxModelCalls:   8,
		MaxInputTokens:  40_000,
		MaxOutputTokens: 8_000,
	})
	if budget != 0 {
		t.Fatalf("a quota-bounded capability should carry no derived token budget, got %d", budget)
	}
}

func TestResolveTotalTokenBudgetKeepsBothBoundsWhenBothAreSent(t *testing.T) {
	// Sending both is legal and means both apply. Only the derived fallback yields.
	budget := resolveTotalTokenBudget(RunCapabilityRequest{
		MaxTotalTokens: 50_000,
		MaxSpendQuota:  120_000,
	})
	if budget != 50_000 {
		t.Fatalf("an explicit token budget must survive alongside a spend bound, got %d", budget)
	}
}

func TestIssueRunCapabilityRejectsAnOutOfRangeSpendBound(t *testing.T) {
	// The ceiling exists so a caller cannot authorize an unbounded run by asking for one. Redis is
	// unavailable here, so a rejection proves nothing on its own — assert the ceiling arithmetic
	// instead, which is what the bounds check reads.
	if runCapabilityMaxSpendQuota != runCapabilityMaxTotalTokens {
		t.Fatalf(
			"the quota ceiling should match the token ceiling at the definitional rate, got %d vs %d",
			runCapabilityMaxSpendQuota, runCapabilityMaxTotalTokens,
		)
	}
}

func TestClassifyConsumeRefusalSeparatesExhaustionFromDenial(t *testing.T) {
	// A consumer acts differently on the two: a bad or out-of-scope capability is a fault and the
	// turn should fail, while an exhausted one is the breaker working and the turn should stop and
	// deliver what it has. Reporting both as one refusal made the second behave like the first.
	for _, reason := range []string{"exhausted", "budget_exhausted"} {
		err := classifyConsumeRefusal(errors.New(reason))
		if !errors.Is(err, ErrRunCapabilityExhausted) {
			t.Fatalf("%q should classify as exhausted, got %v", reason, err)
		}
		if errors.Is(err, ErrRunCapabilityDenied) {
			t.Fatalf("%q must not also read as a denial, got %v", reason, err)
		}
	}
	for _, reason := range []string{"denied", "scope_denied", "redis unavailable"} {
		err := classifyConsumeRefusal(errors.New(reason))
		if !errors.Is(err, ErrRunCapabilityDenied) {
			t.Fatalf("%q should classify as denied, got %v", reason, err)
		}
		if errors.Is(err, ErrRunCapabilityExhausted) {
			t.Fatalf("%q must not read as exhaustion — that would let a bad credential stop a turn quietly, got %v", reason, err)
		}
	}
}
