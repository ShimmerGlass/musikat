package musicbrainz

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/shimmerglass/musikat/database"
	"go.uploadedlobster.com/mbtypes"
	mbz "go.uploadedlobster.com/musicbrainzws2"
)

type MusicBrainz struct {
	client *mbz.Client
	tick   <-chan time.Time
}

func New() *MusicBrainz {
	mbc := mbz.NewClient(mbz.AppInfo{
		Name:    "Musikat",
		Version: "0.0.1",
		URL:     "https://github.com/ShimmerGlass/musikat",
	})

	return &MusicBrainz{
		client: mbc,
		tick:   time.Tick(time.Second),
	}
}

func (m *MusicBrainz) ArtistReleaseGroups(ctx context.Context, artistMBzID string) ([]database.ReleaseGroup, error) {
	rgs := []mbz.ReleaseGroup{}
	paginator := paginator()

	for {
		<-m.tick
		mbRes, err := m.client.BrowseReleaseGroups(ctx, mbz.ReleaseGroupFilter{
			ArtistMBID: mbtypes.MBID(artistMBzID),
			Includes:   []string{"artist-credits"},
		}, paginator)
		if err != nil {
			return nil, fmt.Errorf("mbz artist release groups: %w", err)
		}

		rgs = append(rgs, mbRes.ReleaseGroups...)
		if len(rgs) >= mbRes.Count {
			break
		}

		paginator.Offset += len(mbRes.ReleaseGroups)
	}

	return lo.FilterMap(rgs, func(rg mbz.ReleaseGroup, _ int) (database.ReleaseGroup, bool) {
		if slices.Contains(rg.SecondaryTypes, "Compilation") {
			return database.ReleaseGroup{}, false
		}

		return database.ReleaseGroup{
			MBzID:         string(rg.ID),
			Name:          rg.Title,
			PrimaryType:   rg.PrimaryType,
			SecondaryType: strings.Join(rg.SecondaryTypes, ","),
			ReleaseDate:   rg.FirstReleaseDate.String(),

			Artists: lo.Map(rg.ArtistCredit, func(artist mbz.ArtistCreditEntry, _ int) database.Artist {
				return database.Artist{
					MBzID: string(artist.Artist.ID),
					Name:  artist.Artist.Name,
				}
			}),
		}, true
	}), nil
}

func (m *MusicBrainz) ReleaseGroupsReleases(ctx context.Context, releaseGroupMBzID string) ([]string, error) {
	releases := []mbz.Release{}
	paginator := paginator()

	for {
		<-m.tick
		mbRes, err := m.client.BrowseReleases(ctx, mbz.ReleaseFilter{
			ReleaseGroupMBID: mbtypes.MBID(releaseGroupMBzID),
		}, paginator)
		if err != nil {
			return nil, err
		}

		releases = append(releases, mbRes.Releases...)
		if len(releases) >= mbRes.Count {
			break
		}

		paginator.Offset += len(mbRes.Releases)
	}

	return lo.Map(releases, func(rel mbz.Release, _ int) string {
		return string(rel.ID)
	}), nil
}

func paginator() mbz.Paginator {
	return mbz.Paginator{
		Offset: 0,
		Limit:  mbz.MaxLimit,
	}
}
