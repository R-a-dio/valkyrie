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

const updateUserQuery = `
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

const updateUserAndDJQuery = `
UPDATE
	users, djs 
SET 
	users.pass=:password,
	users.email=:email,
	users.ip=:ip,
	users.updated_at=CURRENT_TIMESTAMP()
	djs.djname=:dj.name,
	djs.djtext=:dj.text,
	djs.djimage=:dj.image,
	djs.visible=:dj.visible,
	djs.priority=:dj.priority,
	djs.css=:dj.css,
	djs.djcolor=:dj.color,
	djs.role=:dj.role,
	djs.theme_id=:dj.theme.id,
	djs.regex=:dj.regex
WHERE
	users.id=:id AND djs.id=:dj.id;
`

// Update implements radio.UserStorage
func (us UserStorage) Update(user radio.User) (radio.User, error) {
	const op errors.Op = "mariadb/UserStorage.Update"
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	var query string

	handle, tx, err := requireTx(handle)
	if err != nil {
		return user, errors.E(op, err)
	}
	defer tx.Rollback()

	query = updateUserQuery
	if user.DJ.ID != 0 { // use combi query if there is a dj
		query = updateUserAndDJQuery
	}

	// update the users (and dj) table
	_, err = sqlx.NamedExec(handle, query, user)
	if err != nil {
		return user, errors.E(op, err)
	}

	// update the permissions table
	err = us.updatePermissions(handle, user)
	if err != nil {
		return user, errors.E(op, err)
	}

	err = tx.Commit()
	if err != nil {
		return user, errors.E(op, err)
	}
	return user, nil
}

const createUserQuery = `
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

func (us UserStorage) Create(user radio.User) (radio.UserID, error) {
	const op errors.Op = "mariadb/UserStorage.Create"
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	handle, tx, err := requireTx(handle)
	if err != nil {
		return 0, errors.E(op, err)
	}
	defer tx.Rollback()

	res, err := sqlx.NamedExec(handle, createUserQuery, user)
	if err != nil {
		return 0, errors.E(op, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, errors.E(op, err)
	}
	user.ID = radio.UserID(id)

	err = us.updatePermissions(handle, user)
	if err != nil {
		return 0, errors.E(op, err)
	}

	err = tx.Commit()
	if err != nil {
		return 0, errors.E(op, err)
	}

	return radio.UserID(id), nil
}

const createDJQuery = `
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

const updateUserDJIDQuery = `
UPDATE
	users, djs
SET
	users.djid=:dj.id
WHERE
	users.id=:id;
`

func (us UserStorage) CreateDJ(user radio.User, dj radio.DJ) (radio.DJID, error) {
	const op errors.Op = "mariadb/UserStorage.CreateDJ"
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	user.DJ = dj

	handle, tx, err := requireTx(handle)
	if err != nil {
		return 0, errors.E(op, err)
	}
	defer tx.Rollback()

	res, err := sqlx.NamedExec(handle, createDJQuery, dj)
	if err != nil {
		return 0, errors.E(op, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, errors.E(op, err)
	}
	user.DJ.ID = radio.DJID(id)

	_, err = handle.Exec(updateUserDJIDQuery, user)
	if err != nil {
		return 0, errors.E(op, err)
	}

	err = tx.Commit()
	if err != nil {
		return 0, errors.E(op, err)
	}

	return user.DJ.ID, nil
}

const updatePermissionsQuery = `
INSERT INTO permissions (
	user_id,
	permission
) VALUES (?, ?);
`
const deletePermissionQuery = `
DELETE FROM 
	permissions
WHERE
	permissions.user_id = ?;
`

func (us UserStorage) updatePermissions(handle handle, user radio.User) error {
	const op errors.Op = "mariadb/UserStorage.updatePermissions"
	handle, deferFn := handle.span(op)
	defer deferFn()

	handle, tx, err := requireTx(handle)
	if err != nil {
		return errors.E(op, err)
	}
	defer tx.Rollback()

	_, err = handle.Exec(deletePermissionQuery, user.ID)
	if err != nil {
		return errors.E(op, err)
	}

	for perm := range user.UserPermissions {
		_, err = handle.Exec(updatePermissionsQuery, user.ID, perm)
		if err != nil {
			return errors.E(op, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.E(op, err)
	}
	return nil
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
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	var query = fmt.Sprintf(getUserQuery, "users.user=?")

	var user radio.User

	err := sqlx.Get(handle, &user, query, name)
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
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	var user radio.User

	var query = fmt.Sprintf(getUserQuery, "users.id=?")

	err := sqlx.Get(handle, &user, query, id)
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
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	var query = fmt.Sprintf(getUserQuery, "djs.id=?")

	var user radio.User

	err := sqlx.Get(handle, &user, query, id)
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
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	users, err := UserStorage{handle}.All()
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
	handle, deferFn := us.handle.span(op)
	defer deferFn()

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

	err := sqlx.Select(handle, &users, query)
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
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	var query = `
	SELECT permission FROM permission_kinds;
	`

	var perms []radio.UserPermission

	err := sqlx.Select(handle, &perms, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return perms, nil
}

// RecordListeners implements radio.UserStorage
func (us UserStorage) RecordListeners(listeners radio.Listeners, user radio.User) error {
	const op errors.Op = "mariadb/UserStorage.RecordListeners"
	handle, deferFn := us.handle.span(op)
	defer deferFn()

	var query = `INSERT INTO listenlog (listeners, dj) VALUES (?, ?);`

	_, err := handle.Exec(query, listeners, user.DJ.ID)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}
