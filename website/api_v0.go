package website

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func NewAPIv0(ctx context.Context, cfg config.Config, storage radio.StorageService,
	streamer radio.StreamerService, manager radio.ManagerService) (*APIv0, error) {

	status, err := newV0Status(ctx, storage, streamer, manager)
	if err != nil {
		return nil, err
	}
	searcher, err := search.Open(cfg)
	if err != nil {
		return nil, err
	}

	api := APIv0{
		Config:   cfg,
		storage:  storage,
		streamer: streamer,
		manager:  manager,
		status:   status,
		search:   searcher,
	}
	return &api, nil
}

type APIv0 struct {
	config.Config

	search   radio.SearchService
	storage  radio.StorageService
	streamer radio.StreamerService
	manager  radio.ManagerService
	status   *v0Status
}

func (a *APIv0) Router() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.SetHeader("Content-Type", "application/json"))
	r.Method("GET", "/", a.status)
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"ping":true}`))
	})
	r.Get("/user-cooldown", a.getUserCooldown)
	r.Get("/news", a.getNews)
	r.Get("/search/{query}", a.getSearch)
	r.Get("/can-request", a.getCanRequest)
	r.Get("/dj-image", a.getDJImage)
	// these are deprecated
	r.Get("/song", a.getSong)
	r.Get("/metadata", a.getMetadata)
	return r
}

func (a *APIv0) getSong(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(410), 410)
}

func (a *APIv0) getMetadata(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(410), 410)
}

func (a *APIv0) getUserCooldown(w http.ResponseWriter, r *http.Request) {

}

func (a *APIv0) getNews(w http.ResponseWriter, r *http.Request) {
}
func (a *APIv0) getSearch(w http.ResponseWriter, r *http.Request) {
}

func (a *APIv0) getSearch(w http.ResponseWriter, r *http.Request) {
	// parse the query string for page and limit settings
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		// TODO: look at error handling
		log.Println(err)
		return
	}

	var limit = 20
	{
		rawLimit := values.Get("limit")
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err == nil && parsedLimit < 20 {
			// TODO: check if we just want to throw a fit if NaN
			// only use the value if it's a number and it's
			// not above the allowed limit
			limit = parsedLimit
		}
	}
	var page = 1
	{
		rawPage := values.Get("page")
		parsedPage, err := strconv.Atoi(rawPage)
		if err == nil {
			// TODO: check if we just want to throw a fit if NaN
			// only use the value if it's a valid number
			page = parsedPage
		}
	}
	var offset = (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	ctx := r.Context()
	// key from the url router, query is part of the url
	query := chi.URLParamFromCtx(ctx, "query")
	result, err := a.search.Search(ctx, query, limit, offset)
	if err != nil {
		// TODO: look at error handling
		log.Println(err)
		return
	}

	// temporary until we fix search api to return pagination help
	songs := result
	// create pagination information for the result
	var response = searchResponse{
		PerPage:     limit,
		CurrentPage: page,
	}

	// move over the results to sanitized output structs
	response.Results = make([]searchResponseItem, len(songs))
	for i := range songs {
		response.Results[i].fromSong(songs[i])
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		// TODO: look at error handling
		log.Println(err)
		return
	}
}

type searchResponse struct {
	Total       int `json:"total"`
	PerPage     int `json:"per_page"`
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
	From        int `json:"from"`
	To          int `json:"to"`

	Results []searchResponseItem `json:"data"`
}

type searchResponseItem struct {
	Artist        string        `json:"artist"`
	Title         string        `json:"title"`
	TrackID       radio.TrackID `json:"id"`
	LastPlayed    int64         `json:"lastplayed"`
	LastRequested int64         `json:"lastrequested"`
	Requestable   bool          `json:"requestable"`
}

// fromSong copies relevant fields from the song given to the response item
func (sri *searchResponseItem) fromSong(s radio.Song) error {
	if !s.HasTrack() {
		// TODO: look at error handling
		return errors.New("Song without track found in search API")
	}
	sri.Artist = s.Artist
	sri.Title = s.Title
	sri.TrackID = s.TrackID
	if s.LastPlayed.IsZero() {
		sri.LastPlayed = 0
	} else {
		sri.LastPlayed = s.LastPlayed.Unix()
	}
	if s.LastRequested.IsZero() {
		sri.LastRequested = 0
	} else {
		sri.LastRequested = s.LastRequested.Unix()
	}
	sri.Requestable = s.Requestable()
	return nil
}

	}

	// send our response when we return
	defer func() {
		// but not if an error occured
		if err != nil {
			// TODO: handle error
			http.Error(w, http.StatusText(501), 501)
			return
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			log.Println(err)
		}
	}()

	// all requests are disabled
	if !status.RequestsEnabled {
		return
	}

	identifier := getIdentifier(r)
	userLastRequest, err := a.storage.Request(r.Context()).LastRequest(identifier)
	if err != nil {
		return
	}

	_, ok := radio.CanUserRequest(
		time.Duration(a.Conf().UserRequestDelay),
		userLastRequest,
	)
	if !ok {
		return
	}

	response.Main.Requests = true
	return
}

type canRequestResponse struct {
	Main struct {
		Requests bool `json:"requests"`
	}
}

func (a *APIv0) getDJImage(w http.ResponseWriter, r *http.Request) {
}

// postRequest handles /request in legacy PHP format
func (a *APIv0) postRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	response := map[string]string{}

	defer func() {
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			log.Println(err)
		}
	}()

	song, ok := ctx.Value(TrackKey).(radio.Song)
	if !ok {
		response["error"] = "invalid parameter"
		return
	}

	identifier := getIdentifier(r)
	err := a.streamer.RequestSong(ctx, song, identifier)
	if err == nil {
		response["success"] = "Thank you for making your request!"
		return
	}

	switch {
	case errors.Is(errors.SongCooldown, err):
		response["error"] = "That song is still on cooldown, You'll have to wait longer to request it."
	case errors.Is(errors.UserCooldown, err):
		response["error"] = "You recently requested a song. You have to wait longer until you can request again."
	case errors.Is(errors.StreamerNoRequests, err):
		response["error"] = "Requests are disabled currently."
	default:
		log.Println(err)
		response["error"] = "something broke, report to IRC."
	}
}

type requestResponse map[string]string

// getIdentifier returns a unique identifier for the user, currently uses the remote
// address for this purpose
func getIdentifier(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// constant used by the net package
		const missingPort = "missing port in address"
		aerr, ok := err.(*net.AddrError)
		if ok && aerr.Err == missingPort {
			return r.RemoteAddr
		}

		panic("getIdentifier: " + err.Error())
	}

	return host
}

func newV0Status(ctx context.Context, storage radio.SongStorageService,
	streamer radio.StreamerService, manager radio.ManagerService) (*v0Status, error) {

	s := v0Status{
		songs:            storage,
		streamer:         streamer,
		manager:          manager,
		updatePeriod:     time.Second * 2,
		longUpdatePeriod: time.Second * 10,
	}

	// initialize the atomic.Value
	s.storeCache(v0StatusJSON{})
	// run a periodic updater
	go s.runUpdate(ctx)
	// but also call update to get an initial value before we return
	return &s, s.updateStatusJSON(ctx)
}

// v0Status implements the root of the /api endpoint
type v0Status struct {
	// song storage to get last played songs
	songs radio.SongStorageService
	// streamer for queue contents
	streamer radio.StreamerService
	// manager for overall stream status
	manager radio.ManagerService

	updatePeriod     time.Duration
	longUpdatePeriod time.Duration
	// cache contains a v0StatusJSON
	cache atomic.Value
}

type v0StatusJSON struct {
	Main v0StatusMain `json:"main"`
	// field to determine when we created the contents of LastPlayed and Queue
	ListCreatedOn time.Time `json:"-"`
}

type v0StatusMain struct {
	NowPlaying   string `json:"np"`
	Listeners    int    `json:"listeners"`
	BitRate      int    `json:"bitrate"`
	IsAFKStream  bool   `json:"isafkstream"`
	IsStreamDesk bool   `json:"isstreamdesk"`

	CurrentTime int64 `json:"current"`
	StartTime   int64 `json:"start_time"`
	EndTime     int64 `json:"end_time"`

	LastSet    string `json:"lastset"`
	TrackID    int    `json:"trackid"`
	Thread     string `json:"thread"`
	Requesting bool   `json:"requesting"`

	DJName string     `json:"djname"`
	DJ     v0StatusDJ `json:"dj"`

	Queue      []v0StatusListEntry `json:"queue"`
	LastPlayed []v0StatusListEntry `json:"lp"`
}

type v0StatusDJ struct {
	ID          int    `json:"id" db:"djid"`
	Name        string `json:"djname" db:"djname"`
	Description string `json:"djtext" db:"djtext"`
	Image       string `json:"djimage" db:"djimage"`
	Color       string `json:"djcolor" db:"djcolor"`
	Visible     bool   `json:"visible" db:"visible"`
	Priority    int    `json:"priority" db:"priority"`
	ThemeCSS    string `json:"css" db:"css"`
	ThemeID     int    `json:"theme_id" db:"theme_id"`
	Role        string `json:"role" db:"role"`
}

type v0StatusListEntry struct {
	Metadata  string `json:"meta" db:"meta"`
	Time      string `json:"time"`
	Type      int    `json:"type" db:"type"`
	Timestamp int64  `json:"timestamp" db:"time"`
}

func (s *v0Status) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	status := s.loadCache()
	status.Main.CurrentTime = time.Now().Unix()

	h := rw.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Access-Control-Allow-Origin", "*")

	e := json.NewEncoder(rw)
	e.SetEscapeHTML(false)
	err := e.Encode(status)
	if err != nil {
		log.Printf("json encoding error: %s", err)
	}
}

func (s *v0Status) loadCache() v0StatusJSON {
	return s.cache.Load().(v0StatusJSON)
}

func (s *v0Status) storeCache(ss v0StatusJSON) {
	s.cache.Store(ss)
}

const timeagoFormat = `<time class="timeago" datetime="2006-01-02T15:04:05-0700">15:04:05</time>`

// createStatusJSON creates a new populated v0StatusJSON, if an error occurs it returns
// the previous v0StatusJSON that was stored in the cache
//
// Additionally, the Queue and LastPlayed fields are only updated if a period of length
// LongUpdatePeriod has passed, otherwise uses the contents of the previous status
func (s *v0Status) createStatusJSON(ctx context.Context) (v0StatusJSON, error) {
	var now = time.Now()
	var status v0StatusJSON

	last := s.loadCache()
	queue := last.Main.Queue
	lastplayed := last.Main.LastPlayed

	// see if we need to update the queue and lastplayed values
	if last.ListCreatedOn.IsZero() ||
		now.Sub(last.ListCreatedOn) < s.longUpdatePeriod {

		q, err := s.streamer.Queue(ctx)
		if err != nil {
			return last, err
		}

		if len(q) > 5 {
			q = q[:5]
		}

		queue = make([]v0StatusListEntry, len(q))
		for i, entry := range q {
			queue[i].Metadata = entry.Song.Metadata
			queue[i].Time = entry.ExpectedStartTime.Format(timeagoFormat)
			queue[i].Timestamp = entry.ExpectedStartTime.Unix()
			if entry.IsUserRequest {
				queue[i].Type = 1
			}
		}

		lp, err := s.songs.Song(ctx).LastPlayed(0, 5)
		if err != nil {
			return last, err
		}

		lastplayed = make([]v0StatusListEntry, len(lp))
		for i, song := range lp {
			lastplayed[i].Metadata = song.Metadata
			lastplayed[i].Time = song.LastPlayed.Format(timeagoFormat)
			lastplayed[i].Timestamp = song.LastPlayed.Unix()
		}

		// record when we created these values, so we know when to refresh again
		status.ListCreatedOn = now
	}

	ms, err := s.manager.Status(ctx)
	if err != nil {
		return last, err
	}

	// End might be the zero time, in which case calling Unix
	// returns a large negative number that we don't want
	var endTime int64
	if !ms.SongInfo.End.IsZero() {
		endTime = ms.SongInfo.End.Unix()
	}
	// Song might not have a track associated with it, so we
	// have to check for that first, before reading the TrackID
	var trackID int
	if ms.Song.HasTrack() {
		trackID = int(ms.Song.TrackID)
	}

	dj := ms.User.DJ
	status.Main = v0StatusMain{
		NowPlaying:  ms.Song.Metadata,
		Listeners:   ms.Listeners,
		IsAFKStream: ms.User.Username == "AFK",
		StartTime:   ms.SongInfo.Start.Unix(),
		EndTime:     endTime,
		LastSet:     now.Format("2006-01-02 15:04:05"),
		TrackID:     trackID,
		Thread:      ms.Thread,
		// TODO(wessie): use RequestsEnabled again when it is implemented properly,
		// right now nothing sets it and the streamer ignores the value too, only
		// reading the configuration file instead
		Requesting: ms.User.Username == "AFK",
		// Requesting:  ms.RequestsEnabled,
		DJName: dj.Name,
		DJ: v0StatusDJ{
			ID:          int(dj.ID),
			Name:        dj.Name,
			Description: dj.Text,
			Image:       dj.Image,
			Color:       dj.Color,
			Visible:     dj.Visible,
			Priority:    dj.Priority,
			ThemeCSS:    dj.CSS,
			ThemeID:     int(dj.Theme.ID),
			Role:        dj.Role,
		},
		Queue:      queue,
		LastPlayed: lastplayed,
	}

	return status, nil
}

func (s *v0Status) updateStatusJSON(ctx context.Context) error {
	ss, err := s.createStatusJSON(ctx)
	if err != nil {
		return err
	}

	s.storeCache(ss)
	return nil
}

func (s *v0Status) runUpdate(ctx context.Context) {
	ticker := time.NewTicker(s.updatePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		err := s.updateStatusJSON(ctx)
		if err != nil {
			log.Printf("status: update error: %s", err)
		}
	}
}
