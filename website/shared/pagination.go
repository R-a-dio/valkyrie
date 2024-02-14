package shared

import (
	"fmt"
	"html/template"
)

func PageCount(total, size int64) int64 {
	full := total / size
	if leftover := total % size; leftover > 0 {
		return full + 1
	}
	return full
}

func NewPagination(current, total int64, urlFormat string) *Pagination {
	return &Pagination{
		Nr:     current,
		Total:  total,
		format: urlFormat,
	}
}

type Pagination struct {
	Nr     int64
	Total  int64
	format string
}

func (p *Pagination) URL() template.URL {
	return template.URL(fmt.Sprintf(p.format, p.Nr))
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
		Nr:     page,
		Total:  p.Total,
		format: p.format,
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
