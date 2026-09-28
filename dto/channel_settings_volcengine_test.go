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

func TestValidateVolcEngineEnhanced4kPercents(t *testing.T) {
	base := ChannelSettings{MediaKitAPIKey: "k"}

	over := base
	over.Lottery4kTo1080Percent = 40
	over.Lottery4kTo720Percent = 30
	over.Lottery4kTo480Percent = 31
	err := over.ValidateVolcEngineEnhanced()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "4k_to_1080p_percent")

	negative := base
	negative.Lottery4kTo720Percent = -1
	require.Error(t, negative.ValidateVolcEngineEnhanced())

	require.NoError(t, ChannelSettings{}.ValidateVolcEngineEnhanced())

	// Any non-zero 4k percent requires the MediaKit key, same as the other tiers.
	withoutKey := ChannelSettings{Lottery4kTo480Percent: 1}
	err = withoutKey.ValidateVolcEngineEnhanced()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mediakit_api_key")

	full := base
	full.Lottery4kTo1080Percent = 40
	full.Lottery4kTo720Percent = 30
	full.Lottery4kTo480Percent = 30
	require.NoError(t, full.ValidateVolcEngineEnhanced())
}
