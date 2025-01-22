package proxy

import (
	"context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/util/eventstream"
)

func (srv *Server) MetadataStream(ctx context.Context) (eventstream.Stream[radio.ProxyMetadataEvent], error) {
	return srv.events.metaStream.SubStream(ctx), nil
}

func (srv *Server) SourceStream(ctx context.Context) (eventstream.Stream[radio.ProxySourceEvent], error) {
	return srv.events.sourceStream.SubStream(ctx), nil
}

func (srv *Server) KickSource(ctx context.Context, id radio.SourceID) error {
	// make the ctx not have a cancel, this is a very specific behavior to this API
	// because it spins off a goroutine that also gets the ctx, but once we return
	// from this function the ctx gets canceled and everything in said goroutine fails
	//
	// also this function call is basically non-reverseable so having it be cancelable by
	// the ctx doesn't make sense.
	ctx = context.WithoutCancel(ctx)
	return srv.proxy.RemoveSourceClient(ctx, id)
}

func (srv *Server) ListSources(ctx context.Context) ([]radio.ProxySource, error) {
	return srv.proxy.ListSources(ctx)
}

func (srv *Server) StatusStream(ctx context.Context, id radio.UserID) (eventstream.Stream[[]radio.ProxySource], error) {
	return srv.events.status.newUserStream(ctx, id), nil
}
