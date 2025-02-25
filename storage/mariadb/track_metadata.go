package mariadb

import (
	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/jmoiron/sqlx"
)

type TrackMetadataStorage struct {
	handle handle
}

// input: radio.TrackMetadata
const TrackMetadataCreateQuery = `
INSERT INTO
	track_metadata (
		track_id,
		id,
		provider,
		album_art_path,
		primary,
		safe
	) VALUES (
		:trackid,
		:id,
		:provider,
		:albumartpath,
		:primary,
		:safe,
	);
`

func (tmds TrackMetadataStorage) Create(tmd radio.TrackMetadata) error {
	const op errors.Op = "mariadb/TrackMetadataStorage.Create"
	handle, deferFn := tmds.handle.span(op)
	defer deferFn()

	_, err := sqlx.NamedExec(handle, TrackMetadataCreateQuery, tmd)
	if err != nil {
		return errors.E(op, err)
	}
	return nil
}

// FromTrackID returns metadata associated with a TrackID
func (tmds TrackMetadataStorage) FromTrackID(tid radio.TrackID) ([]radio.TrackMetadata, error) {
	const op errors.Op = "mariadb/TrackMetadataStorage.FromTrackID"
	handle, deferFn := tmds.handle.span(op)
	defer deferFn()

	var query = "SELECT * FROM track_metadata WHERE track_id=?;"

	tmd := []radio.TrackMetadata{}

	err := sqlx.Select(handle, &tmd, query, tid)
	if err != nil {
		return tmd, errors.E(op, err)
	}
	return tmd, nil
}
