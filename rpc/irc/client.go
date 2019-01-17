package irc

import (
	context "context"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/rpc/manager"
)

// NewAnnounceService returns a new AnnounceService
func NewAnnounceService(addr string, httpclient HTTPClient) radio.AnnounceService {
	return AnnounceService{
		twirp: NewBotProtobufClient(addr, httpclient),
	}
}

// AnnounceService wraps irc.Bot to implement radio.AnnouncerService
type AnnounceService struct {
	twirp Bot
}

// AnnounceSong implements radio.AnnounceService
func (c AnnounceService) AnnounceSong(s radio.Song) error {
	// TODO: implement song passing; current api server doesn't touch the argument so
	// we can get away with passing nothing
	_, err := c.twirp.AnnounceSong(context.TODO(), new(manager.Song))
	return err
}

// AnnounceRequest implements radio.AnnounceService
func (c AnnounceService) AnnounceRequest(s radio.Song) error {
	return nil
}
