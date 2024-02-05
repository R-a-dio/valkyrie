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
		users.id AS 'user.id',
		users.user AS 'user.username'
	FROM
		radio_news
	JOIN
		users ON users.id = radio_news.user_id
	WHERE
		radio_news.id=?;
	`

	var post radio.NewsPost

	err := sqlx.Get(ns.handle, &post, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.E(op, errors.NewsUnknown)
		}
		return nil, errors.E(op, err)
	}

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

	// check for required fields
	field, ok := post.HasRequired()
	if !ok {
		return 0, errors.E(op, errors.InvalidArgument, errors.Info(field))
	}

	new, err := namedExecLastInsertId(ns.handle, newsCreateQuery, post)
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
	user_id=:user.id,
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

	// check for required fields
	field, ok := post.HasRequired()
	if !ok {
		return errors.E(op, errors.InvalidArgument, errors.Info(field))
	}

	// execute
	_, err := sqlx.NamedExec(ns.handle, newsUpdateQuery, post)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

// Delete implements radio.NewsStorage
func (ns NewsStorage) Delete(id radio.NewsPostID) error {
	const op errors.Op = "mariadb/NewsStorage.Delete"

	var query = `
	UPDATE 
		radio_news
	SET
		deleted_at=NOW()
	WHERE
		id=?;
	`

	res, err := ns.handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return errors.E(op, err)
	}

	if affected != 1 {
		return errors.E(op, errors.InvalidArgument)
	}

	return nil
}

// List implements radio.NewsStorage
func (ns NewsStorage) List(limit int, offset int) (radio.NewsList, error) {
	const op errors.Op = "mariadb/NewsStorage.List"

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
		users.id AS 'user.id',
		users.user AS 'user.username'
	FROM
		radio_news
	JOIN
		users ON radio_news.user_id = users.id
	ORDER BY
		radio_news.created_at DESC
	LIMIT ? OFFSET ?;
	`

	var news = radio.NewsList{
		Entries: make([]radio.NewsPost, 0, limit),
	}

	err := sqlx.Select(ns.handle, &news.Entries, query, limit, offset)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}

	query = `SELECT COUNT(*) AS total FROM radio_news;`

	err = sqlx.Get(ns.handle, &news.Total, query)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}
	return news, nil
}

// ListPublic implements radio.NewsStorage
func (ns NewsStorage) ListPublic(limit int, offset int) (radio.NewsList, error) {
	const op errors.Op = "mariadb/NewsStorage.ListPublic"

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
		users.id AS 'user.id',
		users.user AS 'user.username'
	FROM
		radio_news
	JOIN
		users ON radio_news.user_id = users.id
	WHERE
		radio_news.private=0 AND
		radio_news.deleted_at IS NULL
	ORDER BY
		radio_news.created_at DESC
	LIMIT ? OFFSET ?;
	`

	var news = radio.NewsList{
		Entries: make([]radio.NewsPost, 0, limit),
	}

	err := sqlx.Select(ns.handle, &news.Entries, query, limit, offset)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}

	query = `SELECT COUNT(*) AS total FROM radio_news WHERE private = 0 AND deleted_at IS NULL;`

	err = sqlx.Get(ns.handle, &news.Total, query)
	if err != nil {
		return radio.NewsList{}, errors.E(op, err)
	}
	return news, nil
}

// Comments implements radio.NewsStorage
func (ns NewsStorage) Comments(radio.NewsPostID) ([]radio.NewsComment, error) {
	const op errors.Op = "mariadb/NewsStorage.Comments"
	return nil, nil
}
