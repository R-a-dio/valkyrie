package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/R-a-dio/valkyrie/balancer"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/ircbot"
	"github.com/R-a-dio/valkyrie/jobs"
	"github.com/R-a-dio/valkyrie/manager"
	"github.com/R-a-dio/valkyrie/proxy"
	_ "github.com/R-a-dio/valkyrie/search/storage"  // storage search interface
	_ "github.com/R-a-dio/valkyrie/storage/mariadb" // mariadb storage interface
	"github.com/R-a-dio/valkyrie/telemetry"
	"github.com/R-a-dio/valkyrie/tracker"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/Wessie/fdstore"
	"github.com/google/subcommands"
	"github.com/rs/zerolog"
	"golang.org/x/term"
)

type executeFn func(context.Context, config.Loader) error

type executeConfigFn func(context.Context, config.Config) error

type cmd struct {
	name     string
	synopsis string
	usage    string
	setFlags func(*flag.FlagSet)
	execute  executeFn
}

func (c cmd) Name() string     { return c.name }
func (c cmd) Synopsis() string { return c.synopsis }
func (c cmd) Usage() string    { return c.usage }
func (c cmd) SetFlags(f *flag.FlagSet) {
	if c.setFlags != nil {
		c.setFlags(f)
	}
}
func (c cmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	// extract extra arguments from the interface slice; it's fine if we panic here
	// because that is an unrecoverable programmer error
	errCh := args[0].(chan error)

	// add the subcommand name to the logging and telemetry
	zerolog.Ctx(ctx).UpdateContext(func(zc zerolog.Context) zerolog.Context {
		return zc.Str("service", c.name)
	})

	// setup telemetry if wanted

	var telemetryShutdown func()
	defer func() {
		if telemetryShutdown != nil {
			telemetryShutdown()
		}
	}()

	loader := func() (config.Config, error) {
		cfg, err := config.LoadFile(configFile, configEnvFile)
		if err != nil {
			return cfg, err
		}

		if !cfg.Conf().Telemetry.Use { // no telemetry
			return cfg, err
		}

		// yes telemetry
		telemetryShutdown, err = telemetry.Init(ctx, cfg, flag.CommandLine.Arg(0))
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to initialize telemetry")
		}

		return cfg, err
	}

	errCh <- c.execute(ctx, loader)
	return subcommands.ExitSuccess
}

// withConfig turns an executeConfigFn into an executeFn
func withConfig(fn executeConfigFn) executeFn {
	return func(ctx context.Context, l config.Loader) error {
		cfg, err := l()
		if err != nil {
			return err
		}
		return fn(ctx, cfg)
	}
}

var versionCmd = cmd{
	name:     "version",
	synopsis: "display version information of executable",
	usage: `version:
	display version information of executable`,
	execute: printVersion,
}

func printVersion(context.Context, config.Loader) error {
	if info, ok := debug.ReadBuildInfo(); ok { // requires go version 1.12+
		fmt.Printf("%s %s\n", info.Path, info.Main.Version)
		for _, mod := range info.Deps {
			fmt.Printf("\t%s %s\n", mod.Path, mod.Version)
		}
	} else {
		fmt.Printf("%s %s\n", "valkyrie", "(devel)")
	}
	return nil
}

// configEnvFile will be resolved to the environment variable given here
var configEnvFile = "HANYUU_CONFIG"

// configFile will be filled with the -config flag value
var configFile string

// logLevel will be filled with the -loglevel flag value
var logLevel string

// useTelemetry will be filled with the -telemetry flag value
var useTelemetry bool

var configCmd = cmd{
	name:     "config",
	synopsis: "display current configuration",
	usage: `config:
	display current configuration
	`,
	execute: printConfig,
}

func printConfig(_ context.Context, l config.Loader) error {
	// try and load the configuration, but otherwise just print the defaults
	cfg, _ := l()
	return cfg.Save(os.Stdout)
}

// implements cmd for .../valkyrie/manager
var managerCmd = cmd{
	name:     "manager",
	synopsis: "manages shared state between the different parts",
	usage: `manager:
	manages shared state between the different parts
	`,
	execute: withConfig(manager.Execute),
}

// implements cmd for .../valkyrie/ircbot
var ircCmd = cmd{
	name:     "irc",
	synopsis: "run the IRC bot",
	usage: `irc:
	run the IRC bot
	`,
	execute: withConfig(ircbot.Execute),
}

var listenerLogCmd = cmd{
	name:     "listenerlog",
	synopsis: "log listener count to database",
	usage: `listenerlog:
	log listener count to database
	`,
	execute: withConfig(jobs.ExecuteListenerLog),
}

var requestCountCmd = cmd{
	name:     "requestcount",
	synopsis: "reduce request counter in database",
	usage: `requestcount:
	reduce request counter in database
	`,
	execute: withConfig(jobs.ExecuteRequestCount),
}

var websiteCmd = cmd{
	name:     "website",
	synopsis: "runs the r/a/dio website",
	usage: `website:
	runs the r/a/dio website
	`,
	execute: withConfig(website.Execute),
}

var balancerCmd = cmd{
	name:     "balancer",
	synopsis: "runs the stream load balancer",
	usage: `balancer:
	run the stream load balancer
	`,
	execute: withConfig(balancer.Execute),
}

var proxyCmd = cmd{
	name:     "proxy",
	synopsis: "runs the icecast proxy",
	usage: `proxy:
	run the icecast proxy
	`,
	execute: withConfig(proxy.Execute),
}

var listenerTrackerCmd = cmd{
	name:     "listener-tracker",
	synopsis: "runs the icecast listener tracker",
	usage: `listener-tracker:
	run the icecast listener tracker
	`,
	execute: withConfig(tracker.Execute),
}

func main() {
	// setup configuration file as top-level flag
	flag.StringVar(&configFile, "config", "hanyuu.toml", "filepath to configuration file")
	flag.StringVar(&logLevel, "loglevel", "info", "loglevel to use")
	flag.BoolVar(&useTelemetry, "telemetry", false, "to enable telemetry")

	// add all our top-level flags as important flags to subcommands
	flag.VisitAll(func(f *flag.Flag) {
		subcommands.ImportantFlag(f.Name)
	})
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(versionCmd, "")
	subcommands.Register(configCmd, "")
	// streamerCmd is registered in streamer.go to avoid mandatory inclusion, since it
	// depends on a C library (libmp3lame).
	// 		subcommands.Register(streamerCmd, "")
	subcommands.Register(managerCmd, "")
	subcommands.Register(ircCmd, "")
	subcommands.Register(websiteCmd, "")
	subcommands.Register(balancerCmd, "")
	subcommands.Register(proxyCmd, "")
	subcommands.Register(listenerTrackerCmd, "")

	subcommands.Register(listenerLogCmd, "jobs")
	subcommands.Register(requestCountCmd, "jobs")
	subcommands.Register(&databaseCmd{}, "jobs")
	// verifier job is in streamer.go for the above reason

	// subcommands.Register(elasticCmd{}, "search")
	subcommands.Register(&migrateCmd{}, "migrate")

	flag.Parse()
	configEnvFile = os.Getenv(configEnvFile)

	// exit code passed to os.Exit
	var code int
	// setup logger

	// discard logs unless we are connected to a terminal
	lo := io.Discard
	if term.IsTerminal(int(os.Stdout.Fd())) {
		lo = zerolog.ConsoleWriter{Out: os.Stdout}
	}
	logger := zerolog.New(lo).With().Timestamp().Logger()
	// change the level to what the flag told us
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		logger.Error().Err(err).Msg("failed to parse loglevel flag")
		os.Exit(1)
	}
	logger = logger.Level(level).Hook(telemetry.Hook)

	// setup root context
	ctx := context.Background()
	ctx = logger.WithContext(ctx)

	// setup our error channel, we only use this channel if a nil error is returned by
	// executeCommand; because if it is a non-nil error we know our cmd.Execute finished
	// running; otherwise we have to wait for it to finish so we know it had the chance
	// to clean up resources
	errCh := make(chan error, 1)

	// call into another function so that we can use defers
	err = executeCommand(ctx, errCh)
	if err == nil {
		// executeCommand only returns nil when a signal asked us to stop running, this
		// means the command running has already been notified to shutdown and we will
		// wait for it to return
		<-errCh
	} else if exitErr, ok := err.(ExitError); ok {
		// we've received an ExitError which indicates a (potentially) different
		// failure exit code than the default
		code = exitErr.StatusCode()
	} else {
		// normal non-nil error, we exit with the default failure exit code
		code = 1
		// print the error if it's a non-ExitError since it's probably important
		log.Println("exit error:", err)
		logger.Fatal().Err(err).Msg("exit")
	}

	os.Exit(code)
}

// executeCommand runs subcommands.Execute and handles OS signals
//
// if someone is asking us to shutdown by sending us a SIGINT executeCommand
// should (and does) return a nil error. Otherwise it should return the error
// given by subcommands.Execute
func executeCommand(ctx context.Context, errCh chan error) error {
	// setup context that is passed to the command
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGHUP)

	// we want to signal our service manager when we're ready, but for
	// easy of use we just do it after one second so not every command has
	// to implement this themselves. Most commands will also just fail early
	// and return an error if they really can't start and thus stop the timer
	// before it expires.
	readyTimer := time.AfterFunc(time.Second, func() {
		_ = fdstore.Send(nil, fdstore.Ready)
	})
	defer func() {
		_ = fdstore.Send(nil, fdstore.Stopping)
	}()

	// run our command in another goroutine so we can
	// do signal handling on the main goroutine
	go func() {
		code := subcommands.Execute(ctx, errCh)
		readyTimer.Stop()
		// send a fake error over the errCh, this is so subcommands that don't use our
		// `cmd` type don't hang the process, mostly for internal subcommands we register
		errCh <- WithStatusCode(nil, int(code))
	}()

	// handle our signals, we only exit when either the command finishes running and
	// tells us about it through errCh; or when we receive a SIGINT from outside
	for {
		var sig os.Signal

		select {
		case sig = <-signalCh:
		case err := <-errCh:
			return err
		}

		switch sig {
		case os.Interrupt:
			zerolog.Ctx(ctx).Info().Msg("SIGINT received")
			return nil
		case syscall.SIGHUP:
			zerolog.Ctx(ctx).Info().Msg("SIGHUP received: not implemented")
			// TODO: implement this
			if fdstore.Send(nil, fdstore.Reloading) == nil {
				_ = fdstore.Send(nil, fdstore.Ready)
			}
		}
	}
}

// WithStatusCode returns an ExitError with the given status code
func WithStatusCode(err error, code int) error {
	return exitError{err, code}
}

// ExitError is an error that can carry a statuscode to be passed to os.Exit;
type ExitError interface {
	error
	// StatusCode returns a status code to be passed to os.Exit
	StatusCode() int
}

type exitError struct {
	error
	code int
}

// StatusCode returns a status code to be passed to os.Exit
func (err exitError) StatusCode() int {
	return err.code
}
