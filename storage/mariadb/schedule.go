package mariadb

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

type ScheduleStorage struct {
	handle handle
}

var scheduleLatestQuery = `
SELECT
	schedule.id AS id,
	schedule.weekday AS weekday,
	schedule.text AS text,
	schedule.updated_at AS updated_at,
	schedule.notification AS notification,

	ub.id AS 'updatedby.id',
	ub.user AS 'updatedby.username',
	ub.pass AS 'updatedby.password',
	COALESCE(ub.email, '') AS 'updatedby.email',
	ub.ip AS 'updatedby.ip',
	ub.updated_at AS 'updatedby.updated_at',
	ub.deleted_at AS 'updatedby.deleted_at',
	ub.created_at AS 'updatedby.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=ub.id) AS 'updatedby.userpermissions',
	COALESCE(ub_djs.id, 0) AS 'updatedby.dj.id',
	COALESCE(ub_djs.regex, '') AS 'updatedby.dj.regex',
	COALESCE(ub_djs.djname, '') AS 'updatedby.dj.name',
	COALESCE(ub_djs.djtext, '') AS 'updatedby.dj.text',
	COALESCE(ub_djs.djimage, '') AS 'updatedby.dj.image',
	COALESCE(ub_djs.visible, 0) AS 'updatedby.dj.visible',
	COALESCE(ub_djs.priority, 0) AS 'updatedby.dj.priority',
	COALESCE(ub_djs.role, '') AS 'updatedby.dj.role',
	COALESCE(ub_djs.css, '') AS 'updatedby.dj.css',
	COALESCE(ub_djs.djcolor, '') AS 'updatedby.dj.color',
	COALESCE(ub_djs.theme_name, '') AS 'updatedby.dj.theme',

	COALESCE(ow.id, 0) AS 'owner.id',
	COALESCE(ow.user, '') AS 'owner.username',
	COALESCE(ow.pass, '') AS 'owner.password',
	COALESCE(ow.email, '') AS 'owner.email',
	COALESCE(ow.ip, '') AS 'owner.ip',
	COALESCE(ow.updated_at, TIMESTAMP('0000-00-00')) AS 'owner.updated_at',
	COALESCE(ow.deleted_at, TIMESTAMP('0000-00-00')) AS 'owner.deleted_at',
	COALESCE(ow.created_at, TIMESTAMP('0000-00-00')) AS 'owner.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=ow.id) AS 'owner.userpermissions',
	COALESCE(ow_djs.id, 0) AS 'owner.dj.id',
	COALESCE(ow_djs.regex, '') AS 'owner.dj.regex',
	COALESCE(ow_djs.djname, '') AS 'owner.dj.name',
	COALESCE(ow_djs.djtext, '') AS 'owner.dj.text',
	COALESCE(ow_djs.djimage, '') AS 'owner.dj.image',
	COALESCE(ow_djs.visible, 0) AS 'owner.dj.visible',
	COALESCE(ow_djs.priority, 0) AS 'owner.dj.priority',
	COALESCE(ow_djs.role, '') AS 'owner.dj.role',
	COALESCE(ow_djs.css, '') AS 'owner.dj.css',
	COALESCE(ow_djs.djcolor, '') AS 'owner.dj.color',
	COALESCE(ow_djs.theme_name, '') AS 'owner.dj.theme'
FROM
	schedule
RIGHT JOIN
	(SELECT weekday, max(updated_at) AS updated_at FROM schedule GROUP BY weekday) AS s2 USING (weekday, updated_at)
JOIN
	users AS ub ON schedule.updated_by = ub.id
LEFT JOIN
	djs AS ub_djs ON ub.djid = ub_djs.id
LEFT JOIN
	users AS ow ON schedule.owner = ow.id
LEFT JOIN
	djs AS ow_djs ON ow.djid = ow_djs.id
ORDER BY
	schedule.weekday;
`

func (ss ScheduleStorage) Latest() ([]*radio.ScheduleEntry, error) {
	const op errors.Op = "mariadb/ScheduleStorage.Latest"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var tmp []radio.ScheduleEntry

	err := sqlx.Select(handle, &tmp, scheduleLatestQuery)
	if err != nil {
		return nil, errors.E(op, err)
	}

	schedule := make([]*radio.ScheduleEntry, 7)
	for _, entry := range tmp {
		entry := entry
		if entry.Owner != nil && entry.Owner.ID == 0 {
			entry.Owner = nil
		}

		schedule[entry.Weekday] = &entry
	}

	return schedule, nil
}

const scheduleUpdateQuery = `
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
	);
`

var _ = CheckQuery[radio.ScheduleEntry](scheduleUpdateQuery)

func (ss ScheduleStorage) Update(entry radio.ScheduleEntry) error {
	const op errors.Op = "mariadb/ScheduleStorage.Update"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	// entry.Owner can be nil but that breaks the named query above, so we insert
	// a fake user with id 0 if it's nil
	if entry.Owner == nil {
		entry.Owner = &radio.User{}
	}

	_, err := sqlx.NamedExec(handle, scheduleUpdateQuery, entry)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

const scheduleHistoryQuery = `
SELECT
	schedule.id AS id,
	schedule.weekday AS weekday,
	schedule.text AS text,
	schedule.updated_at AS updated_at,
	schedule.notification AS notification,

	ub.id AS 'updatedby.id',
	ub.user AS 'updatedby.username',
	ub.pass AS 'updatedby.password',
	COALESCE(ub.email, '') AS 'updatedby.email',
	ub.ip AS 'updatedby.ip',
	ub.updated_at AS 'updatedby.updated_at',
	ub.deleted_at AS 'updatedby.deleted_at',
	ub.created_at AS 'updatedby.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=ub.id) AS 'updatedby.userpermissions',
	COALESCE(ub_djs.id, 0) AS 'updatedby.dj.id',
	COALESCE(ub_djs.regex, '') AS 'updatedby.dj.regex',
	COALESCE(ub_djs.djname, '') AS 'updatedby.dj.name',
	COALESCE(ub_djs.djtext, '') AS 'updatedby.dj.text',
	COALESCE(ub_djs.djimage, '') AS 'updatedby.dj.image',
	COALESCE(ub_djs.visible, 0) AS 'updatedby.dj.visible',
	COALESCE(ub_djs.priority, 0) AS 'updatedby.dj.priority',
	COALESCE(ub_djs.role, '') AS 'updatedby.dj.role',
	COALESCE(ub_djs.css, '') AS 'updatedby.dj.css',
	COALESCE(ub_djs.djcolor, '') AS 'updatedby.dj.color',
	COALESCE(ub_djs.theme_name, '') AS 'updatedby.dj.theme',

	COALESCE(ow.id, 0) AS 'owner.id',
	COALESCE(ow.user, '') AS 'owner.username',
	COALESCE(ow.pass, '') AS 'owner.password',
	COALESCE(ow.email, '') AS 'owner.email',
	COALESCE(ow.ip, '') AS 'owner.ip',
	COALESCE(ow.updated_at, TIMESTAMP('0000-00-00')) AS 'owner.updated_at',
	COALESCE(ow.deleted_at, TIMESTAMP('0000-00-00')) AS 'owner.deleted_at',
	COALESCE(ow.created_at, TIMESTAMP('0000-00-00')) AS 'owner.created_at',
	(SELECT group_concat(permission) FROM permissions WHERE user_id=ow.id) AS 'owner.userpermissions',
	COALESCE(ow_djs.id, 0) AS 'owner.dj.id',
	COALESCE(ow_djs.regex, '') AS 'owner.dj.regex',
	COALESCE(ow_djs.djname, '') AS 'owner.dj.name',
	COALESCE(ow_djs.djtext, '') AS 'owner.dj.text',
	COALESCE(ow_djs.djimage, '') AS 'owner.dj.image',
	COALESCE(ow_djs.visible, 0) AS 'owner.dj.visible',
	COALESCE(ow_djs.priority, 0) AS 'owner.dj.priority',
	COALESCE(ow_djs.role, '') AS 'owner.dj.role',
	COALESCE(ow_djs.css, '') AS 'owner.dj.css',
	COALESCE(ow_djs.djcolor, '') AS 'owner.dj.color',
	COALESCE(ow_djs.theme_name, '') AS 'owner.dj.theme'
FROM
	schedule
JOIN
	users AS ub ON schedule.updated_by = ub.id
LEFT JOIN
	djs AS ub_djs ON ub.djid = ub_djs.id
LEFT JOIN
	users AS ow ON schedule.owner = ow.id
LEFT JOIN
	djs AS ow_djs ON ow.djid = ow_djs.id
WHERE
	weekday=:weekday
ORDER BY
	updated_by DESC, id DESC
LIMIT :limit OFFSET :offset;
`

var _ = CheckQuery[ScheduleHistoryParams](scheduleHistoryQuery)

type ScheduleHistoryParams struct {
	Weekday radio.ScheduleDay
	Limit   int64
	Offset  int64
}

func (ss ScheduleStorage) History(day radio.ScheduleDay, limit, offset int64) ([]radio.ScheduleEntry, error) {
	const op errors.Op = "mariadb/ScheduleStorage.History"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var history []radio.ScheduleEntry

	err := handle.Select(&history, scheduleHistoryQuery, ScheduleHistoryParams{
		Weekday: day,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, errors.E(op, err)
	}
	return history, nil
}
