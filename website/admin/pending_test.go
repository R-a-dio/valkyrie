package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/website/middleware"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/arbitrary"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
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
}

var genericUser = &radio.User{
	Username: "Wessie",
	UserPermissions: radio.UserPermissions{
		radio.PermActive:      true,
		radio.PermPendingView: true,
		radio.PermPendingEdit: true,
	},
}

var genericPendingSong = radio.PendingSong{
	ID:     50,
	Status: radio.SubmissionAccepted,
	Artist: "test-artist",
	Title:  "test-title",
	Album:  "test-album",
	Tags:   "test-tags",
	Reason: "test-reason",
}

var pendingTests = []pendingTest{
	{
		Name:      "Happy Path",
		TxFunc:    mocks.CommitTx,
		InsertRet: 50,
	},
	{
		Name:      "Fail Insert",
		TxFunc:    mocks.RollbackTx,
		InsertErr: errors.E(errors.Testing),
	},
	{
		Name:                "Fail RemoveSubmission",
		TxFunc:              mocks.RollbackTx,
		RemoveSubmissionErr: errors.E(errors.Testing),
	},
	{
		Name:                 "Fail InsertPostPending",
		TxFunc:               mocks.RollbackTx,
		InsertPostPendingErr: errors.E(errors.Testing),
	},
	{
		Name:             "Fail GetSubmission",
		ExpectedForm:     &PendingForm{},
		TxFunc:           mocks.NotUsedTx,
		GetSubmissionErr: errors.E(errors.Testing),
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

			// setup mocks
			storage := &mocks.StorageServiceMock{}
			storage.SubmissionsFunc = func(contextMoqParam context.Context) radio.SubmissionStorage {
				return &mocks.SubmissionStorageMock{
					GetSubmissionFunc: func(submissionID radio.SubmissionID) (*radio.PendingSong, error) {
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
						return test.RemoveSubmissionErr
					},
				}, test.TxFunc(t), nil
			}
			storage.TrackTxFunc = func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.TrackStorage, radio.StorageTx, error) {
				return &mocks.TrackStorageMock{
					InsertFunc: func(song radio.Song) (radio.TrackID, error) {
						return test.InsertRet, test.InsertErr
					},
				}, mocks.NotUsedTx(t), nil
			}

			state := State{
				Storage: storage,
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

			form, _ := state.postPending(w, req)

			isRollback := test.TxFunc(t).(*mocks.StorageTxMock).CommitFunc == nil
			if isRollback {
				// our returned form should not have an accepted song after a rollback
				assert.Nil(t, form.AcceptedSong)
				// check if we have an expected form return value
				if test.ExpectedForm == nil {
					// if not try the form we send over the wire
					assert.EqualValues(t, formWeSend, form, "")
				} else {
					// else use the expected form for comparison
					assert.EqualValues(t, *test.ExpectedForm, form, "")
				}
			} else {
				// our returned form should have an accepted song after no rollback
				assert.NotNil(t, form.AcceptedSong)
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

			return out.PendingSong == in.PendingSong
		}, pendingSongGen,
	))
	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
