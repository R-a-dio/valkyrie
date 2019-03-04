package mariadb

import (
	"log"
	"regexp"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// UserStorage implements radio.UserStorage
type UserStorage struct {
	handle handle
}

// LookupName implements radio.UserStorage
func (us UserStorage) LookupName(name string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.LookupName"

	var query = `
	SELECT
		IFNULL(users.user, '') AS username,	
		djs.id AS 'dj.id',
		djs.regex AS 'dj.regex'
	FROM
		djs
	LEFT JOIN
		users ON djs.id = users.djid;
	`

	var users []radio.User

	err := sqlx.Select(us.handle, &users, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for _, user := range users {
		if user.DJ.Regex == "" {
			// skip users with no regex
			continue
		}

		re, err := regexp.Compile(`(?i)` + user.DJ.Regex)
		if err != nil {
			log.Printf("%s: invalid regex field: %v", op, err)
			continue
		}

		if re.MatchString(name) {
			return &user, nil
		}
	}

	return nil, errors.E(op, errors.UserUnknown, errors.Info(name))
}

// ByNick implements radio.UserStorage
func (us UserStorage) ByNick(nick string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.ByNick"

	return nil, errors.E(op, errors.NotImplemented)
}

// HasPermission implements radio.UserStorage
func (us UserStorage) HasPermission(user radio.User, perm radio.UserPermission) (bool, error) {
	const op errors.Op = "mariadb/UserStorage.HasPermission"

	return false, errors.E(op, errors.NotImplemented)
}

// RecordListeners implements radio.UserStorage
func (us UserStorage) RecordListeners(listeners int, user radio.User) error {
	const op errors.Op = "mariadb/UserStorage.RecordListeners"

	var query = `INSERT INTO listenlog (listeners, dj) VALUES (?, ?);`

	_, err := us.handle.Exec(query, listeners, user.DJ.ID)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
