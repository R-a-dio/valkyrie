package shared

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/R-a-dio/valkyrie/errors"
)

func PageCount(total, size int64) int64 {
	full := total / size
	if leftover := total % size; leftover > 0 {
		return full + 1
	}
	return full
}

func PageAndOffset(r *http.Request, pageSize int64) (int64, int64, error) {
	var page int64 = 1
	{
		rawPage := r.FormValue("page")
		if rawPage == "" {
			return page, 0, nil
		}
		parsedPage, err := strconv.ParseInt(rawPage, 10, 0)
		if err != nil {
			return page, 0, errors.E(err, errors.InvalidForm)
		}
		page = parsedPage
	}
	var offset = (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	return page, offset, nil
}

func NewPagination(currentPage, totalPages int64, uri *url.URL) *Pagination {
	return &Pagination{
		Nr:    currentPage,
		Total: totalPages,
		uri:   uri,
	}
}

type Pagination struct {
	Nr    int64
	Total int64
	uri   *url.URL
}

func (p *Pagination) URL() template.URL {
	if p == nil || p.uri == nil {
		return template.URL("")
	}

	u := *p.uri
	v := u.Query()
	v.Set("page", strconv.FormatInt(p.Nr, 10))
	u.RawQuery = v.Encode()
	return template.URL(u.RequestURI())
}

func (p *Pagination) BaseURL() template.URL {
	if p == nil || p.uri == nil {
		return template.URL("")
	}
	return template.URL(p.uri.Path)
}

func (p *Pagination) createPage(page int64) *Pagination {
	if p == nil {
		return nil
	}
	if page > p.Total {
		return nil
	}
	if page < 1 {
		return nil
	}

	return &Pagination{
		Nr:    page,
		Total: p.Total,
		uri:   p.uri,
	}
}

func (p *Pagination) First() *Pagination {
	return p.createPage(1)
}

func (p *Pagination) Next(offset int64) *Pagination {
	return p.createPage(p.Nr + offset)
}

func (p *Pagination) Prev(offset int64) *Pagination {
	return p.createPage(p.Nr - offset)
}

func (p *Pagination) Last() *Pagination {
	return p.createPage(p.Total)
}
