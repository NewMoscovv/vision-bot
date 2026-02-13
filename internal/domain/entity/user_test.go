package entity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUser_DefaultState(t *testing.T) {
	u := NewUser(1, 10)
	require.Equal(t, StateMainMenu, u.State)
	require.Equal(t, int64(1), u.ID)
	require.Equal(t, int64(10), u.ChatID)
}
