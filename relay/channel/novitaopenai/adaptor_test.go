package novitaopenai

import (
	"testing"

	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/stretchr/testify/assert"
)

func TestAdaptorContract(t *testing.T) {
	var _ channel.Adaptor = (*Adaptor)(nil)
}

func TestAdaptorIdentity(t *testing.T) {
	adaptor := &Adaptor{}
	assert.Equal(t, ChannelName, adaptor.GetChannelName())
	assert.Equal(t, ModelList, adaptor.GetModelList())
	assert.Equal(t, []string{
		"gpt-image-2",
		"gpt-image-2.5-flare-oai",
		"gpt-image-2.5-sunburst-oai",
	}, adaptor.GetModelList())
}
