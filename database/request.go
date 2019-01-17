package database

import (
	"database/sql"
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
