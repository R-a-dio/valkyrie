package icecast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var DefaultClient = http.DefaultClient

type MetadataFunc func(ctx context.Context, metadata string) error

// MetadataURL takes an URL as passed to DialURL and creates a function
// that can be called to send metadata to icecast for that DialURL
func MetadataURL(u *url.URL, opts ...Option) MetadataFunc {
	uc, _ := url.Parse(u.String())
	mount := uc.Path

	return func(ctx context.Context, metadata string) error {
		query := url.Values{}
		query.Set("mode", "updinfo")
		query.Set("mount", mount)
		query.Set("charset", "UTF-8")
		query.Set("song", metadata)

		uc.Path = "/admin/metadata"
		uc.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uc.String(), nil)
		if err != nil {
			return fmt.Errorf("MetadataFunc: failed to create request: %w", err)
		}
		for _, opt := range opts {
			opt(req)
		}
		checkURLForAuth(uc, req)

		resp, err := DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("MetadataFunc: failed Do: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("MetadataFunc: status not ok: %w", errors.New(resp.Status))
		}

		return nil
	}
}

func Metadata(u string, opts ...Option) (MetadataFunc, error) {
	uri, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	return MetadataURL(uri), nil
}
