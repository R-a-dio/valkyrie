package proxy

import (
	"context"
	"net"
	"syscall"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/rpc"
	"github.com/R-a-dio/valkyrie/storage"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/Wessie/fdstore"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// names for the fdstore listener storage, shouldn't change if possible
const fdstoreGRPCName = "proxy-grpc"
const fdstoreHTTPName = "proxy"

func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "proxy/Execute"

	// setup dependencies
	storage, err := storage.Open(ctx, cfg)
	if err != nil {
		return errors.E(op, err)
	}

	srv, err := NewServer(ctx, cfg, cfg.Manager, storage)
	if err != nil {
		return errors.E(op, err)
	}

	grpcSrv, err := NewGRPC(ctx, srv)
	if err != nil {
		return errors.E(op, err)
	}

	fdstorage := fdstore.NewStoreListenFDs()

	errCh := make(chan error, 2)
	go func() {
		errCh <- srv.Start(ctx, fdstorage)
	}()
	go func() {
		errCh <- grpcSrv.Start(ctx, cfg, fdstorage)
	}()

	select {
	case <-ctx.Done():
		grpcSrv.Stop()
		return srv.Close()
	case <-util.Signal(syscall.SIGUSR2):
		if err := srv.storeSelf(ctx, fdstorage); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store self")
		}
		if err := grpcSrv.storeSelf(ctx, fdstorage); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to store grpc")
		}
		if err := fdstorage.Send(); err != nil {
			zerolog.Ctx(ctx).Error().Ctx(ctx).Err(err).Msg("failed to send store")
		}
		return srv.Close()
	case err = <-errCh:
		grpcSrv.Stop()
		srv.Close()
		return err
	}
}

type GRPC struct {
	srv *grpc.Server
	ln  net.Listener
}

func NewGRPC(ctx context.Context, srv *Server) (*GRPC, error) {
	gs := rpc.NewGrpcServer(ctx)
	rpc.RegisterProxyServer(gs, rpc.NewProxy(srv))

	return &GRPC{
		srv: gs,
	}, nil
}

func (g *GRPC) Start(ctx context.Context, cfg config.Config, fdstorage *fdstore.Store) error {
	const op errors.Op = "proxy.GRPC.Start"

	ln, _, err := util.RestoreOrListen(fdstorage, fdstoreGRPCName, "tcp", cfg.Conf().Proxy.RPCAddr.String())
	if err != nil {
		return errors.E(op, err)
	}
	// we need to clone the listener since we use the GRPC shutdown mechanism
	// before we add the listener to the fdstore, and GRPC will close the listener
	// fd when Stop is called
	clone, err := ln.(fdstore.Filer).File()
	if err != nil {
		return errors.E(op, err)
	}

	g.ln, err = net.FileListener(clone)
	if err != nil {
		return errors.E(op, err)
	}

	err = g.srv.Serve(ln)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (g *GRPC) Stop() error {
	g.srv.Stop()
	return nil
}

func (g *GRPC) storeSelf(ctx context.Context, fdstorage *fdstore.Store) error {
	return fdstorage.AddListener(g.ln, fdstoreGRPCName, nil)
}
