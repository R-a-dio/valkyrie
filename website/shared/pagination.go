package shared

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/R-a-dio/valkyrie/errors"
	"golang.org/x/exp/constraints"
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

func (p *Pagination) RawURL() *url.URL {
	if p == nil {
		return nil
	}
	return p.uri
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

func NewFromPagination[T constraints.Unsigned](key T, prev, next []T, uri *url.URL) *FromPagination[T] {
	// construct a boundaries slices from the prev, key and next arguments
	var boundaries []T
	boundaries = append(boundaries, prev...)
	index := len(boundaries) // record the index of where we are putting the key
	boundaries = append(boundaries, key)
	boundaries = append(boundaries, next...)

	return &FromPagination[T]{
		Key:        key,
		index:      index,
		boundaries: boundaries,
		uri:        uri,
	}
}

type FromPagination[T constraints.Unsigned] struct {
	Key T
	Nr  int

	index      int
	boundaries []T
	uri        *url.URL

	// fields to implement Last()
	last   *T
	lastNr int
}

const FromTimeFormat = "2006-01-02T15-04-05"

func (p *FromPagination[T]) URL() template.URL {
	if p == nil || p.uri == nil {
		return template.URL("")
	}

	u := *p.uri
	v := u.Query()
	v.Set("from", strconv.FormatUint(uint64(p.Key), 10))
	v.Set("page", strconv.FormatInt(int64(p.Nr), 10))
	u.RawQuery = v.Encode()
	return template.URL(u.RequestURI())
}

func (p *FromPagination[T]) BaseURL() template.URL {
	if p == nil || p.uri == nil {
		return template.URL("")
	}
	return template.URL(p.uri.Path)
}

func (p *FromPagination[T]) RawURL() *url.URL {
	if p == nil {
		return nil
	}
	return p.uri
}

// First returns the first page, this uses time.Now() as the Key and
// 1 as the page number.
func (p *FromPagination[T]) First() *FromPagination[T] {
	if p == nil {
		return nil
	}

	return &FromPagination[T]{
		Key:    maxOf[T](),
		Nr:     1,
		uri:    p.uri,
		last:   p.last,
		lastNr: p.lastNr,
	}
}

// Next returns the next page as indicated by the offset from current
func (p *FromPagination[T]) Next(offset int) *FromPagination[T] {
	if p == nil {
		return nil
	}

	index := p.index + offset
	if index >= len(p.boundaries) || index < 0 {
		// index out of range after applying offset, return no page
		return nil
	}

	return &FromPagination[T]{
		Key:        p.boundaries[index],
		Nr:         p.Nr + offset,
		index:      index,
		boundaries: p.boundaries,
		uri:        p.uri,
		last:       p.last,
		lastNr:     p.lastNr,
	}
}

// Prev returns the previous page as indicated by the offset from current
func (p *FromPagination[T]) Prev(offset int) *FromPagination[T] {
	if p == nil {
		return nil
	}

	index := p.index - offset
	if index < 0 || index >= len(p.boundaries) {
		// index out of range after applying offset, return no page
		return nil
	}

	return &FromPagination[T]{
		Key:        p.boundaries[index],
		Nr:         p.Nr - offset,
		index:      index,
		boundaries: p.boundaries,
		uri:        p.uri,
		last:       p.last,
		lastNr:     p.lastNr,
	}
}

// WithPage returns a page with the page number set to nr
func (p *FromPagination[T]) WithPage(nr int) *FromPagination[T] {
	if p == nil {
		return nil
	}

	n := *p
	n.Nr = nr
	return &n
}

// WithLast returns a page with the last page info set to key and nr
func (p *FromPagination[T]) WithLast(key T, nr int) *FromPagination[T] {
	if p == nil {
		return nil
	}

	n := *p
	n.last = &key
	n.lastNr = nr
	return &n
}

// Last returns the last page, will return nil if WithLast wasn't called beforehand
// on a parent
func (p *FromPagination[T]) Last() *FromPagination[T] {
	if p.last == nil {
		return nil
	}
	// we don't have information on what the last page is
	return &FromPagination[T]{
		Key:  *p.last,
		Nr:   p.lastNr,
		uri:  p.uri,
		last: p.last,
	}
}

func sizeOf[T constraints.Integer]() uint {
	x := uint16(1 << 8)
	y := uint32(2 << 16)
	z := uint64(4 << 32)
	return 1 + uint(T(x))>>8 + uint(T(y))>>16 + uint(T(z))>>32
}

func minOf[T constraints.Integer]() T {
	if ones := ^T(0); ones < 0 {
		return ones << (8*sizeOf[T]() - 1)
	}
	return 0
}

func maxOf[T constraints.Integer]() T {
	ones := ^T(0)
	if ones < 0 {
		return ones ^ (ones << (8*sizeOf[T]() - 1))
	}
	return ones
}
