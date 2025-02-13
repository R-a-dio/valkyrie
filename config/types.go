package config

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"time"
)

// Duration is a time.Duration that supports Text(Un)Marshaler
type Duration time.Duration

// MarshalText implements encoding.TextMarshaler
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler
func (d *Duration) UnmarshalText(text []byte) error {
	n, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(n)
	return nil
}

type URL string

func (u URL) URL() *url.URL {
	// any file-based configuration will go through UnmarshalText
	// for the URL type, and the defaults are tested in the
	// roundtrip test, so there should be no way for a URL value
	// to be a string that doesn't url.Parse correctly.
	uri, err := url.Parse(string(u))
	if err != nil {
		panic("unreachable: unless you did something stupid")
	}
	return uri
}

func (u URL) MarshalText() ([]byte, error) {
	return []byte(u), nil
}

func (u *URL) UnmarshalText(text []byte) error {
	_, err := url.Parse(string(text))
	if err != nil {
		return err
	}
	*u = URL(text)
	return nil
}

type AddrPort struct {
	ap netip.AddrPort
}

func MustParseAddrPort(s string) AddrPort {
	ap, err := ParseAddrPort(s)
	if err != nil {
		panic("MustParseAddrPort: " + err.Error())
	}
	return ap
}

func ParseAddrPort(s string) (AddrPort, error) {
	host, sPort, err := net.SplitHostPort(s)
	if err != nil {
		return AddrPort{}, err
	}

	if host == "localhost" {
		host = localAddr.String()
	}

	var addr = localAddr
	if host != "" {
		addr, err = netip.ParseAddr(host)
		if err != nil {
			return AddrPort{}, err
		}
	}

	port, err := strconv.ParseUint(sPort, 10, 16)
	if err != nil {
		return AddrPort{}, err
	}

	return AddrPort{
		ap: netip.AddrPortFrom(addr, uint16(port)),
	}, nil
}

func (ap AddrPort) String() string {
	return ap.ap.String()
}

func (ap AddrPort) Port() uint16 {
	return ap.ap.Port()
}

func (ap AddrPort) Addr() netip.Addr {
	return ap.ap.Addr()
}

func (ap AddrPort) MarshalText() ([]byte, error) {
	return ap.ap.MarshalText()
}

var localAddr = netip.MustParseAddr("127.0.0.1")

func (ap *AddrPort) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}
	res, err := ParseAddrPort(string(text))
	if err != nil {
		return err
	}
	*ap = res

	return nil
}
