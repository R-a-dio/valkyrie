package icecast

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

var Dialer net.Dialer

type Option func(req *http.Request)

// DialURL connects to the icecast server at `u` with the options given
//
// The ctx given is used for the dialing and sending of the request
func DialURL(ctx context.Context, u *url.URL, opts ...Option) (net.Conn, error) {
	conn, err := dial(ctx, u, opts...)
	if err == nil {
		return conn, nil
	}
	if conn != nil {
		conn.Close()
	}
	return nil, err
}

// Dial calls url.Parse on `u` and calls DialURL
func Dial(ctx context.Context, u string, opts ...Option) (net.Conn, error) {
	uri, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("Dial: failed to parse url: %w", err)
	}

	return DialURL(ctx, uri, opts...)
}

// UserAgent sets the User-Agent header
func UserAgent(ua string) Option {
	return func(req *http.Request) {
		req.Header.Set("User-Agent", ua)
	}
}

// Auth sets the BasicAuth user and password
func Auth(user, password string) Option {
	return func(req *http.Request) {
		req.SetBasicAuth(user, password)
	}
}

// ContentType sets the Content-Type header and is REQUIRED
func ContentType(ct string) Option {
	return func(req *http.Request) {
		req.Header.Set("Content-Type", ct)
	}
}

// Public sets the ice-public header
func Public(public bool) Option {
	return func(req *http.Request) {
		if public {
			req.Header.Set("ice-public", "1")
		} else {
			req.Header.Set("ice-public", "0")
		}
	}
}

// Name sets the ice-name header
func Name(name string) Option {
	return func(req *http.Request) {
		req.Header.Set("ice-name", name)
	}
}

// Description sets the ice-description header
func Description(desc string) Option {
	return func(req *http.Request) {
		req.Header.Set("ice-description", desc)
	}
}

// URL sets the ice-url header
func URL(u string) Option {
	return func(req *http.Request) {
		req.Header.Set("ice-url", u)
	}
}

// Genre sets the ice-genre header
func Genre(genre string) Option {
	return func(req *http.Request) {
		req.Header.Set("ice-genre", genre)
	}
}

// WithHeader sets a header on the request to icecast
func WithHeader(header, value string) Option {
	return func(req *http.Request) {
		req.Header.Set(header, value)
	}
}

func dial(ctx context.Context, u *url.URL, opts ...Option) (net.Conn, error) {
	// setup a request
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// apply options
	for _, opt := range opts {
		opt(req)
	}

	// check if we have the required options
	if req.Header.Get("Content-Type") == "" {
		return nil, errors.New("Content-Type header not set")
	}

	// check if we have an auth header set by an option
	checkURLForAuth(u, req)

	// connect to the configured host
	conn, err := Dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	// we might return with an error before we finish sending and receiving
	// the request/response so return the conn from dial so that Dial can close
	// it if there is an error

	// write our request
	err = req.Write(conn)
	if err != nil {
		return conn, fmt.Errorf("failed to write request: %w", err)
	}

	rdr := bufio.NewReader(conn)
	line, err := textproto.NewReader(rdr).ReadLine()
	if err != nil {
		return conn, fmt.Errorf("failed to read response: %w", err)
	}

	_, status, ok := strings.Cut(line, " ")
	if !ok {
		return conn, fmt.Errorf("malformed HTTP response: %s", line)
	}

	status = strings.TrimLeft(status, " ")

	statusCode, _, _ := strings.Cut(status, " ")
	if len(statusCode) != 3 {
		return conn, fmt.Errorf("malformed HTTP status code: %s", statusCode)
	}

	if statusCode != "200" {
		return conn, fmt.Errorf("status not ok: %w", errors.New(status))
	}

	return conn, nil
}

func checkURLForAuth(u *url.URL, req *http.Request) {
	_, _, ok := req.BasicAuth()
	if ok {
		return
	}

	// as a nice little bonus we set BasicAuth for the user if they forgot to
	// use the Auth option but did pass user/password info in the URL
	if passwd, ok := u.User.Password(); ok {
		Auth(u.User.Username(), passwd)(req)
	}
}
