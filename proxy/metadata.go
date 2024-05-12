package proxy

import (
	"context"
	"hash/fnv"
	"html"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/japanese"
)

func (s *Server) GetMetadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := r.URL.Query()

	mount := GetMountpoint(r)
	if mount == "" {
		// ignore empty mount
		hlog.FromRequest(r).Info().Msg("empty mount")
		return
	}

	metadata := query.Get("song")
	if metadata == "" {
		// ignore empty metadata
		hlog.FromRequest(r).Info().Msg("empty metadata")
		return
	}

	charset := query.Get("charset")
	if charset == "" {
		// default to latin1, because reasons
		charset = "latin1"
	}

	metadata = ToUTF8(ctx, charset, metadata)
	if metadata == "" {
		// above can return an empty string due to text-encoding changes
		hlog.FromRequest(r).Info().Msg("empty metadata")
		return
	}

	err := s.proxy.SendMetadata(ctx, &Metadata{
		Time:       time.Now(),
		Identifier: IdentFromRequest(r),
		User:       *middleware.UserFromContext(ctx),
		MountName:  mount,
		Addr:       r.RemoteAddr,
		Value:      metadata,
	})
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to send metadata to proxy manager")
		return
	}

	response := []byte("<?xml version=\"1.0\"?>\n<iceresponse><message>Metadata update successful</message><return>1</return></iceresponse>\n")

	w.Header().Set("Content-Length", strconv.Itoa(len(response)))
	_, _ = w.Write(response)
}

func (s *Server) GetListClients(w http.ResponseWriter, r *http.Request) {
	response := []byte("<?xml version=\"1.0\"?>\n<icestats><source mount=\"" + html.EscapeString(GetMountpoint(r)) + "\"><Listeners>0</Listeners></source></icestats>\n")

	w.Header().Set("Content-Length", strconv.Itoa(len(response)))
	_, err := w.Write(response)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Msg("failed to write listclients response")
	}
}

type Metadata struct {
	Time       time.Time
	Identifier Identifier
	User       radio.User
	MountName  string
	Addr       string

	Value string
}

func ToUTF8(ctx context.Context, charset, meta string) string {
	if charset != "latin1" {
		// we special case latin1, but others can just decode to utf8
		enc, err := htmlindex.Get(charset)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).
				Str("text-encoding", "proxy").
				Str("charset", charset).
				Msg("unknown charset")
			return ""
		}

		res, err := enc.NewDecoder().String(meta)
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).
				Str("text-encoding", "proxy").
				Str("charset", charset).
				Str("metadata", meta).
				Msg("failed to decode")
			return ""
		}
		return res
	}

	// we're dealing with potentially "fake latin1", first see if we
	// just got send valid utf8 instead
	if utf8.ValidString(meta) {
		return meta
	}

	// if not valid utf8 we try SJIS
	sjisMeta, err := japanese.ShiftJIS.NewDecoder().String(meta)
	if err == nil && !strings.ContainsRune(sjisMeta, utf8.RuneError) {
		// successfully converted to sjis
		return sjisMeta
	}

	// so that leaves it maybe being actual valid latin1
	latinMeta, err := charmap.ISO8859_1.NewDecoder().String(meta)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).
			Str("text-encoding", "proxy").
			Str("charset", charset).
			Str("metadata", meta).
			Msg("failed to decode")
		return ""
	}
	return latinMeta
}

type identifierKey struct{}

type Identifier uint64

func IdentFromRequest(r *http.Request) Identifier {
	v := r.Context().Value(identifierKey{})
	if v == nil {
		return 0
	}
	return v.(Identifier)
}

func IdentifierMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// collect the user
		user := middleware.UserFromContext(ctx)
		if user == nil {
			// no user available, which means we can't really make an identifier so
			// just don't add one and continue to the next handler
			next.ServeHTTP(w, r)
			return
		}
		// we don't want the port since that is randomized for each connection and
		// we use this identifier to match the same client along multiple connections
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			// constant used by the net package
			const missingPort = "missing port in address"

			// if we don't have a port for some reason just use the RemoteAddr as-is
			var aerr *net.AddrError
			if errors.As(err, &aerr) && aerr.Err == missingPort {
				host = r.RemoteAddr
			}
		}

		h := fnv.New64a()
		_, _ = io.WriteString(h, user.Username)
		_, _ = io.WriteString(h, GetMountpoint(r))
		_, _ = io.WriteString(h, host)
		id := Identifier(h.Sum64())

		ctx = context.WithValue(ctx, identifierKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetMountpoint(r *http.Request) string {
	if r.URL.Path == "/admin/metadata" || r.URL.Path == "/admin/listclients" {
		// but if it's one of the above routes we get the mountpoint from a GET parameter
		return strings.ToLower(r.URL.Query().Get("mount"))
	}
	return strings.ToLower(r.URL.Path)
}

func GetAudioFormat(r *http.Request) string {
	switch r.Header.Get("Content-Type") {
	case "audio/mpeg":
		return "MP3"
	case "audio/ogg", "application/ogg":
		return "OGG"
	}
	return ""
}
