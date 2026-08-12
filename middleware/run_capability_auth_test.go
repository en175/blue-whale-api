package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCapabilityModelScope(t *testing.T) {
	assert.True(t, containsCapabilityModel([]string{"MiniMax-M3"}, "MiniMax-M3"))
	assert.False(t, containsCapabilityModel([]string{"MiniMax-M3"}, "another-model"))
}

func TestRunCapabilityOutputBoundsPreserveAbsentAndZero(t *testing.T) {
	zero, allowed, excessive, negative := 0, 2048, 2049, -1
	assert.False(t, exceedsCapabilityOutput(nil, nil, 2048))
	assert.False(t, exceedsCapabilityOutput(&zero, &allowed, 2048))
	assert.True(t, exceedsCapabilityOutput(&excessive, nil, 2048))
	assert.True(t, exceedsCapabilityOutput(&negative, nil, 2048))
}
