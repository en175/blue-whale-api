package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVideoDownloadDisposition(t *testing.T) {
	assert.Equal(t, `attachment; filename="task_abc-1.mp4"`, videoDownloadDisposition("task_abc-1"))
	assert.Equal(t, `attachment; filename="taskabc.mp4"`, videoDownloadDisposition("task/abc"))
	assert.Equal(t, `attachment; filename="video.mp4"`, videoDownloadDisposition("..."))
}
