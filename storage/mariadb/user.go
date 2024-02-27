package mariadb

import (
	"database/sql"
	"fmt"
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
func (us UserStorage) UpdateUser(user radio.User) (radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.UpdateUser"

	var query string

	// start trans
	handle, tx, err := requireTx(us.handle)
	if err != nil {
		return user, errors.E(op, err)
	}
	defer tx.Rollback()
	// if userid is 0, insert new user

	if user.ID == 0 {
		query = `
			INSERT INTO users (
				user,
				pass,
				email,
				ip,
				updated_at,
				created_at
			)
			VALUES (
				:username,
				:password,
				:email,
				:ip,
				CURRENT_TIMESTAMP(),
				CURRENT_TIMESTAMP()
			);
		`
	} else {
		query = `
			UPDATE 
				users
			SET
				pass=:password,
				email=:email,
				ip=:ip,
				updated_at=CURRENT_TIMESTAMP()
			WHERE
				users.id=:id;
		`
	}
	result, err := sqlx.NamedExec(handle, query, user)
	if err != nil {
		return user, errors.E(op, err)
	}
	if user.ID == 0 {
		last, err := result.LastInsertId()
		if err != nil {
			return user, errors.E(op, err)
		}
		user.ID = radio.UserID(last)
	}

	// delete perms
	query = `
		DELETE FROM permissions
		WHERE
			permissions.user_id = ?
		;
	`
	_, err = handle.Exec(query, user.ID)
	if err != nil {
		return user, errors.E(op, err)
	}

	// insert perms
	query = `
		INSERT INTO permissions (
			user_id,
			permission
		)
		VALUES (?, ?);
	`
	for perm := range user.UserPermissions {
		_, err := handle.Exec(query, user.ID, perm)
		if err != nil {
			return user, errors.E(op, err)
		}
	}

	// If djid is zero and dj is nondefault, insert
	// Otherwise, if djid is nonzero, update
	if user.DJ.ID == 0 && user.DJ != (radio.DJ{}) {
		query = `
			INSERT INTO djs (
				djname,
				djtext,
				djimage,
				visible,
				priority,
				css,
				djcolor,
				role,
				theme_id,
				regex
			) VALUES (
				:name,
				:text,
				:image,
				:visible,
				:priority,
				:css,
				:color,
				:role,
				:theme.id,
				:regex
			);
		`
		result, err = sqlx.NamedExec(handle, query, user.DJ)
		if err != nil {
			return user, errors.E(op, err)
		}
		last, err := result.LastInsertId()
		if err != nil {
			return user, errors.E(op, err)
		}
		user.DJ.ID = radio.DJID(last)

		// insert the new ID into the users table
		query = `
			UPDATE
				users
			SET
				djid=:dj.id
			WHERE
				id=:id;
		`

		_, err = sqlx.NamedExec(handle, query, user)
		if err != nil {
			return user, errors.E(op, err)
		}
	} else if user.DJ.ID != 0 {
		query = `
			UPDATE
				djs 
			SET 
				djname=:name,
				djtext=:text,
				djimage=:image,
				visible=:visible,
				priority=:priority,
				css=:css,
				djcolor=:color,
				role=:role,
				theme_id=:theme.id,
				regex=:regex
			WHERE
				id=:id;
		`
		_, err = sqlx.NamedExec(handle, query, user.DJ)
		if err != nil {
			return user, errors.E(op, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return user, errors.E(op, err)
	}

	return user, nil
}

// getQuery is for single-row returns on the users table, the only thing you can
// change by using fmt is the WHERE clause. Just read the query really
const getUserQuery = `
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
	%s
GROUP BY
	users.id;
`

// Get implements radio.UserStorage
func (us UserStorage) Get(name string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.Get"

	var query = fmt.Sprintf(getUserQuery, "users.user=?")

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

func (us UserStorage) GetByID(id radio.UserID) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.GetByID"

	var user radio.User

	var query = fmt.Sprintf(getUserQuery, "users.id=?")

	err := sqlx.Get(us.handle, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.UserUnknown)
		}
		return nil, errors.E(op, err)
	}
	return &user, nil
}

// GetByDJID implements radio.UserStorage
func (us UserStorage) GetByDJID(id radio.DJID) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.GetByDJID"

	var query = fmt.Sprintf(getUserQuery, "djs.id=?")

	var user radio.User

	err := sqlx.Get(us.handle, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.UserUnknown)
		}
		return nil, errors.E(op, err)
	}

	return &user, nil
}

// LookupName implements radio.UserStorage
func (us UserStorage) LookupName(name string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.LookupName"

	users, err := us.All()
	if err != nil {
		return nil, errors.E(op, err)
	}

	for _, user := range users {
		if user.DJ.Regex == "" {
			// skip users with no regex
			continue
		}

		if MatchName(user.DJ.Regex, name) {
			return &user, nil
		}
	}

	return nil, errors.E(op, errors.UserUnknown, errors.Info(name))
}

func MatchName(regex, name string) bool {
	re, err := regexp.Compile(`(?i)` + regex)
	if err != nil {
		return false
	}

	return re.MatchString(name)
}

func (us UserStorage) All() ([]radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.All"

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
		IFNULL(users.user, '') AS username,
		(SELECT group_concat(permission) FROM permissions WHERE user_id=users.id) AS userpermissions,
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
		IFNULL(themes.id, 0) AS 'dj.theme.id',
		IFNULL(themes.name, 'default') AS 'dj.theme.name',
		IFNULL(themes.display_name, 'default') AS 'dj.theme.displayname',
		IFNULL(themes.author, 'unknown') AS 'dj.theme.author'
	FROM
		djs
	JOIN
		users ON djs.id = users.djid
	LEFT JOIN
		themes ON djs.theme_id = themes.id;
	`
	var users []radio.User

	err := sqlx.Select(us.handle, &users, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return users, nil
}

// ByNick implements radio.UserStorage
func (us UserStorage) ByNick(nick string) (*radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.ByNick"

	return nil, errors.E(op, errors.NotImplemented)
}

// Permissions implements radio.UserStorage
func (us UserStorage) Permissions() ([]radio.UserPermission, error) {
	const op errors.Op = "mariadb/UserStorage.Permissions"

	var query = `
	SELECT permission FROM permission_kinds;
	`

	var perms []radio.UserPermission

	err := sqlx.Select(us.handle, &perms, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return perms, nil
}

// RecordListeners implements radio.UserStorage
func (us UserStorage) RecordListeners(listeners radio.Listeners, user radio.User) error {
	const op errors.Op = "mariadb/UserStorage.RecordListeners"

	var query = `INSERT INTO listenlog (listeners, dj) VALUES (?, ?);`

	_, err := us.handle.Exec(query, listeners, user.DJ.ID)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
