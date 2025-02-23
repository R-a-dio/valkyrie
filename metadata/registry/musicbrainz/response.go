package musicbrainz

import "time"

type MusicBrainzRecording struct {
	Created    time.Time `json:"created"`
	Count      int       `json:"count"`
	Offset     int       `json:"offset"`
	Recordings []struct {
		ID           string `json:"id"`
		Score        int    `json:"score"`
		Title        string `json:"title"`
		Length       int    `json:"length,omitempty"`
		Video        any    `json:"video"`
		ArtistCredit []struct {
			Joinphrase string `json:"joinphrase"`
			Name       string `json:"name"`
			Artist     struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				SortName string `json:"sort-name"`
				Aliases  []struct {
					SortName  string `json:"sort-name"`
					TypeID    string `json:"type-id"`
					Name      string `json:"name"`
					Locale    any    `json:"locale"`
					Type      string `json:"type"`
					Primary   any    `json:"primary"`
					BeginDate any    `json:"begin-date"`
					EndDate   any    `json:"end-date"`
				} `json:"aliases"`
			} `json:"artist"`
		} `json:"artist-credit"`
		FirstReleaseDate string `json:"first-release-date"`
		Releases         []struct {
			ID           string `json:"id"`
			StatusID     string `json:"status-id"`
			Count        int    `json:"count"`
			Title        string `json:"title"`
			Status       string `json:"status"`
			ArtistCredit []struct {
				Joinphrase string `json:"joinphrase"`
				Name       string `json:"name"`
				Artist     struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					SortName string `json:"sort-name"`
				} `json:"artist"`
			} `json:"artist-credit"`
			ReleaseGroup struct {
				ID            string `json:"id"`
				TypeID        string `json:"type-id"`
				PrimaryTypeID string `json:"primary-type-id"`
				Title         string `json:"title"`
				PrimaryType   string `json:"primary-type"`
			} `json:"release-group"`
			Date          string `json:"date"`
			Country       string `json:"country"`
			ReleaseEvents []struct {
				Date string `json:"date"`
				Area struct {
					ID            string   `json:"id"`
					Name          string   `json:"name"`
					SortName      string   `json:"sort-name"`
					Iso31661Codes []string `json:"iso-3166-1-codes"`
				} `json:"area"`
			} `json:"release-events"`
			TrackCount int `json:"track-count"`
			Media      []struct {
				Position int    `json:"position"`
				Format   string `json:"format"`
				Track    []struct {
					ID     string `json:"id"`
					Number string `json:"number"`
					Title  string `json:"title"`
					Length int    `json:"length"`
				} `json:"track"`
				TrackCount  int `json:"track-count"`
				TrackOffset int `json:"track-offset"`
			} `json:"media"`
		} `json:"releases"`
		Isrcs          []string `json:"isrcs,omitempty"`
		Disambiguation string   `json:"disambiguation,omitempty"`
	} `json:"recordings"`
}

type MusicBrainzRelease struct {
	Date               string `json:"date"`
	StatusID           string `json:"status-id"`
	Packaging          string `json:"packaging"`
	TextRepresentation struct {
		Script   string `json:"script"`
		Language string `json:"language"`
	} `json:"text-representation"`
	PackagingID     string `json:"packaging-id"`
	Asin            string `json:"asin"`
	Barcode         string `json:"barcode"`
	Disambiguation  string `json:"disambiguation"`
	Quality         string `json:"quality"`
	CoverArtArchive struct {
		Darkened bool `json:"darkened"`
		Back     bool `json:"back"`
		Front    bool `json:"front"`
		Count    int  `json:"count"`
		Artwork  bool `json:"artwork"`
	} `json:"cover-art-archive"`
	ID            string `json:"id"`
	ReleaseEvents []struct {
		Date string `json:"date"`
		Area struct {
			Disambiguation string   `json:"disambiguation"`
			ID             string   `json:"id"`
			Type           any      `json:"type"`
			TypeID         any      `json:"type-id"`
			Name           string   `json:"name"`
			SortName       string   `json:"sort-name"`
			Iso31661Codes  []string `json:"iso-3166-1-codes"`
		} `json:"area"`
	} `json:"release-events"`
	Status  string `json:"status"`
	Country string `json:"country"`
	Title   string `json:"title"`
}
