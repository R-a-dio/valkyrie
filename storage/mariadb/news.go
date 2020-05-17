package mariadb

import (
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
	return nil, nil
}

// Create implements radio.NewsStorage
func (ns NewsStorage) Create(radio.NewsPost) (radio.NewsPostID, error) {
	const op errors.Op = "mariadb/NewsStorage.Create"
	return 0, nil
}

// Update implements radio.NewsStorage
func (ns NewsStorage) Update(radio.NewsPost) error {
	const op errors.Op = "mariadb/NewsStorage.Update"
	return nil
}

// Delete implements radio.NewsStorage
func (ns NewsStorage) Delete(radio.NewsPostID) error {
	const op errors.Op = "mariadb/NewsStorage.Delete"
	return nil
}

// List implements radio.NewsStorage
func (ns NewsStorage) List(limit int, offset int) (radio.NewsList, error) {
	const op errors.Op = "mariadb/NewsStorage.List"

	// TODO: implement user lookup
	var query = `
	SELECT
		radio_news.id AS id,
		radio_news.title AS title,
		radio_news.header AS header,
		radio_news.text AS body,
		radio_news.deleted_at AS deleted_at,
		radio_news.created_at AS created_at,
		radio_news.updated_at AS updated_at,
		radio_news.private AS private
	FROM
		radio_news
	JOIN
		users ON radio_news.user_id = users.id
	WHERE
		private=0
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

	query = `SELECT COUNT(*) AS total FROM radio_news WHERE private = 0;`

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
