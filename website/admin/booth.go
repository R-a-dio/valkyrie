package admin

import (
	"bytes"
	"cmp"
	"context"
	"html/template"
	"net/http"
	"slices"
	"strings"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/templates"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/R-a-dio/valkyrie/util/sse"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/gorilla/csrf"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type boothInput struct{}

func (boothInput) TemplateBundle() string {
	return "booth"
}

type BoothStreamerList struct {
	boothInput
	Streamers []radio.ProxySource
}

func (BoothStreamerList) TemplateName() string {
	return "proxy-streamers"
}

func NewBoothStreamerList(ctx context.Context, ps radio.ProxyService) (*BoothStreamerList, error) {
	const op errors.Op = "website/admin.NewBoothStreamerList"

	s, err := ps.ListSources(ctx)
	if err != nil {
		return nil, errors.E(op, err)
	}
	s = filterAndSortProxySources(s)
	return &BoothStreamerList{Streamers: s}, nil
}

type BoothInput struct {
	middleware.Input
	boothInput
	CSRFTokenInput template.HTML

	StreamerList *BoothStreamerList
	ProxyStatus  *BoothProxyStatusInput
	StreamerInfo *BoothStopStreamerInput
	ThreadInfo   *BoothSetThreadInput
}

type BoothProxyStatusInput struct {
	boothInput
	Connections []radio.ProxySource
}

func (BoothProxyStatusInput) TemplateName() string {
	return "proxy-status"
}

func NewBoothInput(ps radio.ProxyService, r *http.Request, connectTimeout time.Duration) (*BoothInput, error) {
	const op errors.Op = "website/admin.NewBoothInput"

	// setup the input here, we need some of the fields it populates
	input := &BoothInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
	}

	connections, err := util.OneOff(r.Context(), func(ctx context.Context) (eventstream.Stream[[]radio.ProxySource], error) {
		return ps.StatusStream(ctx, input.User.ID)
	})
	if err != nil {
		return nil, errors.E(op, err)
	}

	input.StreamerList, err = NewBoothStreamerList(r.Context(), ps)
	if err != nil {
		return nil, errors.E(op, err)
	}

	input.ProxyStatus = &BoothProxyStatusInput{
		Connections: connections,
	}

	input.StreamerInfo, err = NewBoothStopStreamerInput(r, connectTimeout, input.Status.StreamUser)
	if err != nil {
		return nil, errors.E(op, err)
	}

	input.ThreadInfo, err = NewBoothSetThreadInput(r)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return input, nil
}

func (s *State) GetBooth(w http.ResponseWriter, r *http.Request) {
	input, err := NewBoothInput(s.Proxy, r, time.Duration(s.Conf().Streamer.ConnectTimeout))
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func filterAndSortProxySources(sources []radio.ProxySource) []radio.ProxySource {
	var sm []radio.ProxySource
	for _, source := range sources {
		// TODO: don't use a literal string for main mountpoint
		if source.MountName == "/main.mp3" {
			sm = append(sm, source)
		}
	}
	// sort them by their priority
	slices.SortStableFunc(sm, func(a, b radio.ProxySource) int {
		return cmp.Compare(a.Priority, b.Priority)
	})
	return sm
}
func proxySourceListToMap(sources []radio.ProxySource) map[string][]radio.ProxySource {
	// generate a mapping of mountname to sources
	sm := make(map[string][]radio.ProxySource, 3)
	for _, source := range sources {
		sm[source.MountName] = append(sm[source.MountName], source)
	}

	// then sort the sources by their priority value
	for _, sources := range sm {
		slices.SortStableFunc(sources, func(a, b radio.ProxySource) int {
			return cmp.Compare(a.Priority, b.Priority)
		})
	}
	return sm
}

func NewBoothStopStreamerInput(r *http.Request, timeout time.Duration, currentUser *radio.User) (*BoothStopStreamerInput, error) {
	var isRobot bool
	var isLive bool
	user := middleware.UserFromContext(r.Context())

	// guard against no current user
	if currentUser.IsValid() {
		// check if the user currently streaming is a robot
		isRobot = currentUser.UserPermissions.HasExplicit(radio.PermRobot)
		// check if the user currently streaming is equal to our user
		isLive = currentUser.ID == user.ID
	}

	return &BoothStopStreamerInput{
		CSRFTokenInput: csrf.TemplateField(r),
		AllowedToKill:  true,
		Success:        false,
		CurrentIsRobot: isRobot,
		UserIsLive:     isLive,
		ConnectTimeout: timeout,
	}, nil
}

type BoothStopStreamerInput struct {
	boothInput
	CSRFTokenInput template.HTML

	// UserIsLive is true if the user is currently live on the main mountpoint
	UserIsLive bool
	// AllowedToKill is true if the user is allowed to use the kill button
	AllowedToKill bool
	// Success is true if the Stop command succeeded
	Success bool
	// CurrentIsRobot is true if the current live user is a robot (Hanyuu-sama)
	// if this is false the kill button should be disabled
	CurrentIsRobot bool

	// ConnectTimeout is how long the AFK streamer will wait before connecting again
	// after being kicked
	ConnectTimeout time.Duration
}

func (BoothStopStreamerInput) FormAction() template.HTMLAttr {
	return "/admin/booth/stop-streamer"
}

func (BoothStopStreamerInput) TemplateName() string {
	return "stop-streamer"
}

func (s *State) boothCheckGuestPermission(r *http.Request, action radio.GuestAction) (ok bool, err error) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)

	if !user.UserPermissions.HasExplicit(radio.PermGuest) {
		// not a guest, so we return true
		return true, nil
	}

	// TODO: prefix handling should be somewhere else
	return s.Guest.Do(ctx, strings.TrimPrefix(user.Username, "guest_"), action)
}

func (s *State) PostBoothStopStreamer(w http.ResponseWriter, r *http.Request) {
	input, err := s.postBoothStopStreamer(r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func (s *State) postBoothStopStreamer(r *http.Request) (*BoothStopStreamerInput, error) {
	ctx := r.Context()

	input, err := NewBoothStopStreamerInput(r, time.Duration(s.Conf().Streamer.ConnectTimeout), middleware.InputFromRequest(r).Status.StreamUser)
	if err != nil {
		return nil, err
	}

	// check for guest user and if they have permission to continue
	if ok, err := s.boothCheckGuestPermission(r, radio.GuestKill); err != nil {
		// guest service broken or offline
		zerolog.Ctx(ctx).Error().Err(err).Msg("failed guest permission check")
		return input, nil
	} else if !ok {
		// guest doesn't have permission to do this
		input.AllowedToKill = false
		return input, nil
	}

	errCh := make(chan error, 1)
	go func() {
		select {
		case errCh <- s.Streamer.Stop(ctx, false):
		case <-ctx.Done():
		}
	}()

	select {
	case err = <-errCh:
	case <-time.After(time.Second / 2):
	}

	if err != nil {
		// streamer service offline or broken
		zerolog.Ctx(ctx).Error().Err(err).Msg("failed to stop streamer")
		return input, nil
	}

	input.Success = true
	return input, nil
}

func NewBoothSetThreadInput(r *http.Request, thread ...string) (*BoothSetThreadInput, error) {
	input := middleware.InputFromRequest(r)
	var threadRes string
	if len(thread) == 0 {
		threadRes = input.Status.Thread
	} else {
		threadRes = thread[0]
	}

	var AllowedToThread bool
	if su := input.Status.StreamUser; su.IsValid() {
		AllowedToThread = su.ID == middleware.UserFromContext(r.Context()).ID
	}

	return &BoothSetThreadInput{
		CSRFTokenInput:  csrf.TemplateField(r),
		Thread:          threadRes,
		AllowedToThread: AllowedToThread,
		Success:         false,
	}, nil
}

type BoothSetThreadInput struct {
	boothInput
	CSRFTokenInput template.HTML

	Thread string
	// AllowedToThread is true if the user is allowed to update the thread
	AllowedToThread bool
	// Success is true if the UpdateThread succeeded
	Success bool
}

func (BoothSetThreadInput) FormAction() template.HTMLAttr {
	return "/admin/booth/set-thread"
}

func (BoothSetThreadInput) TemplateName() string {
	return "set-thread"
}

func (s *State) PostBoothSetThread(w http.ResponseWriter, r *http.Request) {
	input, err := s.postBoothSetThread(r)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}

	err = s.TemplateExecutor.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err, "")
		return
	}
}

func (s *State) postBoothSetThread(r *http.Request) (*BoothSetThreadInput, error) {
	ctx := r.Context()

	input, err := NewBoothSetThreadInput(r)
	if err != nil {
		return nil, err
	}

	// check if the user trying to change it is currently streaming
	if !input.AllowedToThread {
		return input, nil
	}

	// check for guest user and if they have permission to continue
	if ok, err := s.boothCheckGuestPermission(r, radio.GuestThread); err != nil {
		// guest service broken or offline
		zerolog.Ctx(ctx).Error().Err(err).Msg("failed guest permission check")
		return input, nil
	} else if !ok {
		// guest doesn't have permission to do this
		input.AllowedToThread = false
		return input, nil
	}

	thread := r.FormValue("thread")

	err = s.Manager.UpdateThread(ctx, thread)
	if err != nil {
		// manager service broken or offline
		zerolog.Ctx(ctx).Error().Err(err).Msg("failed to update thread")
		return input, nil
	}

	input.Success = true
	input.Thread = thread
	return input, nil
}

func NewBoothAPI(cfg config.Config, tmpl templates.Executor) *BoothAPI {
	return &BoothAPI{
		Proxy:   cfg.Proxy,
		Manager: cfg.Manager,
		tmpl:    tmpl,
		connectTimeoutCfg: config.Value(cfg, func(c config.Config) time.Duration {
			return time.Duration(c.Conf().Streamer.ConnectTimeout)
		}),
	}
}

type BoothAPI struct {
	Proxy             radio.ProxyService
	Manager           radio.ManagerService
	tmpl              templates.Executor
	connectTimeoutCfg func() time.Duration
}

func (b *BoothAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := hlog.FromRequest(r)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	controller := http.NewResponseController(w)
	me := middleware.UserFromContext(ctx)

	// setup SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeCh := make(chan []byte, 16)
	// function to execute a template and send the data on our channel
	write := func(eventName string, input templates.TemplateSelectable) {
		var buf bytes.Buffer
		// execute our template
		err := b.tmpl.Execute(&buf, r, input)
		if err != nil {
			logger.Error().Err(err).Str("event", eventName).Msg("failed to execute template")
			return
		}

		// create an SSE event out of it
		data := sse.Event{Name: eventName, Data: buf.Bytes()}.Encode()

		select {
		case writeCh <- data:
		case <-ctx.Done():
		}
	}

	// stream for who is currently streaming
	util.StreamValue(ctx, b.Manager.CurrentUser, func(ctx context.Context, user *radio.User) {
		// update the streamer view
		write("streamer", (*BoothStreamerInput)(user))

		// update the stop button state
		input, err := NewBoothStopStreamerInput(r, b.connectTimeoutCfg(), user)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create stop-streamer input")
			return
		}

		write("stop-streamer", input)
	})

	// stream for the generic stream status
	util.StreamValue(ctx, b.Manager.CurrentStatus, func(ctx context.Context, status radio.Status) {
		write("status", &BoothStatusInput{Status: status})
	})

	// stream for thread updates
	util.StreamValue(ctx, b.Manager.CurrentThread, func(ctx context.Context, thread radio.Thread) {
		input, err := NewBoothSetThreadInput(r, thread)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create thread input")
			return
		}

		write("thread", input)
	})

	// stream for our own personal status updates from the proxy
	util.StreamValue(ctx,
		func(ctx context.Context) (eventstream.Stream[[]radio.ProxySource], error) {
			return b.Proxy.StatusStream(ctx, me.ID)
		},
		func(ctx context.Context, conns []radio.ProxySource) {
			input := &BoothProxyStatusInput{
				Connections: conns,
			}

			write("proxy-status", input)
		},
	)

	util.StreamValue(ctx, b.Proxy.SourceStream, func(ctx context.Context, event radio.ProxySourceEvent) {
		switch event.Event {
		case radio.SourceConnect, radio.SourceDisconnect:
		default:
			return
		}

		input, err := NewBoothStreamerList(ctx, b.Proxy)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create streamer list input")
			return
		}

		write("proxy-streamers", input)
	})

	// now keep the request alive with a ticker and a ping send to the client every 10 seconds
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		var data []byte
		select {
		case <-ticker.C:
			data = sse.Event{Name: "ping"}.Encode()
		case data = <-writeCh:
		case <-ctx.Done():
			return
		}

		// send any data we receive from the streams
		_, err := w.Write(data)
		if err != nil {
			logger.Error().Err(err).Msg("failed to write data")
			continue
		}

		// and flush, otherwise smaller events won't get send instantly
		if err = controller.Flush(); err != nil {
			logger.Error().Err(err).Msg("failed to flush data")
			continue
		}
	}
}

type BoothStreamerInput radio.User

func (*BoothStreamerInput) TemplateBundle() string {
	return "booth"
}

func (*BoothStreamerInput) TemplateName() string {
	return "streamer"
}

type BoothStatusInput struct {
	boothInput
	radio.Status
}

func (BoothStatusInput) TemplateName() string {
	return "status"
}
