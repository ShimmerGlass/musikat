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

func TestReleaseEnd(t *testing.T) {
	type testCase struct {
		in       string
		expected time.Time
		full     bool
	}

	for _, tc := range []testCase{
		{"2020-02-12", time.Date(2020, time.February, 13, 0, 0, 0, 0, time.UTC), true},
		{"2020-02-29", time.Date(2020, time.March, 1, 0, 0, 0, 0, time.UTC), true},
		{"2020-12-31", time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC), true},
		{"2020-2-12", time.Date(2020, time.February, 13, 0, 0, 0, 0, time.UTC), true},
		{"2026-05", time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC), false},
		{"2026-12", time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC), false},
		{"2026-05-??", time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC), false},
		{"1998", time.Date(1999, time.January, 1, 0, 0, 0, 0, time.UTC), false},
		{"2005-??-31", time.Date(2006, time.January, 1, 0, 0, 0, 0, time.UTC), false},
		{"", time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC), false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			rg := ReleaseGroup{ReleaseDate: tc.in}
			require.Equal(t, tc.expected, rg.ReleaseEnd())
			require.Equal(t, tc.full, rg.IsFullDate())
		})
	}
}

func TestTypeLabel(t *testing.T) {
	for _, tc := range []struct {
		primary, secondary, want string
	}{
		{"Album", "", "Album"},
		{"Single", "", "Single"},
		{"Album", "Live,Compilation", "Album (Live, Compilation)"},
		{"", "", "Unknown"},
		{"", "Live", "Unknown (Live)"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			rg := ReleaseGroup{PrimaryType: tc.primary, XXSecondaryTypes: tc.secondary}
			require.Equal(t, tc.want, rg.TypeLabel())
		})
	}
}
