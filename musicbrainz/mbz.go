package musicbrainz

import (
	"context"
	"fmt"
	"net/http"
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

	coverArtClient  *http.Client
	coverArtBaseURL string
}

// Option customizes a MusicBrainz client.
type Option func(*MusicBrainz)

// WithBaseURL sets the MusicBrainz web service base URL.
func WithBaseURL(url string) Option {
	return func(m *MusicBrainz) { m.client.SetBaseURL(url) }
}

// WithCoverArtBaseURL sets the Cover Art Archive base URL.
func WithCoverArtBaseURL(url string) Option {
	return func(m *MusicBrainz) { m.coverArtBaseURL = url }
}

// WithRateLimit sets the minimum interval between MusicBrainz requests.
func WithRateLimit(interval time.Duration) Option {
	return func(m *MusicBrainz) { m.tick = time.Tick(interval) }
}

func New(opts ...Option) *MusicBrainz {
	mbc := mbz.NewClient(mbz.AppInfo{
		Name:    "Musikat",
		Version: "0.0.1",
		URL:     "https://github.com/ShimmerGlass/musikat",
	})

	m := &MusicBrainz{
		client: mbc,
		tick:   time.Tick(time.Second),

		coverArtClient:  newCoverArtClient(),
		coverArtBaseURL: defaultCoverArtBaseURL,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
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
			MBzID:            string(rg.ID),
			Name:             rg.Title,
			PrimaryType:      rg.PrimaryType,
			XXSecondaryTypes: strings.Join(rg.SecondaryTypes, ","),
			ReleaseDate:      rg.FirstReleaseDate.String(),

			Artists: lo.Map(rg.ArtistCredit, func(artist mbz.ArtistCreditEntry, _ int) database.Artist {
				return database.Artist{
					MBzID: string(artist.Artist.ID),
					Name:  artist.Artist.Name,
				}
			}),
		}, true
	}), nil
}

func (m *MusicBrainz) ReleaseGroupsReleases(ctx context.Context, releaseGroupMBzID string) ([]mbz.Release, error) {
	releases := []mbz.Release{}
	paginator := paginator()

	for {
		<-m.tick
		mbRes, err := m.client.BrowseReleases(ctx, mbz.ReleaseFilter{
			ReleaseGroupMBID: mbtypes.MBID(releaseGroupMBzID),
			Includes:         []string{"recordings"},
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

	return releases, nil
}

func paginator() mbz.Paginator {
	return mbz.Paginator{
		Offset: 0,
		Limit:  mbz.MaxLimit,
	}
}
