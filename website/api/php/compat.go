package php

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rs/zerolog/hlog"
)

// MoveTokenToHeaderForRequests handles the case where someone sends a JSON body
// as form values to the request api (at /request). It extracts a '_token' JSON field
// and puts it into the X-CSRF-Token header instead.
//
// This is needed because our old android app uses this to communicate with the
// request api, this was apparently a laravel feature where Form.Get (Input::get)
// style API also read the request body if it was JSON.
func MoveTokenToHeaderForRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") == "application/json" &&
			strings.HasPrefix(r.URL.Path, "/request") {
			token := struct {
				Token string `json:"_token"`
			}{}

			err := json.NewDecoder(r.Body).Decode(&token)
			if err != nil {
				// ignore any errors, just log them and let the request go on as if
				// we never existed.
				hlog.FromRequest(r).Error().Err(err).Msg("compatibility")
			} else {
				r.Header.Set("X-CSRF-Token", token.Token)
			}
		}
		next.ServeHTTP(w, r)
	})
}
