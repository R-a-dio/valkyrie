package admin

import (
	"bytes"
	"cmp"
	"context"
	"html/template"
	"net/http"
	"net/url"
	"slices"
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

	StreamerList   *BoothStreamerList
	ProxyStatus    *BoothProxyStatusInput
	StreamerInfo   *BoothStopStreamerInput
	ThreadInfo     *BoothSetThreadInput
	BoothStreamURL *url.URL
}

type BoothProxyStatusInput struct {
	boothInput
	Connections []radio.ProxySource
}

func (BoothProxyStatusInput) TemplateName() string {
	return "proxy-status"
}

func NewBoothInput(gs radio.GuestService, ps radio.ProxyService, r *http.Request, connectTimeout time.Duration, boothStreamURL *url.URL) (*BoothInput, error) {
	const op errors.Op = "website/admin.NewBoothInput"

	// setup the input here, we need some of the fields it populates
	input := &BoothInput{
		Input:          middleware.InputFromRequest(r),
		CSRFTokenInput: csrf.TemplateField(r),
		BoothStreamURL: boothStreamURL,
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

	input.StreamerInfo, err = NewBoothStopStreamerInput(gs, r, connectTimeout, input.Status.StreamUser)
	if err != nil {
		return nil, errors.E(op, err)
	}

	input.ThreadInfo, err = NewBoothSetThreadInput(gs, r)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return input, nil
}

func (s *State) GetBooth(w http.ResponseWriter, r *http.Request) {
	input, err := NewBoothInput(s.Guest, s.Proxy, r, s.Config.StreamerConnectTimeout(), s.Config.BoothStreamURL())
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

func NewBoothStopStreamerInput(gs radio.GuestService, r *http.Request, timeout time.Duration, currentUser *radio.User) (*BoothStopStreamerInput, error) {
	ctx := r.Context()
	var isRobot bool
	var isLive bool
	user := middleware.UserFromContext(ctx)

	// guard against no current user
	if currentUser.IsValid() {
		// check if the user currently streaming is a robot
		isRobot = currentUser.UserPermissions.HasExplicit(radio.PermRobot)
		// check if the user currently streaming is equal to our user
		isLive = currentUser.ID == user.ID
	}

	var allowedToKill = true
	if radio.IsGuest(*user) {
		var err error
		allowedToKill, err = gs.CanDo(ctx, radio.UsernameToNick(user.Username), radio.GuestKill)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to check guest can-do")
		}
	}

	return &BoothStopStreamerInput{
		CSRFTokenInput: csrf.TemplateField(r),
		AllowedToKill:  allowedToKill,
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

func (bssi *BoothStopStreamerInput) FormAction() template.HTMLAttr {
	if bssi.UserIsLive {
		return "/admin/booth/start-streamer"
	}
	return "/admin/booth/stop-streamer"
}

func (BoothStopStreamerInput) TemplateName() string {
	return "stop-streamer"
}

func (s *State) boothCheckGuestPermission(r *http.Request, action radio.GuestAction) (ok bool, err error) {
	ctx := r.Context()
	user := middleware.UserFromContext(ctx)

	if !radio.IsGuest(*user) {
		// not a guest, so we return true
		return true, nil
	}

	// TODO: prefix handling should be somewhere else
	return s.Guest.Do(ctx, radio.UsernameToNick(user.Username), action)
}

func (s *State) PostBoothStartStreamer(w http.ResponseWriter, r *http.Request) {
	input, err := s.postBoothStartStreamer(r)
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

func (s *State) postBoothStartStreamer(r *http.Request) (*BoothStopStreamerInput, error) {
	ctx := r.Context()

	input, err := NewBoothStopStreamerInput(
		s.Guest,
		r,
		s.Config.StreamerConnectTimeout(),
		middleware.InputFromRequest(r).Status.StreamUser,
	)
	if err != nil {
		return nil, err
	}

	return input, s.Streamer.Start(ctx)
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

	input, err := NewBoothStopStreamerInput(s.Guest, r, s.Config.StreamerConnectTimeout(), middleware.InputFromRequest(r).Status.StreamUser)
	if err != nil {
		return nil, err
	}

	// check for guest user and if they have permission to continue
	if ok, err := s.boothCheckGuestPermission(r, radio.GuestKill); err != nil {
		// guest service broken or offline
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed guest permission check")
		return input, nil
	} else if !ok {
		// guest doesn't have permission to do this
		input.AllowedToKill = false
		return input, nil
	}

	errCh := make(chan error, 1)
	go func() {
		zerolog.Ctx(ctx).Info().Ctx(ctx).Msg("trying to stop streamer")
		select {
		case errCh <- s.Streamer.Stop(
			context.WithoutCancel(ctx),
			middleware.UserFromContext(ctx),
			false,
		):
		case <-ctx.Done():
		}
	}()

	select {
	case err = <-errCh:
	case <-time.After(time.Second / 2):
	}

	if err != nil {
		// streamer service offline or broken
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to stop streamer")
		return input, nil
	}

	input.Success = true
	return input, nil
}

func NewBoothSetThreadInput(gs radio.GuestService, r *http.Request, thread ...string) (*BoothSetThreadInput, error) {
	ctx := r.Context()
	input := middleware.InputFromRequest(r)

	// if a thread argument was given, use that as our thread input
	var threadRes string
	if len(thread) == 0 {
		threadRes = input.Status.Thread
	} else {
		threadRes = thread[0]
	}

	var allowedToThread bool
	if su := input.Status.StreamUser; su.IsValid() {
		allowedToThread = su.ID == middleware.UserFromContext(ctx).ID
	}

	// if we're a guest check with the guest service if we're allowed to do this
	if allowedToThread && radio.IsGuest(*input.User) {
		var err error
		allowedToThread, err = gs.CanDo(ctx, radio.UsernameToNick(input.User.Username), radio.GuestThread)
		if err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to check guest can-do")
		}
	}

	return &BoothSetThreadInput{
		CSRFTokenInput:  csrf.TemplateField(r),
		Thread:          threadRes,
		AllowedToThread: allowedToThread,
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
	input, err := s.postBoothSetThread(s.Guest, r)
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

func (s *State) postBoothSetThread(gs radio.GuestService, r *http.Request) (*BoothSetThreadInput, error) {
	ctx := r.Context()

	input, err := NewBoothSetThreadInput(gs, r)
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
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed guest permission check")
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
		zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to update thread")
		return input, nil
	}

	input.Success = true
	input.Thread = thread
	return input, nil
}

func NewBoothAPI(cfg config.Config, tmpl templates.Executor) *BoothAPI {
	return &BoothAPI{
		Guest:   cfg.Guest,
		Proxy:   cfg.Proxy,
		Manager: cfg.Manager,
		tmpl:    tmpl,
		connectTimeoutCfg: config.Value(cfg, func(cfg config.Config) time.Duration {
			return time.Duration(cfg.Conf().Streamer.ConnectTimeout)
		}),
	}
}

type BoothAPI struct {
	Guest             radio.GuestService
	Proxy             radio.ProxyService
	Manager           radio.ManagerService
	tmpl              templates.Executor
	connectTimeoutCfg func() time.Duration
}

func (s *State) sseBoothAPI(w http.ResponseWriter, r *http.Request) {
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
		err := s.TemplateExecutor.Execute(&buf, r, input)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Str("event", eventName).Msg("failed to execute template")
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
	util.StreamValue(ctx, s.Manager.CurrentUser, func(ctx context.Context, user *radio.User) {
		// update the streamer view
		write("streamer", (*BoothStreamerInput)(user))

		// update the stop button state
		input, err := NewBoothStopStreamerInput(s.Guest, r, s.Config.StreamerConnectTimeout(), user)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to create stop-streamer input")
			return
		}

		write("stop-streamer", input)
	})

	// stream for the generic stream status
	util.StreamValue(ctx, s.Manager.CurrentStatus, func(ctx context.Context, status radio.Status) {
		write("status", &BoothStatusInput{Status: status})
	})

	// stream for thread updates
	util.StreamValue(ctx, s.Manager.CurrentThread, func(ctx context.Context, thread radio.Thread) {
		input, err := NewBoothSetThreadInput(s.Guest, r, thread)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to create thread input")
			return
		}

		write("thread", input)
	})

	// stream for our own personal status updates from the proxy
	util.StreamValue(ctx,
		func(ctx context.Context) (eventstream.Stream[[]radio.ProxySource], error) {
			return s.Proxy.StatusStream(ctx, me.ID)
		},
		func(ctx context.Context, conns []radio.ProxySource) {
			input := &BoothProxyStatusInput{
				Connections: conns,
			}

			write("proxy-status", input)
		},
	)

	util.StreamValue(ctx, s.Proxy.SourceStream, func(ctx context.Context, event radio.ProxySourceEvent) {
		switch event.Event {
		case radio.SourceConnect, radio.SourceDisconnect:
		default:
			return
		}

		input, err := NewBoothStreamerList(ctx, s.Proxy)
		if err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to create streamer list input")
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
			logger.Error().Ctx(ctx).Err(err).Msg("failed to write data")
			continue
		}

		// and flush, otherwise smaller events won't get send instantly
		if err = controller.Flush(); err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("failed to flush data")
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
