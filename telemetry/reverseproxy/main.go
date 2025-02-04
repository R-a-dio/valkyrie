package reverseproxy

import (
	"context"
	"net/http/httputil"
	"net/url"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	vmiddleware "github.com/R-a-dio/valkyrie/website/middleware"
)

func New(ctx context.Context, cfg config.Config) *httputil.ReverseProxy {
	AdminMonitoringURL := config.Value(cfg, func(cfg config.Config) *url.URL {
		return cfg.Conf().Website.AdminMonitoringURL.URL()
	})
	AdminMonitoringUserHeader := config.Value(cfg, func(cfg config.Config) string {
		return cfg.Conf().Website.AdminMonitoringUserHeader
	})
	AdminMonitoringRoleHeader := config.Value(cfg, func(cfg config.Config) string {
		return cfg.Conf().Website.AdminMonitoringRoleHeader
	})

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(AdminMonitoringURL())
			pr.SetXForwarded()
			pr.Out.Host = pr.In.Host

			u := vmiddleware.UserFromContext(pr.In.Context())
			pr.Out.Header.Add(AdminMonitoringUserHeader(), u.Username)
			if u.UserPermissions.Has(radio.PermAdmin) {
				pr.Out.Header.Add(AdminMonitoringRoleHeader(), "Admin")
			}
		},
	}
}
