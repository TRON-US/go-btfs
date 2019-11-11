package coreapi

import (
	"github.com/magiconair/properties/assert"
	"testing"
)

func TestIsPath(t *testing.T) {
	var pathResult = []struct {
		in  string
		out bool
	}{
		{".", true},
		{"/", true},
		{"~", true},
		{"|", false},
		{"", false},
	}
	for _, tt := range pathResult {
		t.Run(tt.in, func(t *testing.T) {
			r := isPath(tt.in)
			assert.Equal(t, r, tt.out)
		})
	}
}
