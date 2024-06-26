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
)

func NewClient(uri *url.URL) *Client {
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
		hc:        &http.Client{},
	}
}

type Client struct {
	searchURL string
	deleteURL string
	updateURL string

	hc *http.Client
}

var _ radio.SearchService = &Client{}

func (c *Client) Search(ctx context.Context, query string, limit int64, offset int64) (*radio.SearchResult, error) {
	const op errors.Op = "search/bleve.Client.Search"
	uri := c.searchURL + fmt.Sprintf("?q=%s&limit=%d&offset=%d", url.QueryEscape(query), limit, offset)

	resp, err := c.hc.Get(uri)
	if err != nil {
		return nil, errors.E(op, err)
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

	resp, err := c.hc.Post(c.deleteURL, "application/msgpack", &buf)
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
		return err
	}

	resp, err := c.hc.Post(c.updateURL, "application/msgpack", &buf)
	if err != nil {
		return errors.E(op, err)
	}
	defer resp.Body.Close()

	return nil
}
