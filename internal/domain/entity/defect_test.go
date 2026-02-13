package entity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefectAreaCenter(t *testing.T) {
	d := DefectArea{X: 10, Y: 20, Width: 8, Height: 6}
	x, y := d.Center()
	require.Equal(t, 14, x)
	require.Equal(t, 23, y)
}
