package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/util"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pendingTest struct {
	// Name to use for the test
	Name string
	// User to use for the request
	User radio.User
	// Song to use for the request
	PendingSong radio.PendingSong
	// Form we expect back
	ExpectedForm *PendingForm
	// Status to use in the form submission
	Status radio.SubmissionStatus
	// ShouldRollback indicates if this test should be a rollback
	ShouldRollback bool

	// Return values for SubmissionStorage.GetSubmission
	GetSubmissionRet *radio.PendingSong
	GetSubmissionErr error

	// Return tx for SubmissionsTx
	TxFunc func(*testing.T) radio.StorageTx

	// Return values for SubmissionStorage.InsertPostPending
	InsertPostPendingErr error

	// Return values for SubmissionStorage.RemoveSubmission
	RemoveSubmissionErr error

	// Return values for TrackStorage.Insert
	InsertRet radio.TrackID
	InsertErr error

	// Return values for TrackStorage.UpdateMetadata
	UpdateMetadataErr error

	// Return values for TrackStorage.Get
	GetRet *radio.Song
	GetErr error
}

var genericUser = &radio.User{
	Username: "Wessie",
	UserPermissions: radio.UserPermissions{
		radio.PermActive:      struct{}{},
		radio.PermPendingView: struct{}{},
		radio.PermPendingEdit: struct{}{},
	},
}

var genericPendingSong = radio.PendingSong{
	ID:       50,
	Status:   radio.SubmissionAccepted,
	Artist:   "test-artist",
	Title:    "test-title",
	Album:    "test-album",
	Tags:     "test-tags",
	Reason:   "test-reason",
	FilePath: "test-filepath.mp3",
}

var pendingTests = []pendingTest{
	{
		Name:      "Accept Happy Path",
		Status:    radio.SubmissionAccepted,
		TxFunc:    mocks.CommitTx,
		InsertRet: 50,
	},
	{
		Name:           "Accept Fail Insert",
		Status:         radio.SubmissionAccepted,
		ShouldRollback: true,
		TxFunc:         mocks.RollbackTx,
		InsertErr:      errors.E(errors.Testing),
	},
	{
		Name:                "Accept Fail RemoveSubmission",
		Status:              radio.SubmissionAccepted,
		ShouldRollback:      true,
		TxFunc:              mocks.RollbackTx,
		RemoveSubmissionErr: errors.E(errors.Testing),
	},
	{
		Name:                 "Accept Fail InsertPostPending",
		Status:               radio.SubmissionAccepted,
		ShouldRollback:       true,
		TxFunc:               mocks.RollbackTx,
		InsertPostPendingErr: errors.E(errors.Testing),
	},
	{
		Name:             "Accept Fail GetSubmission",
		Status:           radio.SubmissionAccepted,
		ShouldRollback:   true,
		ExpectedForm:     &PendingForm{},
		TxFunc:           mocks.NotUsedTx,
		GetSubmissionErr: errors.E(errors.Testing),
	},
	{
		Name:   "Decline Happy Path",
		TxFunc: mocks.CommitTx,
		Status: radio.SubmissionDeclined,
	},
	{
		Name:           "Decline Fail RemoveSubmission",
		ShouldRollback: true,
		TxFunc:         mocks.CommitErrTx,
		Status:         radio.SubmissionDeclined,
	},
	{
		Name:   "Replace Happy Path",
		Status: radio.SubmissionReplacement,
		TxFunc: mocks.CommitTx,
		PendingSong: radio.PendingSong{
			FilePath:      "testfile.mp3",
			ReplacementID: 50,
		},
		GetRet: &radio.Song{
			DatabaseTrack: &radio.DatabaseTrack{
				FilePath: "50_random.mp3",
			},
		},
	},
}

func TestPostPending(t *testing.T) {
	for _, test := range pendingTests {
		t.Run(test.Name, func(t *testing.T) {
			// setup defaults
			if test.User.Username == "" {
				test.User = *genericUser
			}
			if test.PendingSong == *new(radio.PendingSong) {
				test.PendingSong = genericPendingSong
			}
			test.PendingSong.Status = test.Status
			var newSong radio.Song

			// setup mocks
			storage := &mocks.StorageServiceMock{}
			storage.SubmissionsFunc = func(contextMoqParam context.Context) radio.SubmissionStorage {
				return &mocks.SubmissionStorageMock{
					GetSubmissionFunc: func(submissionID radio.SubmissionID) (*radio.PendingSong, error) {
						assert.Equal(t, test.PendingSong.ID, submissionID)
						if test.GetSubmissionRet == nil {
							return &test.PendingSong, test.GetSubmissionErr
						}
						return test.GetSubmissionRet, test.GetSubmissionErr
					},
				}
			}
			storage.SubmissionsTxFunc = func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.SubmissionStorage, radio.StorageTx, error) {
				return &mocks.SubmissionStorageMock{
					InsertPostPendingFunc: func(pendingSong radio.PendingSong) error {
						return test.InsertPostPendingErr
					},
					RemoveSubmissionFunc: func(submissionID radio.SubmissionID) error {
						assert.Equal(t, test.PendingSong.ID, submissionID)
						return test.RemoveSubmissionErr
					},
				}, test.TxFunc(t), nil
			}
			storage.TrackTxFunc = func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.TrackStorage, radio.StorageTx, error) {
				return &mocks.TrackStorageMock{
					InsertFunc: func(song radio.Song) (radio.TrackID, error) {
						return test.InsertRet, test.InsertErr
					},
					UpdateMetadataFunc: func(song radio.Song) error {
						switch test.Status {
						case radio.SubmissionAccepted:
							assert.Equal(t, test.InsertRet, song.TrackID)
						case radio.SubmissionReplacement:
							assert.Equal(t, test.GetRet.TrackID, song.TrackID)
						}

						newSong = song
						return test.UpdateMetadataErr
					},
					GetFunc: func(id radio.TrackID) (*radio.Song, error) {
						return test.GetRet, test.GetErr
					},
				}, mocks.NotUsedTx(t), nil
			}

			// setup a config file
			cfg, err := config.LoadFile()
			require.NoError(t, err)

			// setup a fake filesystem
			path := pendingPath(cfg)
			fs := afero.NewMemMapFs()
			// make our pending directory, this will also make the musicpath
			require.NoError(t, fs.MkdirAll(path, 0775))

			// write a bit of data to the file we've "uploaded"
			testFileContents := []byte("a music file")
			require.NoError(t, afero.WriteFile(fs,
				filepath.Join(path, test.PendingSong.FilePath),
				testFileContents,
				0775),
			)

			// also write a bit of data to the file returned by TrackStorage.Get if one exists
			getFileContents := []byte("these were already there")
			if test.GetRet != nil {
				require.NoError(t, afero.WriteFile(fs,
					util.AbsolutePath(cfg.Conf().MusicPath, test.GetRet.FilePath),
					getFileContents,
					0775),
				)
			}

			// setup state
			state := State{
				Storage: storage,
				Config:  cfg,
				FS:      fs,
			}

			var formWeSend = PendingForm{
				PendingSong: test.PendingSong,
				Errors:      make(map[string]string),
			}

			// setup http request
			body := strings.NewReader(formWeSend.ToValues().Encode())
			req := httptest.NewRequest(http.MethodPost, "/admin/pending", body)
			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			req = middleware.RequestWithUser(req, &test.User)
			w := httptest.NewRecorder()

			form, err := state.postPending(w, req)

			if test.ShouldRollback {
				// should only rollback if there was an error
				assert.Error(t, err)
				// we shouldn't have an accepted song after a rollback
				assert.Nil(t, form.AcceptedSong)
				// the form is updated with a ReviewedAt field when it passes through
				// parsing but we don't have the exact value so we swap it over to the
				// "input" form before comparing.
				formWeSend.ReviewedAt = form.ReviewedAt
				// check if we have an expected form return value
				if test.ExpectedForm == nil {
					// if not try the form we send over the wire
					assert.EqualValues(t, formWeSend, form, "")
				} else {
					// else use the expected form for comparison
					assert.EqualValues(t, *test.ExpectedForm, form, "")
				}
			} else {
				// should have no error after a successful commit
				assert.NoError(t, err)

				switch test.PendingSong.Status {
				case radio.SubmissionAccepted:
					// our returned form should have an accepted song after no rollback
					assert.NotNil(t, form.AcceptedSong)
					// and we should have a file at the filepath given to the database layer
					contents, err := afero.ReadFile(fs, util.AbsolutePath(cfg.Conf().MusicPath, newSong.FilePath))
					if assert.NoError(t, err) {
						assert.Equal(t, testFileContents, contents)
					}
				case radio.SubmissionDeclined:
					assert.Zero(t, len(storage.TrackTxCalls()), "decline path should not have a TrackStorage instance")

					// a decline should not have an accepted song
					assert.Nil(t, form.AcceptedSong)
					// and the file should be gone
					fullPath := util.AbsolutePath(pendingPath(cfg), form.FilePath)
					ok, err := afero.Exists(fs, fullPath)
					if assert.NoError(t, err) {
						assert.False(t, ok, "file should no longer exist", fullPath)
					}
				case radio.SubmissionReplacement:
					contents, err := afero.ReadFile(fs, util.AbsolutePath(cfg.Conf().MusicPath, test.GetRet.FilePath))
					if assert.NoError(t, err) {
						assert.Equal(t, testFileContents, contents)
					}
				}
			}
		})
	}
}

func TestPendingFormRoundTrip(t *testing.T) {
	arbitraties := arbitrary.DefaultArbitraries()

	pendingSongGen := gen.Struct(reflect.TypeOf(radio.PendingSong{}), map[string]gopter.Gen{
		"ID":            arbitraties.GenForType(reflect.TypeOf(radio.SubmissionID(0))),
		"Status":        gen.OneConstOf(radio.SubmissionAccepted, radio.SubmissionDeclined, radio.SubmissionReplacement),
		"Title":         gen.AnyString(),
		"Artist":        gen.AnyString(),
		"Album":         gen.AnyString(),
		"Tags":          gen.AnyString(),
		"ReplacementID": arbitraties.GenForType(reflect.TypeOf(radio.TrackID(0))),
		"Reason":        gen.AnyString(),
	})
	_ = pendingSongGen

	properties := gopter.NewProperties(nil)

	properties.Property("form should roundtrip", prop.ForAll(
		func(song radio.PendingSong) bool {
			in := PendingForm{PendingSong: song}
			var out PendingForm

			out.Update(in.ToValues())

			// ReviewedAt is set to Now on Update so copy that over to our in
			// so we can actually compare them
			in.ReviewedAt = out.ReviewedAt

			return out.PendingSong == in.PendingSong
		}, pendingSongGen,
	))
	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
