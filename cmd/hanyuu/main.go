package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/ircbot"
	"github.com/R-a-dio/valkyrie/jobs"
	"github.com/R-a-dio/valkyrie/manager"
	"github.com/google/subcommands"
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

	loader := func() (config.Config, error) {
		return config.LoadFile(configFile, configEnvFile)
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

var verifierCmd = cmd{
	name:     "verifier",
	synopsis: "verifies that tracks marked unusable can be decoded with ffmpeg",
	usage: `verifier:
	verifies that all tracks marked with usable=0 can be decoded with ffmpeg
	and marks them with usable=1 if it succeeds
	`,
	execute: withConfig(jobs.ExecuteVerifier),
}

func main() {
	// setup configuration file as top-level flag
	flag.StringVar(&configFile, "config", "hanyuu.toml", "filepath to configuration file")
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

	subcommands.Register(listenerLogCmd, "jobs")
	subcommands.Register(requestCountCmd, "jobs")
	subcommands.Register(verifierCmd, "jobs")

	subcommands.Register(elasticCmd{}, "search")

	flag.Parse()
	configEnvFile = os.Getenv(configEnvFile)

	// exit code passed to os.Exit
	var code int
	// setup root context
	ctx := context.Background()
	// setup our error channel, we only use this channel if a nil error is returned by
	// executeCommand; because if it is a non-nil error we know our cmd.Execute finished
	// running; otherwise we have to wait for it to finish so we know it had the chance
	// to clean up resources
	errCh := make(chan error, 1)

	// call into another function so that we can use defers
	err := executeCommand(ctx, errCh)
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
		fmt.Println(err)
	}

	os.Exit(code)
}

// executeCommand runs subcommands.Execute and handles OS signals
//
// if someone is asking us to shutdown by sending us a SIGINT executeCommand
// should (and does) return a nil error. Otherwise it should return the error given by
// subcommands.Execute
func executeCommand(ctx context.Context, errCh chan error) error {
	// setup context that is passed to the command
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalCh := make(chan os.Signal, 2)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGHUP)

	// run our command in another goroutine so we can
	// do signal handling on the main goroutine
	go func() {
		code := subcommands.Execute(ctx, errCh)
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
			log.Printf("SIGINT received")
			return nil
		case syscall.SIGHUP:
			log.Printf("SIGHUP received: not implemented")
			// TODO: implement this
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
