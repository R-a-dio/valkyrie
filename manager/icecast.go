package manager

import (
	"context"
	"encoding/json"
	"net/http"
)

type IcecastStatus struct {
	Icestats struct {
		Admin              string `json:"admin"`
		Host               string `json:"host"`
		Location           string `json:"location"`
		ServerID           string `json:"server_id"`
		ServerStart        string `json:"server_start"`
		ServerStartIso8601 string `json:"server_start_iso8601"`
		Source             []struct {
			AudioInfo          string `json:"audio_info,omitempty"`
			Bitrate            int    `json:"bitrate,omitempty"`
			Channels           int    `json:"channels,omitempty"`
			Genre              string `json:"genre"`
			ListenerPeak       int    `json:"listener_peak"`
			Listeners          int    `json:"listeners"`
			Listenurl          string `json:"listenurl"`
			Samplerate         int    `json:"samplerate,omitempty"`
			ServerDescription  string `json:"server_description"`
			ServerName         string `json:"server_name"`
			ServerType         string `json:"server_type"`
			ServerURL          string `json:"server_url,omitempty"`
			StreamStart        string `json:"stream_start"`
			StreamStartIso8601 string `json:"stream_start_iso8601"`
			Title              string `json:"title"`
		} `json:"source"`
	} `json:"icestats"`
}

// FetchIcecastStatus fetches the status page of an icecast server.
// This requires an icecast server version equal to or higher than 2.4
func FetchIcecastStatus(ctx context.Context, uri string) (IcecastStatus, error) {
	var is IcecastStatus

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return is, err
	}
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return is, err
	}

	err = json.NewDecoder(resp.Body).Decode(&is)
	return is, err
}

type RelayInfo struct {
	Online    *bool `json:"online"`
	Primary   bool  `json:"primary"`
	Disabled  bool  `json:"disabled"`
	Noredir   bool  `json:"noredir"`
	Listeners int   `json:"listeners"`
	Max       int   `json:"max"`
	Priority  int   `json:"priority"`
	Links     struct {
		Status string `json:"status"`
		Stream string `json:"stream"`
	} `json:"links"`
}

type RelayStatus struct {
	Relays    map[string]RelayInfo `json:"relays"`
	Listeners int                  `json:"listeners"`
	StreamURL string               `json:"stream_url"`
}

// FetchRelayStatus fetches the status page of the load balancer
func FetchRelayStatus(ctx context.Context, uri string) (RelayStatus, error) {
	var rs RelayStatus

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return rs, err
	}
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rs, err
	}

	err = json.NewDecoder(resp.Body).Decode(&rs)
	return rs, err
}
