package website

import (
	"net/http"

	"github.com/gorilla/mux"
)

/*
legacy routes, these all redirect to "stream.r-a-d.io/main":
	/main.mp3
	/main
	/stream.mp3
	/stream
	/R-a-dio

api routes, some of these might never have been implemented in PHP
	GET /api - done
	GET /api/ping - done
	GET /api/song -> links only valid one day, not implemented
	GET /api/user-cooldown - done
	GET /api/news
	GET /api/search
	GET /api/metadata - useless, not implemented
	GET /api/can-request - done
	POST /request - done
	GET /api/dj-image -> turn into static route

public routes
	GET /
	GET /index
	GET /queue
	GET /last-played
	GET /staff
	GET, POST /faves
	GET /irc
	GET, POST /search
	GET, POST /news
	GET, POST /login
	GET, POST /submit
	ANY /set-theme
	ANY /logout

private routes
	PUT, POST, DELETE /api/dj-image
	DELETE /news
	GET /admin/
	GET /admin/index
	GET /admin/dev
	GET /admin/restore
	GET, POST, PUT, DELETE /admin/news
	GET, POST /admin/pending
	GET /admin/pending-song
	GET /admin/song
	GET, POST /admin/songs
	GET, POST, PUT, DELETE /admin/users
	GET, PUT /admin/profile

*/

var dummy http.HandlerFunc

func PublicRoutes() {
	r := mux.NewRouter()
	r.HandleFunc("/", dummy)
	r.HandleFunc("/queue", dummy)
	r.HandleFunc("/last-played", dummy)
	r.HandleFunc("/staff", dummy)
	r.HandleFunc("/faves", dummy)
	r.HandleFunc("/irc", dummy)
	r.HandleFunc("/search", dummy)
	r.HandleFunc("/news", dummy)
	r.HandleFunc("/login", dummy)
	r.HandleFunc("/submit", dummy)
	r.HandleFunc("/set-theme", dummy)
	r.HandleFunc("/logout", dummy)
}
