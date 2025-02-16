package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"
	"time"

	. "github.com/R-a-dio/valkyrie/cmd"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/ircbot"
	"github.com/R-a-dio/valkyrie/jobs"
	"github.com/R-a-dio/valkyrie/manager"
	"github.com/R-a-dio/valkyrie/proxy"
	"github.com/R-a-dio/valkyrie/search/bleve"
	"github.com/R-a-dio/valkyrie/streamer"
	"github.com/R-a-dio/valkyrie/telemetry"
	"github.com/R-a-dio/valkyrie/telemetry/otelzerolog"
	"github.com/R-a-dio/valkyrie/tracker"
	"github.com/R-a-dio/valkyrie/util/buildinfo"
	"github.com/R-a-dio/valkyrie/website"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

const (
	flagTelemetry = "telemetry"
	flagConfig    = "config"
)

func main() {
	// global flags
	var configFile string
	var logLevel string
	var disableStdout bool
	var enableTelemetry bool

	root := &cobra.Command{
		Use:   "hanyuu",
		Short: "collection of services, helpers and one-off jobs in one executable",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if disableStdout {
				cmd.SetOut(io.Discard)
			}

			// setup logging
			err := NewLogger(cmd, cmd.OutOrStdout(), logLevel)
			if err != nil {
				return err
			}
			return nil
		},
		Version: buildinfo.GitRef + " (" + buildinfo.GitMod + ")",
	}

	// add global flags
	root.PersistentFlags().StringVar(&configFile, flagConfig, "hanyuu.toml", "filepath to configuration file")
	root.PersistentFlags().BoolVar(&enableTelemetry, flagTelemetry, false, "enable telemetry collection")
	root.PersistentFlags().StringVar(&logLevel, "log", "info", "what log level to use")
	root.PersistentFlags().BoolVarP(&disableStdout, "quiet", "q", false, "set to disable logs being printed to stdout")

	root.AddGroup(
		&cobra.Group{ID: "services", Title: "long-running services"},
		&cobra.Group{ID: "jobs", Title: "one-off jobs"},
		&cobra.Group{ID: "helpers", Title: "helper functions"},
	)

	// generic management commands
	root.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "prints a verbose versions list",
			Long:  "prints all module versions",
			Args:  cobra.NoArgs,
			Run:   versionVerbose,
		},
		&cobra.Command{
			Use:   "config",
			Short: "prints the current configuration",
			Args:  cobra.NoArgs,
			RunE: SimpleCommand(func(cmd *cobra.Command, args []string) error {
				return cfgFromContext(cmd.Context()).Save(cmd.OutOrStdout())
			}),
		},
	)

	// service commands
	root.AddCommand(
		&cobra.Command{
			Use:     "manager",
			GroupID: "services",
			Short:   "run the manager, inter-process state management",
			Args:    cobra.NoArgs,
			RunE:    Command(manager.Execute),
		},
		&cobra.Command{
			Use:     "irc",
			GroupID: "services",
			Short:   "run the IRC bot",
			Args:    cobra.NoArgs,
			RunE:    Command(ircbot.Execute),
		},
		&cobra.Command{
			Use:     "website",
			GroupID: "services",
			Short:   "run the website",
			Args:    cobra.NoArgs,
			RunE:    Command(website.Execute),
		},
		&cobra.Command{
			Use:     "proxy",
			GroupID: "services",
			Short:   "run the icecast proxy",
			Args:    cobra.NoArgs,
			RunE:    Command(proxy.Execute),
		},
		&cobra.Command{
			Use:     "telemetry-proxy",
			GroupID: "services",
			Short:   "run the telemetry reverse-proxy as a standalone process",
			Args:    cobra.NoArgs,
			RunE:    Command(telemetry.ExecuteStandalone),
		},
		&cobra.Command{
			Use:     "listener-tracker",
			GroupID: "services",
			Short:   "run the icecast listener tracker",
			Args:    cobra.NoArgs,
			RunE:    Command(tracker.Execute),
		},
		&cobra.Command{
			Use:     "blevesearch",
			GroupID: "services",
			Short:   "run the bleve search provider",
			Args:    cobra.NoArgs,
			RunE:    Command(bleve.Execute),
		},
		&cobra.Command{
			Use:     "streamer",
			GroupID: "services",
			Short:   "run the AFK streamer",
			Args:    cobra.NoArgs,
			RunE:    Command(streamer.Execute),
		},
	)

	// one-off jobs
	root.AddCommand(
		&cobra.Command{
			Use:     "requestcount",
			GroupID: "jobs",
			Short:   "reduce request counter in the database",
			Args:    cobra.NoArgs,
			RunE:    Command(jobs.ExecuteRequestCount),
		},
		&cobra.Command{
			Use:     "check-tracks",
			GroupID: "jobs",
			Short:   "checks the tracks table for mismatching hashes, and fixes them",
			Args:    cobra.NoArgs,
			RunE:    Command(jobs.ExecuteTracksHash),
		},
		&cobra.Command{
			Use:     "index-search",
			GroupID: "jobs",
			Short:   "re-index the search index from storage",
			Args:    cobra.NoArgs,
			RunE:    Command(jobs.ExecuteIndexSearch),
		},
		&cobra.Command{
			Use:     "listenerlog",
			GroupID: "jobs",
			Short:   "log listener count to the database",
			Args:    cobra.NoArgs,
			RunE:    Command(jobs.ExecuteListenerLog),
		},
		&cobra.Command{
			Use:     "verifier",
			GroupID: "jobs",
			Short:   "verifies that tracks marked unusable can be decoded with ffmpeg",
			Args:    cobra.NoArgs,
			RunE:    Command(jobs.ExecuteVerifier),
		},
	)

	// subcommands
	root.AddCommand(
		DatabaseCommand(),
		MigrationCommand(),
	)

	cmd, err := root.ExecuteC()
	if err != nil {
		zerolog.Ctx(cmd.Context()).Fatal().Err(err).Msg("fatal error occured")
	}
}

// constructServiceName constructs a name from the command, it does this by
// walking upwards and collecting their .Name() joined by periods. That is
// if we run `hanyuu migrate up` the name will come out as `migrate.up`
func constructServiceName(cmd *cobra.Command) string {
	var names []string
	for cmd != nil {
		names = append(names, cmd.Name())
		cmd = cmd.Parent()
	}

	slices.Reverse(names)
	if len(names) > 1 {
		// remove the 'hanyuu' if a subcommand was called
		names = names[1:]
	}
	return strings.Join(names, ".")
}

type configKey struct{}

func cfgFromContext(ctx context.Context) config.Config {
	return ctx.Value(configKey{}).(config.Config)
}

func cfgWithContext(ctx context.Context, cfg config.Config) context.Context {
	return context.WithValue(ctx, configKey{}, cfg)
}

// cobraFn is the function cobra expects in its Command.RunE, only defined
// here for readability concerns
type cobraFn func(cmd *cobra.Command, args []string) error

// convertExecuteFn converts an ExecuteFn to a cobraFn
func convertExecuteFn(fn ExecuteFn) cobraFn {
	return func(cmd *cobra.Command, args []string) error {
		return fn(cmd.Context(), cfgFromContext(cmd.Context()))
	}
}

// SimpleCommand is the wrapper to use when your function is already a cobraFn
func SimpleCommand(fn cobraFn) cobraFn {
	return executeCommand(fn)
}

// Command is the wrapper to use when your function is an ExecuteFn and does
// not require smooth-restart support (let us handle SIGUSR2)
func Command(fn ExecuteFn) cobraFn {
	return executeCommand(convertExecuteFn(fn))
}

// executeCommand sets up the environment and executes the function given with it, it
// handles OS signals, config loading, telemetry setup and systemd notifications
func executeCommand(fn cobraFn) cobraFn {
	return func(cmd *cobra.Command, args []string) error {
		// make a ctx we can cancel
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		// load the configuration file
		configFile, _ := cmd.PersistentFlags().GetString(flagConfig)
		cfg, err := config.LoadFile(configFile, os.Getenv("HANYUU_CONFIG"))
		if err != nil {
			return err
		}
		ctx = cfgWithContext(ctx, cfg)

		// setup telemetry if this is wanted
		if enable, _ := cmd.PersistentFlags().GetBool(flagTelemetry); enable || cfg.Conf().Telemetry.Use {
			var telemetryShutdown func()
			ctx, telemetryShutdown, err = SetupTelemetry(ctx, cfg, constructServiceName(cmd))
			if err != nil {
				zerolog.Ctx(cmd.Context()).Err(err).Ctx(cmd.Context()).Msg("failed to initialize telemetry")
			}
			if telemetryShutdown != nil {
				defer telemetryShutdown()
			}
		}

		// run the OS signal handler
		ctx = signalHandler(ctx)

		// add the updated ctx to the cmd
		cmd.SetContext(ctx)

		// when we exit, try and send this knowledge to systemd
		defer func() {
			_ = fdstore.Notify(fdstore.Stopping)
			_ = fdstore.WaitBarrier(time.Second)
		}()

		// we need to signal systemd that we're ready, doing this "correctly" would mean doing
		// it in each separate commands main loop, but we just do it here for now
		if err := fdstore.Notify(fdstore.Ready); err != nil && !errors.Is(err, fdstore.ErrNoSocket) {
			zerolog.Ctx(ctx).Err(err).Msg("failed to send sd_notify READY")
		}

		err = fn(cmd, args)
		if err != nil && !errors.Is(err, context.Canceled) {
			// return the original error if it isn't a context canceled
			return err
		}

		return nil
	}
}

func signalHandler(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	// put in a way for the commands to tell us to not react to USR2
	ctx, USR2WasUsed := WithUSR2Signal(ctx)

	go func() {
		defer cancel()
		// setup signal handling
		signalCh := make(chan os.Signal, 5)
		signal.Notify(signalCh, os.Interrupt, syscall.SIGHUP, syscall.SIGUSR2)

		// now handle OS signals
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-signalCh:
				switch sig {
				case os.Interrupt:
					// exit when we get an interrupt
					zerolog.Ctx(ctx).Info().Msg("SIGINT received")
					return
				case syscall.SIGHUP:
					// reload the configuration file, NOT IMPLEMENTED
					zerolog.Ctx(ctx).Info().Msg("SIGHUP received: not implemented")
					// notify systemd that we reloaded correctly even though it isn't implemented,
					// otherwise it will hang forever waiting for us to signal it
					if fdstore.Notify(fdstore.Reloading) == nil {
						_ = fdstore.Notify(fdstore.Ready)
					}
				case syscall.SIGUSR2:
					// only react to SIGUSR2 if the ExecuteFn isn't handling it
					if !USR2WasUsed() {
						zerolog.Ctx(ctx).Info().Msg("SIGUSR2 received")
						return
					}
				}
			}
		}
	}()
	return ctx
}

func versionVerbose(cmd *cobra.Command, args []string) {
	if info, ok := debug.ReadBuildInfo(); ok { // requires go version 1.12+
		cmd.Printf("%s %s\n", info.Path, buildinfo.Version)
		for _, mod := range info.Deps {
			cmd.Printf("\t%s %s\n", mod.Path, mod.Version)
		}
	} else {
		cmd.Printf("%s %s\n", "valkyrie", "(devel)")
	}
}

func NewLogger(cmd *cobra.Command, out io.Writer, level string) error {
	zlevel, err := zerolog.ParseLevel(level)
	if err != nil {
		return err
	}

	lo := zerolog.ConsoleWriter{Out: out}
	logger := zerolog.New(lo).
		Level(zlevel).With().
		Timestamp().
		Str("service.name", constructServiceName(cmd)).
		Str("service.version", buildinfo.ShortRef).
		Logger()

	cmd.SetContext(logger.WithContext(cmd.Context()))
	return nil
}

func SetupTelemetry(ctx context.Context, cfg config.Config, serviceName string) (context.Context, func(), error) {
	logger := zerolog.Ctx(ctx).Hook(otelzerolog.Hook(
		buildinfo.InstrumentationName,
		buildinfo.InstrumentationVersion,
	))

	ctx = logger.WithContext(ctx)
	shutdownFn, err := telemetry.Init(ctx, cfg, serviceName)
	return ctx, shutdownFn, err
}
