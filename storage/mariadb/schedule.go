package mariadb

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

type ScheduleStorage struct {
	handle handle
}

func (ss ScheduleStorage) Latest() ([]radio.ScheduleEntry, error) {
	const op errors.Op = "mariadb/ScheduleStorage.Latest"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `
	SELECT 
		schedule.id AS id, 
		schedule.weekday AS weekday, 
		schedule.text AS text, 
		schedule.owner AS 'user.id', 
		schedule.updated_at AS updated_at, 
		schedule.notification AS notification
	FROM 
		schedule
	`

	var schedule []radio.ScheduleEntry

	err := sqlx.Select(handle, &schedule, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	return schedule, nil
}

func (ss ScheduleStorage) Update(entry radio.ScheduleEntry) error {
	return nil
}

func (ss ScheduleStorage) History(day radio.ScheduleDay, limit, offset int64) ([]radio.ScheduleEntry, error) {
	return nil, nil
}
