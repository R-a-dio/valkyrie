package compat

// this package exists because some old icecast source clients use
// ICE/1.0 in the HTTP/1.0 line and would be rejected by the Go
// stdlib net/http, instead we intercept new connections and see
// if this case occurs and transparently adjust the request so
// that it actually uses HTTP/1.0 before passing it over to the
// caller. This does mean the Conn returned by this package isn't
// one of the stdlib ones but hopefully not much requires those
// explicitly.
