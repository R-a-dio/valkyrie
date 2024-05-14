package proxy

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/proxy/compat"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util/graceful"
	"github.com/rs/zerolog"
)

func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "proxy/Execute"

	// setup dependencies
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}
	m := cfg.Manager

	srv, err := NewServer(ctx, cfg, m, storage)
	if err != nil {
		return errors.E(op, err)
	}

	// check if we're a child of an existing process
	if graceful.IsChild(ctx) {
		err := srv.handleResume(ctx, cfg)
		if err != nil {
			// resuming failed, just exit and hope someone in charge restarts us
			return err
		}
		graceful.Finish(ctx)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case <-graceful.Signal(ctx):
		return srv.handleRestart(ctx, cfg)
	case err = <-errCh:
		return err
	}
}

func (srv *Server) handleResume(ctx context.Context, cfg config.Config) error {
	parent, err := graceful.Parent(ctx)
	if err != nil {
		return err
	}
	defer parent.Close()

	return srv.readSelf(ctx, cfg, parent)
}

func (srv *Server) handleRestart(ctx context.Context, cfg config.Config) error {
	dst, err := graceful.StartChild(ctx)
	if err != nil {
		return err
	}
	defer dst.Close()

	return srv.writeSelf(dst)
}

type wireServer struct {
	MasterServer string
}

func (srv *Server) writeSelf(dst *net.UnixConn) error {
	var ws wireServer
	ws.MasterServer = string(srv.cfg.Conf().Proxy.MasterServer)

	srv.listenerMu.Lock()
	fd, err := getFile(srv.listener)
	srv.listenerMu.Unlock()
	if err != nil {
		return fmt.Errorf("fd failure in server: %w", err)
	}
	defer fd.Close()

	err = graceful.WriteJSONFile(dst, ws, fd)
	if err != nil {
		return err
	}

	return srv.proxy.writeSelf(dst)
}

func (srv *Server) readSelf(ctx context.Context, cfg config.Config, src *net.UnixConn) error {
	var ws wireServer

	zerolog.Ctx(ctx).Info().Msg("resume: reading server data")
	file, err := graceful.ReadJSONFile(src, &ws)
	if err != nil {
		return err
	}
	defer file.Close()

	zerolog.Ctx(ctx).Info().Any("ws", ws).Msg("resume")

	srv.listenerMu.Lock()
	srv.listener, err = net.FileListener(file)
	srv.listenerMu.Unlock()
	if err != nil {
		return err
	}

	return srv.proxy.readSelf(ctx, cfg, src)
}

func getFile(un any) (*os.File, error) {
	if un == nil {
		return nil, errors.New("nil passed to getFile")
	}

	if f, ok := un.(interface{ File() (*os.File, error) }); ok {
		return f.File()
	}

	switch v := un.(type) {
	case *compat.Listener:
		return getFile(v.Listener)
	case *compat.Conn:
		return getFile(v.Conn)
	default:
		fmt.Printf("unknown type in getFile: %#v", un)
		panic("unknown type")
	}
}
