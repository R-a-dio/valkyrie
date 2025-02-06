package manager

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/rs/zerolog"
)

const (
	TUNEIN_PARTNER_ID  = "partnerId"
	TUNEIN_PARTNER_KEY = "partnerKey"
	TUNEIN_STATION_ID  = "id"
)

var tuneinSplitMetadataRe = regexp.MustCompile("^((?P<artist>.*?) - )?(?P<title>.*)$")

type TuneinUpdater struct {
	cancel              context.CancelFunc
	client              *http.Client
	cfgTuneinEndpoint   func() string
	cfgTuneinPartnerID  func() string
	cfgTuneinStationID  func() string
	cfgTuneinPartnerKey func() string
}

func NewTuneinUpdater(ctx context.Context, cfg config.Config, manager radio.ManagerService, client *http.Client) (*TuneinUpdater, error) {
	tu := &TuneinUpdater{
		client: client,
		cfgTuneinEndpoint: config.Value(cfg, func(c config.Config) string {
			return cfg.Conf().Tunein.Endpoint
		}),
		cfgTuneinPartnerID: config.Value(cfg, func(cfg config.Config) string {
			return cfg.Conf().Tunein.PartnerID
		}),
		cfgTuneinStationID: config.Value(cfg, func(c config.Config) string {
			return cfg.Conf().Tunein.StationID
		}),
		cfgTuneinPartnerKey: config.Value(cfg, func(c config.Config) string {
			return cfg.Conf().Tunein.Key
		}),
	}
	ctx, tu.cancel = context.WithCancel(ctx)

	util.StreamValue(ctx, manager.CurrentSong, func(ctx context.Context, su *radio.SongUpdate) {
		log.Println("update", su)
		if su == nil {
			return
		}

		log.Println(time.Since(su.Info.Start))
		if time.Since(su.Info.Start) < time.Second*5 {
			err := tu.Update(ctx, su.Metadata)
			if err != nil {
				log.Println(err)
				zerolog.Ctx(ctx).Err(err).Ctx(ctx).Msg("failed to update tunein")
			}
		}
	})
	return tu, nil
}

func (tu *TuneinUpdater) Close() error {
	tu.cancel()
	return nil
}

// Update tries to update the tunein api by using the api endpoint documented at
// https://tunein.com/broadcasters/api/
func (tu *TuneinUpdater) Update(ctx context.Context, metadata string) error {
	const op errors.Op = "manager/TuneinUpdater.Update"

	splits := tuneinSplitMetadataRe.FindStringSubmatch(metadata)
	if len(splits) < 3 {
		return errors.E(op, errors.InvalidArgument)
	}
	artist, title := splits[2], splits[3]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tu.cfgTuneinEndpoint(), nil)
	if err != nil {
		return errors.E(op, err)
	}

	// build query parameters
	q := req.URL.Query()
	q.Set(TUNEIN_PARTNER_ID, tu.cfgTuneinPartnerID())
	q.Set(TUNEIN_STATION_ID, tu.cfgTuneinStationID())
	q.Set(TUNEIN_PARTNER_KEY, tu.cfgTuneinPartnerKey())
	q.Set("title", title)
	q.Set("artist", artist)
	req.URL.RawQuery = q.Encode()

	resp, err := tu.client.Do(req)
	if err != nil {
		return errors.E(op, err)
	}
	defer resp.Body.Close()

	// anything not 200 is an error, though the documentation doesn't really tell us what it returns
	if resp.StatusCode != 200 {
		return errors.E(op, "received non-200 status code")
	}

	return nil
}
