package mariadb

import (
	"database/sql"
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

// UpdateUser implements radio.UserStorage
func (us UserStorage) UpdateUser(user radio.User) error {
	return nil
}

// Get implements radio.UserStorage
func (us UserStorage) Get(name string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.Get"

	var query = `
	SELECT
		users.id AS id,
		users.user AS username,
		users.pass AS password,
		IFNULL(users.email, '') AS email,
		users.ip AS ip,
		users.updated_at AS updated_at,
		users.deleted_at AS deleted_at,
		users.created_at AS created_at,
		group_concat(permissions.permission) AS userpermissions,
		IFNULL(djs.id, 0) AS 'dj.id',
		IFNULL(djs.regex, '') AS 'dj.regex',
		IFNULL(djs.djname, '') AS 'dj.name',

		IFNULL(djs.djtext, '') AS 'dj.text',
		IFNULL(djs.djimage, '') AS 'dj.image',

		IFNULL(djs.visible, 0) AS 'dj.visible',
		IFNULL(djs.priority, 0) AS 'dj.priority',
		IFNULL(djs.role, '') AS 'dj.role',

		IFNULL(djs.css, '') AS 'dj.css',
		IFNULL(djs.djcolor, '') AS 'dj.color',
		IFNULL(themes.id, 0) AS 'dj.theme.id',
		IFNULL(themes.name, '') AS 'dj.theme.name',
		IFNULL(themes.display_name, '') AS 'dj.theme.displayname',
		IFNULL(themes.author, '') AS 'dj.theme.author'
	FROM
		users
	LEFT JOIN
		djs ON users.djid = djs.id
	LEFT JOIN
		themes ON djs.theme_id = themes.id
	LEFT JOIN
		permissions ON users.id=permissions.user_id
	WHERE
		users.user=?
	GROUP BY
		users.id;
	`

	var user radio.User

	err := sqlx.Get(us.handle, &user, query, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.UserUnknown, errors.Info(name))
		}
		return nil, errors.E(op, err, errors.Info(name))
	}

	return &user, nil
}

// LookupName implements radio.UserStorage
func (us UserStorage) LookupName(name string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.LookupName"

	var query = `
	SELECT
		IFNULL(users.user, '') AS username,
		djs.id AS 'dj.id',
		djs.regex AS 'dj.regex',
		djs.djname AS 'dj.name',

		djs.djtext AS 'dj.text',
		djs.djimage AS 'dj.image',

		djs.visible AS 'dj.visible',
		djs.priority AS 'dj.priority',
		djs.role AS 'dj.role',

		djs.css AS 'dj.css',
		djs.djcolor AS 'dj.color',
		themes.id AS 'dj.theme.id',
		themes.name AS 'dj.theme.name',
		themes.display_name AS 'dj.theme.displayname',
		themes.author AS 'dj.theme.author'
	FROM
		djs
	LEFT JOIN
		users ON djs.id = users.djid
	JOIN
		themes ON djs.theme_id = themes.id;
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
