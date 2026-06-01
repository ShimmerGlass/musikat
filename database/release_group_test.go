package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReleaseTime(t *testing.T) {
	type testCase struct {
		in       string
		expected time.Time
	}

	for _, tc := range []testCase{
		{
			"2020-2-12",
			time.Date(2020, time.February, 12, 0, 0, 0, 0, time.UTC),
		},
		{
			"1998",
			time.Date(1998, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"2005-??-31",
			time.Date(2005, time.January, 31, 0, 0, 0, 0, time.UTC),
		},
	} {
		t.Run(tc.in, func(t *testing.T) {
			rg := ReleaseGroup{ReleaseDate: tc.in}
			require.Equal(t, tc.expected, rg.ReleaseTime())
		})
	}
}
