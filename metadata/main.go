package metadata

import (
	"context"

	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

// Execute executes the balancer with the context ctx and config cfg.
// Execution of the balancer can be halted by cancelling ctx.
func Execute(ctx context.Context, cfg config.Config) error {
	const op errors.Op = "metadata/Execute"

	_ = cfg.Conf().Providers.Metadata

	return nil
}
