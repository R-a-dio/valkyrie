package util

import "net/http"

// IsHTMX checks if a request was made by HTMX through the Hx-Request header
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("Hx-Request") == "true"
}
