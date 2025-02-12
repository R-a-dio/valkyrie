package metadata

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
)

type Updater struct {
	providers []Provider
}

func NewUpdater(ctx context.Context, cfg config.Config) (*Updater, error)

func (upd *Updater) start(ctx context.Context) error
