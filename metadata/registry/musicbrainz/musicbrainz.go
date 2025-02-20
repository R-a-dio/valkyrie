package musicbrainz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/metadata/registry"
)

const (
	NAME      = "musicbrainz"
	CV_PHRASE = "(CV: "

	// https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting
	UA = "valkyrie-metadata ( github.com/R-a-dio/valkyrie )"
)

var (
	types = []string{"front", "back"}
)

func get[T any](ctx context.Context, url string) (T, error) {
	var result T

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return result, err
	}

	req.Header.Add("User-Agent", UA)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	switch any(result).(type) {
	case []byte:
		respb, err := io.ReadAll(resp.Body)
		if err != nil {
			return result, err
		}
		return any(respb).(T), nil // this is for sure safe lamo

	default:
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

func getRelease(ctx context.Context, releaseID string) (*registry.Release, error) {
	rel := new(registry.Release)
	baseURL := "https://musicbrainz.org/ws/2/release/" + releaseID
	queryParams := url.Values{}
	queryParams.Set("fmt", "json")

	fullURL := fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())

	mbrel, err := get[MusicBrainzRelease](ctx, fullURL)
	if err != nil {
		return nil, err
	}

	rel.Date = mbrel.Date
	rel.ID = mbrel.ID
	rel.Name = mbrel.Title

	rel.Art = map[string][]byte{}

	artURL := "https://coverartarchive.org/release/" + releaseID + "/"

	for _, t := range types {
		bs, err := get[[]byte](ctx, artURL+t)
		if err != nil {
			return nil, err
		}
		rel.Art[t] = bs
	}

	return rel, nil
}

func searchSong(ctx context.Context, songTitle string) ([]*registry.FindResult, error) {
	baseURL := "https://musicbrainz.org/ws/2/recording/"
	queryParams := url.Values{}
	queryParams.Set("query", songTitle)
	queryParams.Set("fmt", "json")
	queryParams.Set("limit", "5")

	fullURL := fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())

	mbrec, err := get[MusicBrainzRecording](ctx, fullURL)
	if err != nil {
		return nil, err
	}

	if len(mbrec.Recordings) == 0 {
		return nil, errors.New("no results") // TODO(kipukun): make this an actual type probs
	}

	var frs []*registry.FindResult

	for _, recording := range mbrec.Recordings {
		fr := new(registry.FindResult)
		fr.Provider = NAME

		fr.Info.Title = recording.Title
		fr.Info.ID = recording.ID

		fr.Info.Artists = make([]string, 0)

		for _, artist := range recording.ArtistCredit {
			if artist.Joinphrase == CV_PHRASE {
				continue
			}

			fr.Info.Artists = append(fr.Info.Artists, artist.Name)

		}

		fr.Info.Releases = make([]*registry.Release, 0)
		for _, release := range recording.Releases {
			if release.Status == "Pseudo-Release" {
				continue
			}

			rel, err := getRelease(ctx, release.ID)
			if err != nil {
				return nil, err
			}

			fr.Info.Releases = append(fr.Info.Releases, rel)

		}

		frs = append(frs, fr)

	}

	return frs, nil
}

func init() {
	registry.Register(NAME, &musicbrainz{})
}

type musicbrainz struct {
	c *http.Client
}

func (mbz *musicbrainz) Find(ctx context.Context, auth registry.AuthString, s radio.Song) ([]*registry.FindResult, error) {
	return searchSong(ctx, s.Title)
}
