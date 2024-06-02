package mariadb

import (
	"database/sql"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

// SubmissionStorage implements radio.SubmissionStorage
type SubmissionStorage struct {
	handle handle
}

type adjustedPendingSong struct {
	NullTrackID *radio.TrackID
	NullReason  *string
	Metadata    string

	radio.PendingSong
}

const submissionInsertPostPendingQuery = `
INSERT INTO
	postpending (
		trackid,
		meta,
		ip,
		accepted,
		time,
		reason,
		good_upload
	) VALUES (
		:nulltrackid,
		:metadata,
		:useridentifier,
		:status,
		:reviewedat,
		:nullreason,
		:goodupload
	)
`

func (ss SubmissionStorage) InsertPostPending(pend radio.PendingSong) error {
	const op errors.Op = "mariadb/SubmissionStorage.InsertPostPending"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	adjusted := adjustedPendingSong{
		Metadata:    pend.Metadata(),
		PendingSong: pend,
	}

	if pend.Reason != "" {
		adjusted.NullReason = &pend.Reason
	}
	if pend.AcceptedSong != nil {
		adjusted.NullTrackID = &pend.AcceptedSong.TrackID
	}

	_, err := sqlx.NamedExec(handle, submissionInsertPostPendingQuery, adjusted)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (ss SubmissionStorage) RemoveSubmission(id radio.SubmissionID) error {
	const op errors.Op = "mariadb/SubmissionStorage.RemoveSubmission"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var query = `DELETE FROM pending WHERE id=?;`

	_, err := handle.Exec(query, id)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// LastSubmissionTime implements radio.SubmissionStorage
func (ss SubmissionStorage) LastSubmissionTime(identifier string) (time.Time, error) {
	const op errors.Op = "mariadb/SubmissionStorage.LastSubmissionTime"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var t time.Time

	query := "SELECT time FROM uploadtime WHERE ip=? ORDER BY time DESC LIMIT 1;"

	err := sqlx.Get(handle, &t, query, identifier)
	if err == sql.ErrNoRows { // no rows means never uploaded, so it's OK
		err = nil
	}
	if err != nil {
		return t, errors.E(op, err)
	}

	return t, nil
}

// UpdateSubmissionTime implements radio.SubmissionStorage
func (ss SubmissionStorage) UpdateSubmissionTime(identifier string) error {
	const op errors.Op = "mariadb/SubmissionStorage.UpdateSubmissionTime"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	//query := "INSERT INTO uploadtime (ip, time) VALUES (?, NOW());"
	query := `
	INSERT INTO
		uploadtime (ip, time)
	VALUES
		(?, NOW())
	ON DUPLICATE KEY UPDATE
		time = NOW();
	`

	_, err := handle.Exec(query, identifier)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

func (ss SubmissionStorage) SubmissionStats(identifier string) (radio.SubmissionStats, error) {
	const op errors.Op = "mariadb/SubmissionStorage.SubmissionStats"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	var stats radio.SubmissionStats

	var input = struct {
		Identifier string
	}{
		Identifier: identifier,
	}

	// get the statistics first
	query := `
	SELECT 
		(SELECT count(*) FROM pending) AS current_pending,
		COALESCE(SUM(accepted >= 0), 0) AS accepted_total,
		COALESCE(SUM(accepted >= 0 && time > DATE_SUB(NOW(), INTERVAL 2 WEEK)), 0) AS accepted_last_two_weeks,
		COALESCE(SUM(accepted >= 0 && ip=:identifier), 0) AS accepted_you,
		COALESCE(SUM(accepted = 0), 0) AS declined_total,
		COALESCE(SUM(accepted = 0 && time > DATE_SUB(NOW(), INTERVAL 2 WEEK)), 0) AS declined_last_two_weeks,
		COALESCE(SUM(accepted = 0 && ip=:identifier), 0) AS declined_you,
		COALESCE((SELECT time FROM uploadtime WHERE ip=:identifier ORDER BY time DESC LIMIT 1), TIMESTAMP('0000-00-00 00::00::00')) AS last_submission_time
	FROM postpending;
	`

	rows, err := sqlx.NamedQuery(handle, query, input)
	if err != nil {
		return stats, errors.E(op, err)
	}
	defer rows.Close()

	if !rows.Next() {
		return stats, errors.E(op, sql.ErrNoRows)
	}

	if err = rows.StructScan(&stats); err != nil {
		return stats, errors.E(op, err)
	}

	// then get our recent declined and accepted songs separately
	query = `
	SELECT
		id,
		meta AS metadata,
		trackid AS acceptedsong,
		ip AS useridentifier,
		time AS reviewedat,
		reason AS declinereason
	FROM postpending WHERE accepted=? ORDER BY time DESC LIMIT 20;
	`

	stats.RecentDeclines = make([]radio.PostPendingSong, 0, 20)
	err = sqlx.Select(handle, &stats.RecentDeclines, query, radio.SubmissionDeclined)
	if err != nil {
		return stats, errors.E(op, err)
	}

	stats.RecentAccepts = make([]radio.PostPendingSong, 0, 20)
	err = sqlx.Select(handle, &stats.RecentAccepts, query, radio.SubmissionAccepted)
	if err != nil {
		return stats, errors.E(op, err)
	}

	return stats, nil
}

func (ss SubmissionStorage) InsertSubmission(song radio.PendingSong) error {
	const op errors.Op = "mariadb/SubmissionStorage.InsertSubmission"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	query := `
	INSERT INTO
		pending (
			artist,
			track, 
			album,
			path,
			comment,
			origname,
			submitter,
			submitted,
			replacement,
			bitrate,
			length,
			format,
			mode
		) VALUES (
			:artist,
			:title,
			:album,
			:filepath,
			:comment,
			:filename,
			:useridentifier,
			:submittedat,
			:replacementid,
			:bitrate,
			from_go_duration(:length),
			:format,
			:encodingmode
		);
	`

	_, err := sqlx.NamedExec(handle, query, song)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (ss SubmissionStorage) All() ([]radio.PendingSong, error) {
	const op errors.Op = "mariadb/SubmissionStorage.All"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	query := `
	SELECT
		id,
		artist,
		track AS title,
		album,
		path AS filepath,
		comment,
		origname AS filename,
		submitter AS useridentifier,
		submitted AS submittedat,
		replacement AS replacementid,
		bitrate,
		to_go_duration(length) AS length,
		format,
		mode AS encodingmode
	 FROM pending;`

	var res []radio.PendingSong

	err := sqlx.Select(handle, &res, query)
	if err != nil {
		return nil, errors.E(op, err)
	}

	for i := 0; i < len(res); i++ {
		res[i].Status = radio.SubmissionAwaitingReview
	}

	return res, nil
}

func (ss SubmissionStorage) GetSubmission(id radio.SubmissionID) (*radio.PendingSong, error) {
	const op errors.Op = "mariadb/SubmissionStorage.GetSubmission"
	handle, deferFn := ss.handle.span(op)
	defer deferFn()

	query := `
	SELECT
		id,
		artist,
		track AS title,
		album,
		path AS filepath,
		comment,
		origname AS filename,
		submitter AS useridentifier,
		submitted AS submittedat,
		replacement AS replacementid,
		bitrate,
		to_go_duration(length) AS length,
		format,
		mode AS encodingmode
	FROM pending WHERE id=?;`

	var song radio.PendingSong
	err := sqlx.Get(handle, &song, query, id)
	if err != nil {
		if errors.IsE(err, sql.ErrNoRows) {
			return nil, errors.E(op, errors.SubmissionUnknown)
		}
		return nil, errors.E(op, err)
	}
	return &song, nil
}
