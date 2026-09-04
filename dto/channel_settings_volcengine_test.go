package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateVolcEngineEnhancedPercents(t *testing.T) {
	err := ChannelSettings{Lottery1080To720Percent: 60, Lottery1080To480Percent: 50, MediaKitAPIKey: "k"}.ValidateVolcEngineEnhanced()
	require.Error(t, err)

	err = ChannelSettings{Lottery720To480Percent: 10}.ValidateVolcEngineEnhanced()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mediakit_api_key")

	require.NoError(t, ChannelSettings{}.ValidateVolcEngineEnhanced())
	require.NoError(t, ChannelSettings{Lottery1080To720Percent: 30, Lottery1080To480Percent: 20, Lottery720To480Percent: 10, MediaKitAPIKey: "k"}.ValidateVolcEngineEnhanced())
}
