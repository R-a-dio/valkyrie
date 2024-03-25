package mariadb

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

type ScheduleStorage struct {
	handle handle
}

var latestScheduleQuery = `
SELECT
	schedule.id AS id,
	schedule.weekday AS weekday,
	schedule.text AS text,
	schedule.updated_at AS updated_at,
	schedule.notification AS notification,

	ub.id AS 'updatedby.id',
	ub.user AS 'updatedby.username',
	ub.pass AS 'updatedby.password',
	IFNULL(ub.email, '') AS 'updatedby.email',
	ub.ip AS 'updatedby.ip',
	ub.updated_at AS 'updatedby.updated_at',
	ub.deleted_at AS 'updatedby.deleted_at',
	ub.created_at AS 'updatedby.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=ub.id) AS 'updatedby.userpermissions',
	IFNULL(ub_djs.id, 0) AS 'updatedby.dj.id',
	IFNULL(ub_djs.regex, '') AS 'updatedby.dj.regex',
	IFNULL(ub_djs.djname, '') AS 'updatedby.dj.name',
	IFNULL(ub_djs.djtext, '') AS 'updatedby.dj.text',
	IFNULL(ub_djs.djimage, '') AS 'updatedby.dj.image',
	IFNULL(ub_djs.visible, 0) AS 'updatedby.dj.visible',
	IFNULL(ub_djs.priority, 0) AS 'updatedby.dj.priority',
	IFNULL(ub_djs.role, '') AS 'updatedby.dj.role',
	IFNULL(ub_djs.css, '') AS 'updatedby.dj.css',
	IFNULL(ub_djs.djcolor, '') AS 'updatedby.dj.color',
	IFNULL(ub_themes.id, 0) AS 'updatedby.dj.theme.id',
	IFNULL(ub_themes.name, '') AS 'updatedby.dj.theme.name',
	IFNULL(ub_themes.display_name, '') AS 'updatedby.dj.theme.displayname',
	IFNULL(ub_themes.author, '') AS 'updatedby.dj.theme.author',

	IFNULL(ow.id, 0) AS 'owner.id',
	IFNULL(ow.user, '') AS 'owner.username',
	IFNULL(ow.pass, '') AS 'owner.password',
	IFNULL(ow.email, '') AS 'owner.email',
	IFNULL(ow.ip, '') AS 'owner.ip',
	IFNULL(ow.updated_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'owner.updated_at',
	IFNULL(ow.deleted_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'owner.deleted_at',
	IFNULL(ow.created_at, TIMESTAMP('0000-00-00 00:00:00')) AS 'owner.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=ow.id) AS 'owner.userpermissions',
	IFNULL(ow_djs.id, 0) AS 'owner.dj.id',
	IFNULL(ow_djs.regex, '') AS 'owner.dj.regex',
	IFNULL(ow_djs.djname, '') AS 'owner.dj.name',
	IFNULL(ow_djs.djtext, '') AS 'owner.dj.text',
	IFNULL(ow_djs.djimage, '') AS 'owner.dj.image',
	IFNULL(ow_djs.visible, 0) AS 'owner.dj.visible',
	IFNULL(ow_djs.priority, 0) AS 'owner.dj.priority',
	IFNULL(ow_djs.role, '') AS 'owner.dj.role',
	IFNULL(ow_djs.css, '') AS 'owner.dj.css',
	IFNULL(ow_djs.djcolor, '') AS 'owner.dj.color',
	IFNULL(ow_themes.id, 0) AS 'owner.dj.theme.id',
	IFNULL(ow_themes.name, 'default') AS 'owner.dj.theme.name',
	IFNULL(ow_themes.display_name, 'default') AS 'owner.dj.theme.displayname',
	IFNULL(ow_themes.author, 'unknown') AS 'owner.dj.theme.author'
FROM
	schedule
RIGHT JOIN
	(SELECT weekday, max(updated_at) AS updated_at FROM schedule GROUP BY weekday) AS s2 USING (weekday, updated_at)
JOIN
	users AS ub ON schedule.updated_by = ub.id
LEFT JOIN
	djs AS ub_djs ON ub.djid = ub_djs.id
LEFT JOIN
	themes AS ub_themes ON ub_djs.theme_id = ub_themes.id
LEFT JOIN
	users AS ow ON schedule.owner = ow.id
LEFT JOIN
	djs AS ow_djs ON ow.djid = ow_djs.id
LEFT JOIN
	themes AS ow_themes ON ow_djs.theme_id = ow_themes.id
ORDER BY
	schedule.weekday;
`

func (ss ScheduleStorage) Latest() ([]radio.ScheduleEntry, error) {
	const op errors.Op = "mariadb/ScheduleStorage.Latest"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var schedule []radio.ScheduleEntry

	err := sqlx.Select(handle, &schedule, latestScheduleQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for i := range schedule {
		if schedule[i].Owner == nil {
			continue
		}
		if schedule[i].Owner.ID == 0 {
			schedule[i].Owner = nil
		}
	}

	return schedule, nil
}

func (ss ScheduleStorage) Update(entry radio.ScheduleEntry) error {
	const op errors.Op = "mariadb/ScheduleStorage.Update"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
	INSERT INTO
		schedule (
			weekday,
			text,
			owner,
			updated_by,
			updated_at,
			notification
		) VALUES (
			:weekday,
			:text,
			IF(:owner.id, :owner.id, NULL),
			:updatedby.id,
			CURRENT_TIMESTAMP(),
			:notification
		)
	`

	// entry.Owner can be nil but that breaks the named query above, so we insert
	// a fake user with id 0 if it's nil
	if entry.Owner == nil {
		entry.Owner = &radio.User{}
	}

	_, err := sqlx.NamedExec(handle, query, entry)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ss ScheduleStorage) History(day radio.ScheduleDay, limit, offset int64) ([]radio.ScheduleEntry, error) {
	return nil, nil
}
