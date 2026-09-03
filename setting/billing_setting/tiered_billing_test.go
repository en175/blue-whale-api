package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBillingExprMap_Valid(t *testing.T) {
	raw := `{"doubao-seedream-5-0-pro":"has(param(\"size\"), \"2048\") ? tier(\"hd\", (param(\"n\") ?? 1.0) * 600000) : tier(\"standard\", (param(\"n\") ?? 1.0) * 300000)"}`
	require.NoError(t, ValidateBillingExprMap(raw))
}

func TestValidateBillingExprMap_EmptyExprAllowed(t *testing.T) {
	// An empty expression is tolerated at save time (it just stays unset).
	require.NoError(t, ValidateBillingExprMap(`{"some-model":""}`))
}

func TestValidateBillingExprMap_EmptyObject(t *testing.T) {
	require.NoError(t, ValidateBillingExprMap(`{}`))
}

func TestValidateBillingExprMap_InvalidJSON(t *testing.T) {
	err := ValidateBillingExprMap(`{not-json`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestValidateBillingExprMap_CompileError(t *testing.T) {
	// Syntactically broken expression must be rejected before save.
	err := ValidateBillingExprMap(`{"m":"tier("}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model m")
}

func TestValidateBillingExprMap_NilParamRuntimeRejected(t *testing.T) {
	// param() without a nil fallback crashes at pre-consume when the field is
	// absent (e.g. multipart edits); the smoke test must reject it at save time.
	err := ValidateBillingExprMap(`{"m":"tier(\"base\", param(\"n\") * 42857)"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model m")
}

func TestValidateBillingExprMap_NegativeResultRejected(t *testing.T) {
	err := ValidateBillingExprMap(`{"m":"tier(\"base\", p * -1.0)"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model m")
}
