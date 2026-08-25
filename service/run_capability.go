package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

const runCapabilityPrefix = "bwcap_"

var ErrRunCapabilityDenied = errors.New("run capability denied")

// ErrRunCapabilityExhausted separates "this capability has spent what it was given" from every
// other reason a capability is refused.
//
// They were one error, reported to the consumer as one 401 reading "invalid, out of scope, or
// exhausted". A consumer cannot act on that: an invalid or out-of-scope capability is a fault and
// the turn should fail, while an exhausted one is the breaker doing its job and the right response
// is to stop starting new calls and deliver what the turn already produced. Collapsing the two
// meant the second was handled like the first — a mid-stream abort that discarded text the User was
// already reading, and any work the turn had committed.
var ErrRunCapabilityExhausted = errors.New("run capability exhausted")

type RunCapabilityRequest struct {
	SubjectUserId   int      `json:"subject_user_id"`
	Consumer        string   `json:"consumer"`
	Purpose         string   `json:"purpose"`
	ResourceId      string   `json:"resource_id"`
	Models          []string `json:"models"`
	MaxModelCalls   int      `json:"max_model_calls"`
	MaxInputTokens  int      `json:"max_input_tokens"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	// MaxTotalTokens bounds raw tokens — prompt plus completion, undiscounted — summed across every
	// call the capability authorizes.
	//
	// It was introduced as a spend bound on the premise that "money is spent in tokens". That premise
	// is false: money is spent in quota, which prices cached prompt tokens, completion tokens, the
	// model and the group each at their own ratio. Measured against real agent traffic the two
	// diverge by 1.5x to 8.7x, and the gap widens precisely as prompt caching improves — so the
	// better a consumer is at caching, the sooner this bound stops it. See MaxSpendQuota.
	//
	// Kept, and still enforced, for callers that predate MaxSpendQuota. New callers should send
	// MaxSpendQuota instead and leave this at zero.
	MaxTotalTokens int `json:"max_total_tokens"`
	// MaxSpendQuota bounds what the capability may actually spend, in the same quota unit the
	// billing path settles in (common.QuotaPerUnit quota to the dollar). Charged from the same
	// value SettleBilling uses, so the bound and the invoice cannot drift.
	//
	// This is a breaker, not a budget, and the two are sized by opposite rules. A budget sits where
	// you want spending to stop and is meant to be reached; a breaker sits where normal operation
	// never reaches, and tripping during ordinary use means it is set wrong. Money is already
	// bounded per user by the balance the relay pre-consumes against — that is what says "you are
	// out of funds". This says "you have run away", and nothing else. Size it from the observed
	// spend distribution with room above it, never from what a run ought to be allowed to cost.
	//
	// Zero means unset: the capability then falls back to MaxTotalTokens, as before.
	MaxSpendQuota  int    `json:"max_spend_quota"`
	ExpiresAt      int64  `json:"expires_at"`
	IdempotencyKey string `json:"idempotency_key"`
	TraceId        string `json:"trace_id"`
}

type RunCapabilityGrant struct {
	CapabilityId string `json:"capability_id"`
	Capability   string `json:"capability"`
	ExpiresAt    int64  `json:"expires_at"`
}

type RunCapabilityContext struct {
	CapabilityId    string
	SubjectUserId   int
	Consumer        string
	ResourceId      string
	Models          []string
	MaxOutputTokens int
	MaxInputTokens  int
	// Digest of the capability value, so the post-billing path can charge real usage against the
	// same Redis record the pre-flight check consumed.
	Digest string
}

var issueRunCapability = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[2],
  'id', ARGV[1], 'subject_user_id', ARGV[2], 'consumer', ARGV[3],
  'resource_id', ARGV[4], 'models', ARGV[5], 'max_calls', ARGV[6],
  'used_calls', 0, 'max_output_tokens', ARGV[7],
  'max_input_tokens', ARGV[8], 'status', 'active',
  'max_total_tokens', ARGV[11], 'used_tokens', 0,
  'max_spend_quota', ARGV[12], 'used_quota', 0)
redis.call('PEXPIRE', KEYS[2], ARGV[9])
redis.call('SET', KEYS[1], ARGV[10], 'PX', ARGV[9])
return 1
`)

// runCapabilityMaxCalls is a runaway breaker, not a budget. It exists so a looping consumer cannot
// hold the gateway open indefinitely even while each individual call stays cheap. Spend is bounded
// by max_total_tokens, so this ceiling is deliberately loose: an agent's reasoning depth is a
// product decision and must not require re-tuning a safety limit every time it changes.
const runCapabilityMaxCalls = 64

// runCapabilityMaxTotalTokens caps what any single capability may authorize, whatever the caller
// asks for.
const runCapabilityMaxTotalTokens = 2_000_000

// runCapabilityMaxSpendQuota is the same ceiling expressed in quota. common.QuotaPerUnit is defined
// as $0.002 per 1K tokens, so the 2,000,000-token ceiling above is exactly 2,000,000 quota at that
// definitional rate; the two ceilings therefore stay the same size rather than one quietly becoming
// the looser of the pair.
const runCapabilityMaxSpendQuota = 2_000_000

// resolveTotalTokenBudget keeps capabilities minted by callers that predate max_total_tokens working
// by deriving the budget those bounds already implied: every call could spend its full input and
// output allowance. It is a compatibility path, not a default worth relying on — an explicit budget
// is always tighter than this.
func resolveTotalTokenBudget(request RunCapabilityRequest) int {
	if request.MaxTotalTokens > 0 {
		return request.MaxTotalTokens
	}
	// A caller that sends a quota budget has said what it wants bounded. Deriving a token budget on
	// top would reintroduce the very bound MaxSpendQuota exists to replace, and it would bind first,
	// since the derived figure is the loosest one the other bounds already imply.
	if request.MaxSpendQuota > 0 {
		return 0
	}
	implied := request.MaxModelCalls * (request.MaxInputTokens + request.MaxOutputTokens)
	if implied <= 0 || implied > runCapabilityMaxTotalTokens {
		return runCapabilityMaxTotalTokens
	}
	return implied
}

func capabilityDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func IssueRunCapability(ctx context.Context, request RunCapabilityRequest, now time.Time) (*RunCapabilityGrant, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, fmt.Errorf("%w: redis unavailable", ErrRunCapabilityDenied)
	}
	if request.SubjectUserId <= 0 || request.Consumer != "blue-whale-agent-runtime" || request.Purpose != "agent_run" || request.ResourceId == "" || len(request.Models) == 0 || request.MaxModelCalls < 1 || request.MaxModelCalls > runCapabilityMaxCalls || request.MaxOutputTokens < 1 || request.MaxTotalTokens < 0 || request.MaxTotalTokens > runCapabilityMaxTotalTokens || request.MaxSpendQuota < 0 || request.MaxSpendQuota > runCapabilityMaxSpendQuota || request.ExpiresAt <= now.Unix() || request.ExpiresAt > now.Add(10*time.Minute).Unix() || request.IdempotencyKey == "" {
		return nil, ErrRunCapabilityDenied
	}
	totalTokenBudget := resolveTotalTokenBudget(request)
	user, err := model.GetUserCache(request.SubjectUserId)
	if err != nil || user.Status != common.UserStatusEnabled {
		return nil, ErrRunCapabilityDenied
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	value := runCapabilityPrefix + hex.EncodeToString(bytes)
	digest := capabilityDigest(value)
	id := "runcap_" + digest[:24]
	key := "run-capability:" + digest
	ttl := time.Until(time.Unix(request.ExpiresAt, 0))
	idempotencyKey := "run-capability-idempotency:" + capabilityDigest(request.Consumer+":"+request.IdempotencyKey)
	accepted, err := issueRunCapability.Run(ctx, common.RDB, []string{idempotencyKey, key},
		id, request.SubjectUserId, request.Consumer, request.ResourceId,
		strings.Join(request.Models, ","), request.MaxModelCalls,
		request.MaxOutputTokens, request.MaxInputTokens, ttl.Milliseconds(), digest,
		totalTokenBudget, request.MaxSpendQuota,
	).Int()
	if err != nil || accepted != 1 {
		return nil, ErrRunCapabilityDenied
	}
	return &RunCapabilityGrant{CapabilityId: id, Capability: value, ExpiresAt: request.ExpiresAt}, nil
}

var consumeRunCapability = redis.NewScript(`
local values = redis.call('HMGET', KEYS[1], 'id', 'subject_user_id', 'consumer', 'resource_id', 'models', 'max_calls', 'used_calls', 'max_output_tokens', 'status', 'max_input_tokens', 'max_total_tokens', 'used_tokens', 'max_spend_quota', 'used_quota')
if not values[1] or values[9] ~= 'active' then return {err='denied'} end
local allowed = false
for model in string.gmatch(values[5], '([^,]+)') do
  if model == ARGV[1] then allowed = true break end
end
if not allowed then return {err='scope_denied'} end
local requested_output = tonumber(ARGV[2]) or 0
if requested_output < 0 or requested_output > tonumber(values[8]) then return {err='scope_denied'} end
if tonumber(values[7]) >= tonumber(values[6]) then return {err='exhausted'} end
local budget = tonumber(values[11])
if budget and budget > 0 and tonumber(values[12] or 0) >= budget then return {err='budget_exhausted'} end
local quota_budget = tonumber(values[13])
if quota_budget and quota_budget > 0 and tonumber(values[14] or 0) >= quota_budget then return {err='budget_exhausted'} end
redis.call('HINCRBY', KEYS[1], 'used_calls', 1)
return values
`)

// recordRunCapabilityUsage charges settled usage against the capability. It runs after the model
// call, because only then is the real cost is known: the pre-flight check can bound one call's
// request, never what it turns out to cost.
//
// Both counters advance on every call, whichever bound the capability was minted with. Charging
// only the one in force would mean the record could not answer what a capability actually cost in
// the other unit, and a capability carrying both bounds is then held to both — strictly the safer
// reading of a caller that asked for two.
var recordRunCapabilityUsage = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('HINCRBY', KEYS[1], 'used_quota', ARGV[2])
return redis.call('HINCRBY', KEYS[1], 'used_tokens', ARGV[1])
`)

// classifyConsumeRefusal maps the consume script's refusal to a sentinel the caller can branch on.
// The script signals its reason in the error text; `exhausted` covers the call breaker and both
// budgets, which are one event from a consumer's point of view — it has spent what it was given.
func classifyConsumeRefusal(err error) error {
	if strings.Contains(err.Error(), "exhausted") {
		return fmt.Errorf("%w: %v", ErrRunCapabilityExhausted, err)
	}
	return fmt.Errorf("%w: %v", ErrRunCapabilityDenied, err)
}

func ConsumeRunCapability(ctx context.Context, value, model string, requestedOutput int) (*RunCapabilityContext, error) {
	if !strings.HasPrefix(value, runCapabilityPrefix) || !common.RedisEnabled || common.RDB == nil {
		return nil, ErrRunCapabilityDenied
	}
	key := "run-capability:" + capabilityDigest(value)
	result, err := consumeRunCapability.Run(ctx, common.RDB, []string{key}, model, requestedOutput).Result()
	if err != nil {
		return nil, classifyConsumeRefusal(err)
	}
	values, ok := result.([]interface{})
	if !ok || len(values) != 14 {
		return nil, ErrRunCapabilityDenied
	}
	text := func(i int) string {
		if values[i] == nil {
			return ""
		}
		return fmt.Sprint(values[i])
	}
	userId, err := strconv.Atoi(text(1))
	if err != nil || userId <= 0 {
		return nil, ErrRunCapabilityDenied
	}
	maxOutput, _ := strconv.Atoi(text(7))
	maxInput, _ := strconv.Atoi(text(9))
	return &RunCapabilityContext{CapabilityId: text(0), SubjectUserId: userId, Consumer: text(2), ResourceId: text(3), Models: strings.Split(text(4), ","), MaxOutputTokens: maxOutput, MaxInputTokens: maxInput, Digest: capabilityDigest(value)}, nil
}

// RecordRunCapabilityUsage charges settled usage against the capability that authorized the call.
//
// `quota` must be the same value the billing path settles — pass what SettleBilling was given, not
// a recomputation. The whole point of the quota bound is that it cannot drift from the invoice, and
// it drifts the moment two places work it out separately.
//
// Failures are non-fatal to the request that already completed: the call is paid for either way,
// and losing one increment must not fail a response the user already received. The capability
// expires on its own within ten minutes, which bounds what a lost increment can cost.
func RecordRunCapabilityUsage(ctx context.Context, digest string, totalTokens int, quota int) {
	if digest == "" || !common.RedisEnabled || common.RDB == nil {
		return
	}
	if totalTokens <= 0 && quota <= 0 {
		return
	}
	_ = recordRunCapabilityUsage.Run(ctx, common.RDB, []string{"run-capability:" + digest}, max(totalTokens, 0), max(quota, 0)).Err()
}
