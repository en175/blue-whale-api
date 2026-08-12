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

type RunCapabilityRequest struct {
	SubjectUserId   int      `json:"subject_user_id"`
	Consumer        string   `json:"consumer"`
	Purpose         string   `json:"purpose"`
	ResourceId      string   `json:"resource_id"`
	Models          []string `json:"models"`
	MaxModelCalls   int      `json:"max_model_calls"`
	MaxInputTokens  int      `json:"max_input_tokens"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	ExpiresAt       int64    `json:"expires_at"`
	IdempotencyKey  string   `json:"idempotency_key"`
	TraceId         string   `json:"trace_id"`
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
}

var issueRunCapability = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[2],
  'id', ARGV[1], 'subject_user_id', ARGV[2], 'consumer', ARGV[3],
  'resource_id', ARGV[4], 'models', ARGV[5], 'max_calls', ARGV[6],
  'used_calls', 0, 'max_output_tokens', ARGV[7],
  'max_input_tokens', ARGV[8], 'status', 'active')
redis.call('PEXPIRE', KEYS[2], ARGV[9])
redis.call('SET', KEYS[1], ARGV[10], 'PX', ARGV[9])
return 1
`)

func capabilityDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func IssueRunCapability(ctx context.Context, request RunCapabilityRequest, now time.Time) (*RunCapabilityGrant, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, fmt.Errorf("%w: redis unavailable", ErrRunCapabilityDenied)
	}
	if request.SubjectUserId <= 0 || request.Consumer != "blue-whale-agent-runtime" || request.Purpose != "agent_run" || request.ResourceId == "" || len(request.Models) == 0 || request.MaxModelCalls < 1 || request.MaxModelCalls > 8 || request.MaxOutputTokens < 1 || request.ExpiresAt <= now.Unix() || request.ExpiresAt > now.Add(10*time.Minute).Unix() || request.IdempotencyKey == "" {
		return nil, ErrRunCapabilityDenied
	}
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
	).Int()
	if err != nil || accepted != 1 {
		return nil, ErrRunCapabilityDenied
	}
	return &RunCapabilityGrant{CapabilityId: id, Capability: value, ExpiresAt: request.ExpiresAt}, nil
}

var consumeRunCapability = redis.NewScript(`
local values = redis.call('HMGET', KEYS[1], 'id', 'subject_user_id', 'consumer', 'resource_id', 'models', 'max_calls', 'used_calls', 'max_output_tokens', 'status', 'max_input_tokens')
if not values[1] or values[9] ~= 'active' then return {err='denied'} end
local allowed = false
for model in string.gmatch(values[5], '([^,]+)') do
  if model == ARGV[1] then allowed = true break end
end
if not allowed then return {err='scope_denied'} end
local requested_output = tonumber(ARGV[2]) or 0
if requested_output < 0 or requested_output > tonumber(values[8]) then return {err='scope_denied'} end
if tonumber(values[7]) >= tonumber(values[6]) then return {err='exhausted'} end
redis.call('HINCRBY', KEYS[1], 'used_calls', 1)
return values
`)

func ConsumeRunCapability(ctx context.Context, value, model string, requestedOutput int) (*RunCapabilityContext, error) {
	if !strings.HasPrefix(value, runCapabilityPrefix) || !common.RedisEnabled || common.RDB == nil {
		return nil, ErrRunCapabilityDenied
	}
	key := "run-capability:" + capabilityDigest(value)
	result, err := consumeRunCapability.Run(ctx, common.RDB, []string{key}, model, requestedOutput).Result()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRunCapabilityDenied, err)
	}
	values, ok := result.([]interface{})
	if !ok || len(values) != 10 {
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
	return &RunCapabilityContext{CapabilityId: text(0), SubjectUserId: userId, Consumer: text(2), ResourceId: text(3), Models: strings.Split(text(4), ","), MaxOutputTokens: maxOutput, MaxInputTokens: maxInput}, nil
}
