package mariadb

import (
	"database/sql"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// NewsStorage implements radio.NewsStorage
type NewsStorage struct {
	handle handle
}

// Get implements radio.NewsStorage
func (ns NewsStorage) Get(id radio.NewsPostID) (*radio.NewsPost, error) {
	const op errors.Op = "mariadb/NewsStorage.Get"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		radio_news.id AS id,
		radio_news.title AS title,
		radio_news.header AS header,
		radio_news.text AS body,
		radio_news.deleted_at AS deleted_at,
		radio_news.created_at AS created_at,
		radio_news.updated_at AS updated_at,
		radio_news.private AS private,
		radio_news.user_id AS 'user.id'
	FROM
		radio_news
	WHERE
		radio_news.id=?;
	`

	var post radio.NewsPost

	err := sqlx.Get(handle, &post, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.NewsUnknown)
		}
		return nil, errors.E(op, err)
	}

	// try to grab the user, but it might not exist in which case
	// we just return the ID that was stored in our table
	user, err := UserStorage{handle}.GetByID(post.User.ID)
	if err != nil {
		post.User.Username = "unknown"
		return &post, nil
	}
	post.User = *user

	return &post, nil
}

const newsCreateQuery = `
INSERT INTO
	radio_news (
		id,
		title,
		header,
		text,
		created_at,
		updated_at,
		private,
		user_id
	) VALUES (
		0,
		:title,
		:header,
		:body,
		NOW(),
		NOW(),
		:private,
		:user.id
	);
`

// Create implements radio.NewsStorage
func (ns NewsStorage) Create(post radio.NewsPost) (radio.NewsPostID, error) {
	const op errors.Op = "mariadb/NewsStorage.Create"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	// check for required fields
	field, ok := post.HasRequired()
	if !ok {
		return 0, errors.E(op, errors.InvalidArgument, errors.Info(field))
	}

	new, err := namedExecLastInsertId(handle, newsCreateQuery, post)
	if err != nil {
		return 0, errors.E(op, err)
	}

	return radio.NewsPostID(new), nil
}

const newsUpdateQuery = `
UPDATE
	radio_news
SET
	title=:title,
	header=:header,
	text=:body,
	deleted_at=:deletedat,
	created_at=:createdat,
	updated_at=NOW(),
	private=:private
WHERE
	id=:id;
`

// Update implements radio.NewsStorage
func (ns NewsStorage) Update(post radio.NewsPost) error {
	const op errors.Op = "mariadb/NewsStorage.Update"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	// check for required fields
	field, ok := post.HasRequired()
	if !ok {
		return errors.E(op, errors.InvalidArgument, errors.Info(field))
	}

	// execute
	_, err := sqlx.NamedExec(handle, newsUpdateQuery, post)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// Delete implements radio.NewsStorage
func (ns NewsStorage) Delete(id radio.NewsPostID) error {
	const op errors.Op = "mariadb/NewsStorage.Delete"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	var query = `
	UPDATE 
		radio_news
	SET
		deleted_at=NOW()
	WHERE
		id=?;
	`

	res, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return errors.E(op, err)
	}

	if affected != 1 {
		return errors.E(op, errors.NewsUnknown)
	}

	return nil
}

// List implements radio.NewsStorage
func (ns NewsStorage) List(limit int64, offset int64) (radio.NewsList, error) {
	const op errors.Op = "mariadb/NewsStorage.List"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		radio_news.id AS id,
		radio_news.title AS title,
		radio_news.header AS header,
		radio_news.text AS body,
		radio_news.deleted_at AS deleted_at,
		radio_news.created_at AS created_at,
		radio_news.updated_at AS updated_at,
		radio_news.private AS private,
		COALESCE(users.id, 0) AS 'user.id',
		COALESCE(users.user, 'unknown') AS 'user.username',
		COALESCE(users.pass, '') AS 'user.password',
		COALESCE(users.email, '') AS 'user.email',
		COALESCE(users.ip, '') AS 'user.ip',
		users.updated_at  AS 'user.updated_at',
		users.deleted_at AS 'user.deleted_at',
		COALESCE(users.created_at, TIMESTAMP('2010-10-10 10:10:10')) AS 'user.created_at',
		group_concat(permissions.permission) AS 'user.userpermissions',
		COALESCE(djs.id, 0) AS 'user.dj.id',
		COALESCE(djs.regex, '') AS 'user.dj.regex',
		COALESCE(djs.djname, '') AS 'user.dj.name',
	
		COALESCE(djs.djtext, '') AS 'user.dj.text',
		COALESCE(djs.djimage, '') AS 'user.dj.image',
	
		COALESCE(djs.visible, 0) AS 'user.dj.visible',
		COALESCE(djs.priority, 0) AS 'user.dj.priority',
		COALESCE(djs.role, '') AS 'user.dj.role',
	 
		COALESCE(djs.css, '') AS 'user.dj.css',
		COALESCE(djs.djcolor, '') AS 'user.dj.color',
		COALESCE(themes.id, 0) AS 'user.dj.theme.id',
		COALESCE(themes.name, 'default') AS 'user.dj.theme.name',
		COALESCE(themes.display_name, 'default') AS 'user.dj.theme.displayname',
		COALESCE(themes.author, 'unknown') AS 'user.dj.theme.author'
	FROM
		radio_news
	LEFT JOIN
		users ON radio_news.user_id = users.id
	LEFT JOIN
		djs ON users.djid = djs.id
	LEFT JOIN
		themes ON djs.theme_id = themes.id
	LEFT JOIN
		permissions ON users.id = permissions.user_id
	GROUP BY
		radio_news.id
	ORDER BY
		radio_news.created_at DESC
	LIMIT ? OFFSET ?;
	`

	var news = radio.NewsList{
		Entries: make([]radio.NewsPost, 0, limit),
	}

	err := sqlx.Select(handle, &news.Entries, query, limit, offset)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}

	query = `SELECT COUNT(*) AS total FROM radio_news;`

	err = sqlx.Get(handle, &news.Total, query)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}
	return news, nil
}

// ListPublic implements radio.NewsStorage
func (ns NewsStorage) ListPublic(limit int64, offset int64) (radio.NewsList, error) {
	const op errors.Op = "mariadb/NewsStorage.ListPublic"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		radio_news.id AS id,
		radio_news.title AS title,
		radio_news.header AS header,
		radio_news.text AS body,
		radio_news.deleted_at AS deleted_at,
		radio_news.created_at AS created_at,
		radio_news.updated_at AS updated_at, 
		radio_news.private AS private,
		COALESCE(users.id, 0) AS 'user.id',
		COALESCE(users.user, 'unknown') AS 'user.username',
		COALESCE(users.pass, '') AS 'user.password',
		COALESCE(users.email, '') AS 'user.email',
		COALESCE(users.ip, '') AS 'user.ip',
		users.updated_at  AS 'user.updated_at',
		users.deleted_at AS 'user.deleted_at',
		COALESCE(users.created_at, TIMESTAMP('2010-10-10 10:10:10')) AS 'user.created_at',
		group_concat(permissions.permission) AS 'user.userpermissions',
		COALESCE(djs.id, 0) AS 'user.dj.id',
		COALESCE(djs.regex, '') AS 'user.dj.regex',
		COALESCE(djs.djname, '') AS 'user.dj.name',
	
		COALESCE(djs.djtext, '') AS 'user.dj.text',
		COALESCE(djs.djimage, '') AS 'user.dj.image',
	
		COALESCE(djs.visible, 0) AS 'user.dj.visible',
		COALESCE(djs.priority, 0) AS 'user.dj.priority',
		COALESCE(djs.role, '') AS 'user.dj.role',
	 
		COALESCE(djs.css, '') AS 'user.dj.css',
		COALESCE(djs.djcolor, '') AS 'user.dj.color',
		COALESCE(themes.id, 0) AS 'user.dj.theme.id',
		COALESCE(themes.name, 'default') AS 'user.dj.theme.name',
		COALESCE(themes.display_name, 'default') AS 'user.dj.theme.displayname',
		COALESCE(themes.author, 'unknown') AS 'user.dj.theme.author'
	FROM
		radio_news
	LEFT JOIN
		users ON radio_news.user_id = users.id
	LEFT JOIN
		djs ON users.djid = djs.id
	LEFT JOIN
		themes ON djs.theme_id = themes.id
	LEFT JOIN
		permissions ON users.id = permissions.user_id
	WHERE
		radio_news.private=0 AND
		radio_news.deleted_at IS NULL
	GROUP BY
		radio_news.id
	ORDER BY
		radio_news.created_at DESC
	LIMIT ? OFFSET ?;
	`

	var news = radio.NewsList{
		Entries: make([]radio.NewsPost, 0, limit),
	}

	err := sqlx.Select(handle, &news.Entries, query, limit, offset)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}

	query = `SELECT COUNT(*) AS total FROM radio_news WHERE private = 0 AND deleted_at IS NULL;`

	err = sqlx.Get(handle, &news.Total, query)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}
	return news, nil
}

// Comments implements radio.NewsStorage
func (ns NewsStorage) Comments(postid radio.NewsPostID) ([]radio.NewsComment, error) {
	const op errors.Op = "mariadb/NewsStorage.Comments"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		radio_comments.id AS id,
		radio_comments.news_id AS postid,
		radio_comments.comment AS body,
		radio_comments.ip AS identifier,
		radio_comments.user_id AS userid,
		radio_comments.created_at AS created_at,
		radio_comments.deleted_at AS deleted_at,
		radio_comments.updated_at AS updated_at,

		COALESCE(users.id, 0) AS 'user.id',
		COALESCE(users.user, '') AS 'user.username',
		COALESCE(users.pass, '') AS 'user.password',
		COALESCE(users.email, '') AS 'user.email',
		COALESCE(users.ip, '') AS 'user.ip',
		users.updated_at  AS 'user.updated_at',
		users.deleted_at AS 'user.deleted_at',
		COALESCE(users.created_at, TIMESTAMP('2010-10-10 10:10:10')) AS 'user.created_at',
		group_concat(permissions.permission) AS 'user.userpermissions',
		COALESCE(djs.id, 0) AS 'user.dj.id',
		COALESCE(djs.regex, '') AS 'user.dj.regex',
		COALESCE(djs.djname, '') AS 'user.dj.name',
	
		COALESCE(djs.djtext, '') AS 'user.dj.text',
		COALESCE(djs.djimage, '') AS 'user.dj.image',
	
		COALESCE(djs.visible, 0) AS 'user.dj.visible',
		COALESCE(djs.priority, 0) AS 'user.dj.priority',
		COALESCE(djs.role, '') AS 'user.dj.role',
	
		COALESCE(djs.css, '') AS 'user.dj.css',
		COALESCE(djs.djcolor, '') AS 'user.dj.color',
		COALESCE(themes.id, 0) AS 'user.dj.theme.id',
		COALESCE(themes.name, 'default') AS 'user.dj.theme.name',
		COALESCE(themes.display_name, 'default') AS 'user.dj.theme.displayname',
		COALESCE(themes.author, 'unknown') AS 'user.dj.theme.author'
	FROM
		radio_comments
	LEFT JOIN
		users ON users.id = radio_comments.user_id
	LEFT JOIN
		djs ON users.djid = djs.id
	LEFT JOIN
		themes ON djs.theme_id = themes.id
	LEFT JOIN
		permissions ON users.id = permissions.user_id
	WHERE
		radio_comments.news_id = ?
	GROUP BY
		radio_comments.id
	ORDER BY
		radio_comments.created_at DESC;
	`

	var comments = []radio.NewsComment{}

	err := sqlx.Select(handle, &comments, query, postid)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for _, comm := range comments {
		if comm.UserID == nil {
			comm.User = nil
		}
	}

	return comments, nil
}

// CommentsPublic implements radio.NewsStorage
func (ns NewsStorage) CommentsPublic(postid radio.NewsPostID) ([]radio.NewsComment, error) {
	const op errors.Op = "mariadb/NewsStorage.Comments"
	handle, deferFn := ns.handle.span(op)
	defer deferFn()

	var query = `
	SELECT
		radio_comments.id AS id,
		radio_comments.news_id AS postid,
		radio_comments.comment AS body,
		radio_comments.ip AS identifier,
		radio_comments.user_id AS userid,
		radio_comments.created_at AS created_at,
		radio_comments.deleted_at AS deleted_at,
		radio_comments.updated_at AS updated_at,

		COALESCE(users.id, 0) AS 'user.id',
		COALESCE(users.user, '') AS 'user.username',
		COALESCE(users.pass, '') AS 'user.password',
		COALESCE(users.email, '') AS 'user.email',
		COALESCE(users.ip, '') AS 'user.ip',
		users.updated_at  AS 'user.updated_at',
		users.deleted_at AS 'user.deleted_at',
		COALESCE(users.created_at, TIMESTAMP('2010-10-10 10:10:10')) AS 'user.created_at',
		group_concat(permissions.permission) AS 'user.userpermissions',
		COALESCE(djs.id, 0) AS 'user.dj.id',
		COALESCE(djs.regex, '') AS 'user.dj.regex',
		COALESCE(djs.djname, '') AS 'user.dj.name',
	
		COALESCE(djs.djtext, '') AS 'user.dj.text',
		COALESCE(djs.djimage, '') AS 'user.dj.image',
	
		COALESCE(djs.visible, 0) AS 'user.dj.visible',
		COALESCE(djs.priority, 0) AS 'user.dj.priority',
		COALESCE(djs.role, '') AS 'user.dj.role',
	 
		COALESCE(djs.css, '') AS 'user.dj.css',
		COALESCE(djs.djcolor, '') AS 'user.dj.color',
		COALESCE(themes.id, 0) AS 'user.dj.theme.id',
		COALESCE(themes.name, 'default') AS 'user.dj.theme.name',
		COALESCE(themes.display_name, 'default') AS 'user.dj.theme.displayname',
		COALESCE(themes.author, 'unknown') AS 'user.dj.theme.author'
	FROM
		radio_comments
	LEFT JOIN
		users ON users.id = radio_comments.user_id
	LEFT JOIN
		djs ON users.djid = djs.id
	LEFT JOIN
		themes ON djs.theme_id = themes.id
	LEFT JOIN
		permissions ON users.id = permissions.user_id
	WHERE
		radio_comments.news_id = ? AND
		radio_comments.deleted_at IS NULL
	GROUP BY
		radio_comments.id
	ORDER BY
		radio_comments.created_at DESC;
	`

	var comments = []radio.NewsComment{}

	err := sqlx.Select(handle, &comments, query, postid)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for _, comm := range comments {
		if comm.UserID == nil {
			comm.User = nil
		}
	}

	return comments, nil
}
