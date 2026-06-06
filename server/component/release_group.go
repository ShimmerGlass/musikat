package component

import (
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/shimmerglass/musikat/database"
)

func splitByDate(rgs []database.ReleaseGroup) (past, upcoming []database.ReleaseGroup) {
	now := time.Now()

	for _, rg := range rgs {
		if rg.ReleaseTime().After(now) {
			upcoming = append(upcoming, rg)
		} else {
			past = append(past, rg)
		}
	}
	return
}

var primaryOrder = map[string]int{
	"Album":     0,
	"EP":        1,
	"Single":    2,
	"Broadcast": 3,
	"Other":     4,
	"":          5,
}

type groupedReleaseGroups struct {
	PrimaryType    string
	SecondaryTypes []string
	ReleaseGroups  []database.ReleaseGroup
}

func groupReleaseGroups(rgs []database.ReleaseGroup) []groupedReleaseGroups {
	m := lo.GroupBy(rgs, func(rg database.ReleaseGroup) string {
		return rg.PrimaryType + "\\" + rg.XXSecondaryTypes
	})

	items := lo.Entries(m)
	slices.SortFunc(items, func(a, b lo.Entry[string, []database.ReleaseGroup]) int {
		aPrimary := a.Value[0].PrimaryType
		aSecondary := a.Value[0].XXSecondaryTypes
		bPrimary := b.Value[0].PrimaryType
		bSecondary := b.Value[0].XXSecondaryTypes

		if aSecondary == "" && bSecondary != "" {
			return -1
		}
		if bSecondary == "" && aSecondary != "" {
			return 1
		}

		if aPrimary == bPrimary {
			return strings.Compare(aSecondary, bSecondary)
		}

		return primaryOrder[aPrimary] - primaryOrder[bPrimary]
	})

	return lo.Map(items, func(item lo.Entry[string, []database.ReleaseGroup], _ int) groupedReleaseGroups {
		return groupedReleaseGroups{
			PrimaryType:    item.Value[0].PrimaryType,
			SecondaryTypes: item.Value[0].SecondaryTypes(),
			ReleaseGroups:  item.Value,
		}
	})
}
