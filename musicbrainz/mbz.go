package musicbrainz

import (
	"context"
	"fmt"
	"slices"
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
	<-m.tick
	mbRes, err := m.client.BrowseReleaseGroups(ctx, mbz.ReleaseGroupFilter{
		ArtistMBID: mbtypes.MBID(artistMBzID),
		Includes:   []string{"artist-credits"},
	}, mbz.DefaultPaginator())
	if err != nil {
		return nil, fmt.Errorf("mbz artist release groups: %w", err)
	}

	return lo.FilterMap(mbRes.ReleaseGroups, func(rg mbz.ReleaseGroup, _ int) (database.ReleaseGroup, bool) {
		if slices.Contains(rg.SecondaryTypes, "Compilation") {
			return database.ReleaseGroup{}, false
		}

		return database.ReleaseGroup{
			MBzID:       string(rg.ID),
			Name:        rg.Title,
			ReleaseType: rg.PrimaryType,
			ReleaseDate: rg.FirstReleaseDate.String(),

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
	<-m.tick
	mbRes, err := m.client.BrowseReleases(ctx, mbz.ReleaseFilter{
		ReleaseGroupMBID: mbtypes.MBID(releaseGroupMBzID),
	}, mbz.DefaultPaginator())
	if err != nil {
		return nil, err
	}

	return lo.Map(mbRes.Releases, func(rel mbz.Release, _ int) string {
		return string(rel.ID)
	}), nil
}
