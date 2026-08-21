package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVisualSessionCRUD 覆盖真人认证会话表的写入/反查/完成回填。
func TestVisualSessionCRUD(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialVisualSession{}))

	require.NoError(t, AddVisualValidateSession(42, "volcengine", "真人组1", "token-1", "https://h5/1", "https://gw/cb", "default"))

	s, err := GetVisualValidateSessionByToken("token-1")
	require.NoError(t, err)
	assert.Equal(t, 42, s.UserId)
	assert.Equal(t, "volcengine", s.Provider)
	assert.Equal(t, "真人组1", s.Name)
	assert.Equal(t, "https://h5/1", s.H5Link)
	assert.Equal(t, "https://gw/cb", s.CallbackURL)
	assert.Equal(t, "default", s.ProjectName)
	assert.Equal(t, VisualSessionProcessing, s.Status)

	// 完成：回填 groupId 并标记 active
	require.NoError(t, CompleteVisualValidateSession("token-1", "group-1", VisualSessionActive))
	s, err = GetVisualValidateSessionByToken("token-1")
	require.NoError(t, err)
	assert.Equal(t, "group-1", s.GroupId)
	assert.Equal(t, VisualSessionActive, s.Status)
}

// TestGetVisualValidateSessionByToken_notFound 验证会话不存在时返回错误。
func TestGetVisualValidateSessionByToken_notFound(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&UserMaterialVisualSession{}))
	_, err := GetVisualValidateSessionByToken("nope")
	require.Error(t, err)
	assert.True(t, IsVisualSessionNotFound(err))
}
