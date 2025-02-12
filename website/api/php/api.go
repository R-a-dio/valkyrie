package php

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/search"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/R-a-dio/valkyrie/website/shared"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/go-chi/chi/v5"
	chiware "github.com/go-chi/chi/v5/middleware"
)

func NewAPI(ctx context.Context, cfg config.Config, storage radio.StorageService,
	newsCache *shared.NewsCache, statusValue util.StreamValuer[radio.Status]) (*API, error) {

	status, err := newV0Status(ctx, storage, cfg.Queue, statusValue)
	if err != nil {
		return nil, err
	}
	searcher, err := search.Open(ctx, cfg)
	if err != nil {
		return nil, err
	}

	api := API{
		cfgUserUploadDelay: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().UserUploadDelay)
		}),
		cfgUserRequestDelay: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().UserRequestDelay)
		}),
		cfgDJImagePath: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().Website.DJImagePath
		}),
		storage:     storage,
		streamer:    cfg.Streamer,
		newsCache:   newsCache,
		status:      status,
		search:      searcher,
		StatusValue: statusValue,
	}
	return &api, nil
}

type API struct {
	cfgUserRequestDelay func() time.Duration
	cfgUserUploadDelay  func() time.Duration
	cfgDJImagePath      func() string
	search              radio.SearchService
	storage             radio.StorageService
	streamer            radio.StreamerService
	newsCache           *shared.NewsCache
	status              *v0Status

	StatusValue util.StreamValuer[radio.Status]
}

func (a *API) Route(r chi.Router) {
	r.Use(chiware.SetHeader("Content-Type", "application/json"))
	r.Method(http.MethodGet, "/", a.status)
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ping":true}`))
	})
	r.Get("/user-cooldown", a.getUserCooldown)
	r.Get("/news", a.getNews)
	r.Get("/search/{query}", a.getSearch)
	r.Get("/can-request", a.getCanRequest)
	// should be static-images only
	r.With(middleware.UserByDJIDCtx(a.storage)).
		Get("/dj-image/{DJID}-*", a.getDJImage)
	r.With(middleware.UserByDJIDCtx(a.storage)).
		Get("/dj-image/{DJID:[0-9]+}", a.getDJImage)
	// these are deprecated
	r.Get("/song", a.getSong)
	r.Get("/metadata", a.getMetadata)
}

func (a *API) getSong(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusGone), http.StatusGone)
}

func (a *API) getMetadata(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusGone), http.StatusGone)
}

func (a *API) getUserCooldown(w http.ResponseWriter, r *http.Request) {
	identifier := r.RemoteAddr

	submissionTime, err := a.storage.Submissions(r.Context()).LastSubmissionTime(identifier)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}

	_, ok := radio.CalculateCooldown(
		a.cfgUserUploadDelay(),
		submissionTime,
	)

	response := userCooldownResponse{
		Cooldown: submissionTime.Unix(),
		Now:      time.Now().Unix(),
		Delay:    int64(a.cfgUserUploadDelay() / time.Second),
	}

	if ok {
		response.Message = "You can upload a song!"
	} else {
		response.Message = "You cannot upload another song just yet. You can upload " +
			submissionTime.Add(a.cfgUserUploadDelay()).Format(timeagoFormat)
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Any("value", response).Msg("")
		return
	}
}

type userCooldownResponse struct {
	// time of last upload
	Cooldown int64 `json:"cooldown"`
	// current time
	Now int64 `json:"now"`
	// configured cooldown in seconds
	Delay int64 `json:"delay"`
	// message to the user
	Message string `json:"message"`
}

func (a *API) getNews(w http.ResponseWriter, r *http.Request) {
	result, err := a.storage.News(r.Context()).ListPublic(3, 0)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}

	// copy the entries to sanitized output struct
	entries := result.Entries
	var response = make([]newsResponse, 0, len(entries))

	for _, e := range entries {
		header, err := a.newsCache.RenderHeader(e)
		if err != nil {
			hlog.FromRequest(r).Err(err).Ctx(r.Context()).Msg("failed to render news header")
		}

		body, err := a.newsCache.RenderBody(e)
		if err != nil {
			hlog.FromRequest(r).Err(err).Ctx(r.Context()).Msg("failed to render news body")
		}

		nr := newsResponse{
			Title:     e.Title,
			Header:    string(header.Output),
			Body:      string(body.Output),
			UpdatedAt: e.CreatedAt.Format("2006-01-02 15:04:05"),
			Author: newsAuthorResponse{
				ID:   e.User.ID,
				User: e.User.Username,
			},
		}

		if e.UpdatedAt != nil {
			nr.UpdatedAt = e.UpdatedAt.Format("2006-01-02 15:04:05")
		}
		response = append(response, nr)
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Any("value", response).Msg("")
		return
	}
}

type newsResponse struct {
	Title     string             `json:"title"`
	Header    string             `json:"header"`
	Body      string             `json:"text"`
	UpdatedAt string             `json:"updated_at"`
	Author    newsAuthorResponse `json:"author"`
}

type newsAuthorResponse struct {
	ID   radio.UserID `json:"id"`
	User string       `json:"user"`
}

func (a *API) getSearch(w http.ResponseWriter, r *http.Request) {
	// parse the query string for page and limit settings
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}

	var limit = 20
	{
		rawLimit := values.Get("limit")
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err == nil && parsedLimit < 20 {
			// if used defined limit isn't a number just return the standard 20
			limit = parsedLimit
		}
	}
	var page = 1
	{
		rawPage := values.Get("page")
		parsedPage, err := strconv.Atoi(rawPage)
		if err == nil {
			// if it's not a number just return the first page
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
	result, err := a.search.Search(ctx, query, int64(limit), int64(offset))
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}

	songs := result.Songs
	// create pagination information for the result
	var response = searchResponse{
		Total:       result.TotalHits,
		PerPage:     limit,
		CurrentPage: page,
		LastPage:    result.TotalHits/limit + 1,
		From:        offset + 1,
		To:          offset + len(songs),
	}

	// move over the results to sanitized output structs
	response.Results = make([]searchResponseItem, len(songs))
	for i := range songs {
		response.Results[i].fromSong(songs[i])
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Any("value", response).Msg("")
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
		panic("song without database track found in search api")
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

func (a *API) getCanRequest(w http.ResponseWriter, r *http.Request) {
	status := a.StatusValue.Latest()

	response := canRequestResponse{}

	// send our response when we return
	var err error
	defer func() {
		// but not if an error occured
		if err != nil {
			hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
			http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
			return
		}
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Any("value", response).Msg("")
			return
		}
	}()

	// all requests are disabled
	if !radio.IsRobot(status.User) {
		return
	}

	identifier := r.RemoteAddr
	userLastRequest, err := a.storage.Request(r.Context()).LastRequest(identifier)
	if err != nil {
		return
	}

	_, ok := radio.CalculateCooldown(
		a.cfgUserRequestDelay(),
		userLastRequest,
	)
	if !ok {
		return
	}

	response.Main.Requests = true
}

type canRequestResponse struct {
	Main struct {
		Requests bool `json:"requests"`
	}
}

func (a *API) getDJImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	w.Header().Del("Content-Type")
	w.Header().Set("Content-Type", "image/png")

	user, ok := middleware.GetUser(ctx)
	if !ok {
		panic("missing UserByDJIDCtx middleware")
	}

	filename := filepath.Join(a.cfgDJImagePath(), user.DJ.ID.String())

	f, err := os.Open(filename)
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		return
	}

	http.ServeContent(w, r, user.DJ.Image, fi.ModTime(), f)
}

// RequestRoute is the router setup for handling requests
func (a *API) RequestRoute(r chi.Router) {
	r.Use(middleware.TrackCtx(a.storage))
	r.Post("/", a.postRequest)
}

// postRequest handles /request in legacy PHP format
func (a *API) postRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	response := map[string]string{}

	defer func() {
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Any("value", response).Msg("")
			return
		}
	}()

	song, ok := middleware.GetTrack(ctx)
	if !ok {
		response["error"] = "invalid parameter"
		return
	}

	err := a.streamer.RequestSong(ctx, song, r.RemoteAddr)
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
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Msg("")
		response["error"] = "something broke, report to IRC."
	}
	hlog.FromRequest(r).Info().Ctx(r.Context()).Err(err).Msg("")
}

func newV0Status(ctx context.Context, storage radio.SongStorageService,
	queue radio.QueueService, status util.StreamValuer[radio.Status]) (*v0Status, error) {

	s := v0Status{
		songs:            storage,
		queue:            queue,
		status:           status,
		updatePeriod:     time.Second * 2,
		longUpdatePeriod: time.Second * 10,
	}

	// initialize the atomic.Value
	s.storeCache(v0StatusJSON{})
	// run a periodic updater
	go s.runUpdate(ctx)
	// but also call update to get an initial value before we return
	err := s.updateStatusJSON(ctx)
	if err != nil {
		// this should be a temporary error so we just ignore it, do log it
		// for debuggability if something does break horribly
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("")
	}
	return &s, nil
}

// v0Status implements the root of the /api endpoint
type v0Status struct {
	// song storage to get last played songs
	songs radio.SongStorageService
	// queue for queue contents
	queue radio.QueueService
	// status value
	status util.StreamValuer[radio.Status]

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
	Listeners    int64  `json:"listeners"`
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

	Tags []string `json:"tags"`
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
		hlog.FromRequest(r).Error().Ctx(r.Context()).Err(err).Any("value", status).Msg("")
		return
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
	const op errors.Op = "website/api/php.v0Status.createStatusJSON"
	ctx, span := otel.Tracer("api/v0").Start(ctx, string(op), trace.WithNewRoot())
	defer span.End()

	var now = time.Now()
	var status v0StatusJSON

	last := s.loadCache()
	queue := last.Main.Queue
	lastplayed := last.Main.LastPlayed

	// see if we need to update the queue and lastplayed values
	if last.ListCreatedOn.IsZero() || now.Sub(last.ListCreatedOn) < s.longUpdatePeriod {

		q, err := s.queue.Entries(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Ctx(ctx).Msg("failed to retrieve queue")
		}

		q = q.Limit(5)

		queue = make([]v0StatusListEntry, len(q))
		for i, entry := range q {
			queue[i].Metadata = entry.Song.Metadata
			queue[i].Time = entry.ExpectedStartTime.Format(timeagoFormat)
			queue[i].Timestamp = entry.ExpectedStartTime.Unix()
			if entry.IsUserRequest {
				queue[i].Type = 1
			}
		}

		lp, err := s.songs.Song(ctx).LastPlayed(radio.LPKeyLast, 5)
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

	ms := s.status.Latest()

	// End might be the zero time, in which case calling Unix
	// returns a large negative number that we don't want
	var endTime int64
	if !ms.SongInfo.End.IsZero() {
		endTime = ms.SongInfo.End.Unix()
	}
	// Song might not have a track associated with it, so we
	// have to check for that first, before reading the TrackID
	var trackID int
	var tags []string
	if ms.Song.HasTrack() {
		trackID = int(ms.Song.TrackID)
		tags = strings.Split(ms.Song.Tags, " ")
		if len(tags) == 0 {
			// make sure it's an empty slice, so it outputs a []
			// and not a null
			tags = []string{}
		}
	}

	// Thread seems to be a literal "none" if no thread is supposed to be shown in
	// the old API
	thread := ms.Thread
	if ms.Thread == "" {
		thread = "none"
	}

	dj := ms.User.DJ
	djName := dj.Name
	if radio.IsGuest(ms.User) {
		djName = "guest:" + djName // see https://github.com/R-a-dio/valkyrie/issues/228
	}

	status.Main = v0StatusMain{
		NowPlaying:  ms.Song.Metadata,
		Listeners:   ms.Listeners,
		IsAFKStream: radio.IsRobot(ms.User),
		StartTime:   ms.SongInfo.Start.Unix(),
		EndTime:     endTime,
		LastSet:     now.Format("2006-01-02 15:04:05"),
		TrackID:     trackID,
		Thread:      thread,
		Requesting:  radio.IsRobot(ms.User),
		DJName:      djName,
		DJ: v0StatusDJ{
			ID:          int(dj.ID),
			Name:        dj.Name,
			Description: dj.Text,
			Image:       dj.Image,
			Color:       dj.Color,
			Visible:     dj.Visible,
			Priority:    dj.Priority,
			ThemeCSS:    dj.CSS,
			ThemeID:     0, // See https://github.com/R-a-dio/valkyrie/issues/175
			Role:        dj.Role,
		},
		Queue:      queue,
		LastPlayed: lastplayed,
		Tags:       tags,
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
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("update")
		}
	}
}
