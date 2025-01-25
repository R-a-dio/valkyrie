package bleve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/blevesearch/bleve/v2"
	"github.com/vmihailenco/msgpack/v4"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func NewClient(uri *url.URL) *Client {
	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	uri.Path = searchPath
	searchURL := uri.String()
	uri.Path = updatePath
	updateURL := uri.String()
	uri.Path = deletePath
	deleteURL := uri.String()
	return &Client{
		searchURL: searchURL,
		deleteURL: deleteURL,
		updateURL: updateURL,
		hc:        client,
	}
}

type Client struct {
	searchURL string
	deleteURL string
	updateURL string

	hc *http.Client
}

type SearchError struct {
	Err string
}

func (se *SearchError) Error() string {
	return se.Err
}

var _ radio.SearchService = &Client{}

func (c *Client) Search(ctx context.Context, query string, limit int64, offset int64) (*radio.SearchResult, error) {
	const op errors.Op = "search/bleve.Client.Search"
	uri := c.searchURL + fmt.Sprintf("?q=%s&limit=%d&offset=%d", url.QueryEscape(query), limit, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.E(op, err)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.E(op, decodeError(resp.Body))
	}

	result := new(bleve.SearchResult)
	err = decodeResult(resp.Body, result)
	if err != nil {
		return nil, errors.E(op, err)
	}

	res, err := bleveToRadio(result)
	if err != nil {
		return nil, errors.E(op, err)
	}
	return res, nil
}

func (c *Client) Delete(ctx context.Context, tids ...radio.TrackID) error {
	const op errors.Op = "search/bleve.Client.Delete"

	var buf bytes.Buffer

	err := msgpack.NewEncoder(&buf).Encode(tids)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.deleteURL, &buf)
	if err != nil {
		return errors.E(op, err)
	}
	req.Header.Set("Content-Type", "application/msgpack")

	resp, err := c.hc.Do(req)
	if err != nil {
		return errors.E(op, err)
	}
	defer resp.Body.Close()

	return nil
}

func (c *Client) Update(ctx context.Context, songs ...radio.Song) error {
	const op errors.Op = "search/bleve.Client.Update"

	var buf bytes.Buffer

	err := msgpack.NewEncoder(&buf).Encode(songs)
	if err != nil {
		return errors.E(op, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.updateURL, &buf)
	if err != nil {
		return errors.E(op, err)
	}
	req.Header.Set("Content-Type", "application/msgpack")

	resp, err := c.hc.Do(req)
	if err != nil {
		return errors.E(op, err)
	}
	defer resp.Body.Close()

	return nil
}
