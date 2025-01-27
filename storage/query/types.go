package query

type Queries struct {
	Session
}

type Session struct {
	Get    string
	Delete string
	Save   string
}

const SessionGetQuery = `
SELECT
	token,
	expiry,
	data
FROM
	sessions
WHERE
	token=:token;
`
