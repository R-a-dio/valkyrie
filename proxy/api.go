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
	return srv.proxy.RemoveSourceClient(ctx, id)
}

func (srv *Server) ListSources(ctx context.Context) ([]radio.ProxySource, error) {
	return srv.proxy.ListSources(ctx)
}
