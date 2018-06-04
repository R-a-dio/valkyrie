package database

import (
	"database/sql"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
)

// UserRequestTime returns the time of last request by this user.
func UserRequestTime(h Handler, user string) (time.Time, error) {
	var t time.Time

	query := "SELECT time FROM requesttime WHERE ip=? LIMIT 1;"
	//query := "SELECT time FROM requesttime WHERE identifier=? LIMIT 1;"

	err := sqlx.Get(h, &t, query, user)
	if err == sql.ErrNoRows {
		err = nil
	}

	return t, err
}

// UpdateUserRequestTime updates the last request time of the given user
// to the current time and date. The `update` parameter if true performs an
// UPDATE query, or an INSERT if false.
func UpdateUserRequestTime(h Handler, user string, update bool) error {
	var query string
	if update {
		query = "INSERT INTO requesttime (ip, time) VALUES (?, NOW());"
		//query = "INSERT INTO requesttime (identifier, time) VALUES (?, NOW());"
	} else {
		query = "UPDATE requesttime SET time=NOW() WHERE ip=?;"
		//query = "UPDATE requesttime SET time=NOW() WHERE identifier=?;"
	}

	_, err := h.Exec(query, user)
	return err
}

// calculateRequestDelay returns the delay between two requests of a song
func calculateRequestDelay(requestCount int) time.Duration {
	if requestCount > 30 {
		requestCount = 30
	}

	var dur float64
	if requestCount >= 0 && requestCount <= 7 {
		dur = -11057*math.Pow(float64(requestCount), 2) +
			172954*float64(requestCount) + 81720
	} else {
		dur = 599955*math.Exp(0.0372*float64(requestCount)) + 0.5
	}

	return time.Duration(time.Duration(dur/2) * time.Second)
}
