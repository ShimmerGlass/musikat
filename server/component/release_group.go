package component

import (
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/shimmerglass/musikat/database"
)

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
		return rg.PrimaryType + "\\" + rg.SecondaryType
	})

	items := lo.Entries(m)
	slices.SortFunc(items, func(a, b lo.Entry[string, []database.ReleaseGroup]) int {
		aPrimary := a.Value[0].PrimaryType
		aSecondary := a.Value[0].SecondaryType
		bPrimary := b.Value[0].PrimaryType
		bSecondary := b.Value[0].SecondaryType

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
			SecondaryTypes: strings.Split(item.Value[0].SecondaryType, ","),
			ReleaseGroups:  item.Value,
		}
	})
}
