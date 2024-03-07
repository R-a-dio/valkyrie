package mariadb

import radio "github.com/R-a-dio/valkyrie"

type ScheduleStorage struct {
	handle handle
}

func (ss ScheduleStorage) Latest() ([]radio.ScheduleEntry, error) {
	return nil, nil
}

func (ss ScheduleStorage) Update(entry radio.ScheduleEntry) error {
	return nil
}

func (ss ScheduleStorage) History(day radio.ScheduleDay, limit, offset int64) ([]radio.ScheduleEntry, error) {
	return nil, nil
}
