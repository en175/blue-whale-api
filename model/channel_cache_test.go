package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/stretchr/testify/require"
)

func TestPickChannelByPriorityAndWeightUsesRetryRankedPriorities(t *testing.T) {
	priorityHigh := int64(10)
	priorityLow := int64(1)
	weight := uint(1)

	high := &Channel{Id: 1, Priority: &priorityHigh, Weight: &weight}
	low := &Channel{Id: 2, Priority: &priorityLow, Weight: &weight}
	candidates := []*Channel{low, high}

	first, err := pickChannelByPriorityAndWeight(candidates, 0, "test")
	require.NoError(t, err)
	require.Equal(t, high.Id, first.Id)

	second, err := pickChannelByPriorityAndWeight(candidates, 1, "test")
	require.NoError(t, err)
	require.Equal(t, low.Id, second.Id)

	// retry 超过优先级数量时收敛到最后一个优先级
	capped, err := pickChannelByPriorityAndWeight(candidates, 99, "test")
	require.NoError(t, err)
	require.Equal(t, low.Id, capped.Id)
}

func TestPickChannelByPriorityAndWeightEmpty(t *testing.T) {
	channel, err := pickChannelByPriorityAndWeight(nil, 0, "test")
	require.NoError(t, err)
	require.Nil(t, channel)
}

func TestGetRandomSatisfiedChannelByTypeSelectsGroupAndChannelType(t *testing.T) {
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousCache
		DB.Exec("DELETE FROM abilities WHERE channel_id IN (SELECT id FROM channels WHERE name LIKE 'type-select-%')")
		DB.Exec("DELETE FROM channels WHERE name LIKE 'type-select-%'")
	})

	priorityHigh := int64(10)
	priorityLow := int64(1)
	weight := uint(1)

	deepSeekDefault := &Channel{
		Type:     constant.ChannelTypeDeepSeek,
		Key:      "key-default",
		Status:   common.ChannelStatusEnabled,
		Name:     "type-select-default",
		Group:    "default",
		Models:   "deepseek-chat",
		Priority: &priorityHigh,
		Weight:   &weight,
	}
	deepSeekVip := &Channel{
		Type:     constant.ChannelTypeDeepSeek,
		Key:      "key-vip",
		Status:   common.ChannelStatusEnabled,
		Name:     "type-select-vip",
		Group:    "vip",
		Models:   "deepseek-chat",
		Priority: &priorityLow,
		Weight:   &weight,
	}
	openAIDefault := &Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "key-openai",
		Status:   common.ChannelStatusEnabled,
		Name:     "type-select-openai",
		Group:    "default",
		Models:   "gpt-4o",
		Priority: &priorityHigh,
		Weight:   &weight,
	}
	for _, channel := range []*Channel{deepSeekDefault, deepSeekVip, openAIDefault} {
		require.NoError(t, DB.Create(channel).Error)
	}

	t.Run("db path filters type and group", func(t *testing.T) {
		channel, err := GetRandomSatisfiedChannelByType("default", constant.ChannelTypeDeepSeek, 0)
		require.NoError(t, err)
		require.Equal(t, deepSeekDefault.Id, channel.Id)

		channel, err = GetRandomSatisfiedChannelByType("vip", constant.ChannelTypeDeepSeek, 0)
		require.NoError(t, err)
		require.Equal(t, deepSeekVip.Id, channel.Id)

		channel, err = GetRandomSatisfiedChannelByType("svip", constant.ChannelTypeDeepSeek, 0)
		require.NoError(t, err)
		require.Nil(t, channel)

		models, err := GetEnabledChannelTypeModels("default", constant.ChannelTypeDeepSeek)
		require.NoError(t, err)
		require.Contains(t, models, "deepseek-chat")
		require.NotContains(t, models, "gpt-4o")
	})

	t.Run("memory cache path uses the same picker", func(t *testing.T) {
		for _, channel := range []*Channel{deepSeekDefault, deepSeekVip, openAIDefault} {
			require.NoError(t, DB.Create(&Ability{
				Group:     channel.Group,
				Model:     channel.Models,
				ChannelId: channel.Id,
				Enabled:   true,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
			}).Error)
		}

		common.MemoryCacheEnabled = true
		InitChannelCache()
		t.Cleanup(func() {
			common.MemoryCacheEnabled = false
		})

		channel, err := GetRandomSatisfiedChannelByType("vip", constant.ChannelTypeDeepSeek, 0)
		require.NoError(t, err)
		require.Equal(t, deepSeekVip.Id, channel.Id)

		channel, err = GetRandomSatisfiedChannelByType("default", constant.ChannelTypeDeepSeek, 0)
		require.NoError(t, err)
		require.Equal(t, deepSeekDefault.Id, channel.Id)

		models, err := GetEnabledChannelTypeModels("default", constant.ChannelTypeDeepSeek)
		require.NoError(t, err)
		require.Contains(t, models, "deepseek-chat")
		require.NotContains(t, models, "gpt-4o")
	})
}
