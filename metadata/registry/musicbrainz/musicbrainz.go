package musicbrainz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	radio "github.com/R-a-dio/valkyrie"
	metadata "github.com/R-a-dio/valkyrie/metadata/registry"
)

const NAME = "musicbrainz"

type MusicBrainzResponse struct {
	Recordings []struct {
		Title  string `json:"title"`
		ID     string `json:"id"`
		Artist []struct {
			Name string `json:"name"`
		} `json:"artist-credit"`
	} `json:"recordings"`
}

func searchSong(songTitle string) (*metadata.FindResult, error) {
	baseURL := "https://musicbrainz.org/ws/2/recording/"
	queryParams := url.Values{}
	queryParams.Set("query", songTitle)
	queryParams.Set("fmt", "json")

	fullURL := fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())
	resp, err := http.Get(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result MusicBrainzResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	if len(result.Recordings) == 0 {
		return nil, errors.New("no results") // TODO(kipukun): make this an actual type probs
	}

	_ = new(metadata.FindResult)

	for _, recording := range result.Recordings {
		artistName := "Unknown Artist"
		if len(recording.Artist) > 0 {
			artistName = recording.Artist[0].Name
		}
		fmt.Printf("Title: %s, Artist: %s, ID: %s\n", recording.Title, artistName, recording.ID)
	}

	return nil, nil
}

func init() {
	metadata.Register(NAME, &musicbrainz{})
}

type musicbrainz struct {
	c *http.Client
}

func (mbz *musicbrainz) Find(ctx context.Context, auth metadata.AuthString, s radio.Song) (*metadata.FindResult, error) {
	panic("todo")
}
