package shared

import (
	"html/template"
	"net/url"
	"strconv"
)

func PageCount(total, size int64) int64 {
	full := total / size
	if leftover := total % size; leftover > 0 {
		return full + 1
	}
	return full
}

func NewPagination(current, total int64, uri *url.URL) *Pagination {
	return &Pagination{
		Nr:    current,
		Total: total,
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
