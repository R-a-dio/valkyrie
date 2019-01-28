package database

import (
	"log"
	"regexp"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/jmoiron/sqlx"
)

/*
	users.id AS id,
	users.user AS username,
	users.updated_at AS updatedat,
	users.deleted_at AS deletedat,
	users.created_at AS createdat,
	users.email AS email,
	users.remember_token AS remembertoken,
	users.ip,
	djs.regex
*/

func LookupNickname(h Handler, name string) (*radio.User, error) {
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

	err := sqlx.Select(h, &users, query)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		if user.DJ.Regex == "" {
			// skip users with no regex
			continue
		}

		re, err := regexp.Compile(`(?i)` + user.DJ.Regex)
		if err != nil {
			log.Printf("database: invalid regex field: %v", err)
			continue
		}

		if re.MatchString(name) {
			return &user, nil
		}
	}

	return nil, radio.ErrUnknownUser
}
