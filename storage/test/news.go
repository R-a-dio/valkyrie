package storagetest

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var usedNewsID = make(map[radio.NewsPostID]struct{})

// returns a random id that hasn't been used yet for testing
func getNewsID() radio.NewsPostID {
	for {
		v, ok := quick.Value(reflect.TypeOf(radio.NewsPostID(0)), rand.New(rand.NewSource(time.Now().Unix())))
		if !ok {
			panic("failed to generate news id")
		}
		id := v.Interface().(radio.NewsPostID)

		if _, ok := usedNewsID[id]; !ok {
			usedNewsID[id] = struct{}{}
			return id
		}
	}
}

func (suite *Suite) TestNewsStorageDeleteNoExist(t *testing.T) {
	ns := suite.Storage(t).News(suite.ctx)

	// some random id that shouldn't exist
	err := ns.Delete(getNewsID())
	assert.Error(t, err)
	// make sure the right error kind is returned
	assert.True(t, errors.Is(errors.NewsUnknown, err))
}

func (suite *Suite) TestNewsStorageDeleteExist(t *testing.T) {
	ns := suite.Storage(t).News(suite.ctx)

	post := createDummyNewsPost("delete test")
	id, err := ns.Create(post)
	if !assert.NoError(t, err) {
		return
	}
	// update our ID
	post.ID = id

	// delete it, we don't actually delete news posts though and they're just
	// marked as deleted by the DeletedAt field
	err = ns.Delete(id)
	if !assert.NoError(t, err) {
		return
	}

	// so grab it back from storage and see if DeletedAt is set
	got, err := ns.Get(id)
	if assert.NoError(t, err) {
		assert.NotNil(t, got.DeletedAt)
	}
}

func (suite *Suite) TestNewsSimpleCreateAndGet(t *testing.T) {
	ns := suite.Storage(t).News(suite.ctx)

	post := createDummyNewsPost("simple create test")

	id, err := ns.Create(post)
	if assert.NoError(t, err) {
		assert.NotZero(t, id)
	}
	// update our id
	post.ID = id

	// then try and grab it back from storage and compare it
	got, err := ns.Get(id)
	if assert.NoError(t, err) {
		// check normal fields
		assert.EqualExportedValues(t, post, *got)
		// check time fields
		assert.WithinDuration(t, post.CreatedAt, got.CreatedAt, time.Second)
		assert.WithinDuration(t, *post.UpdatedAt, *got.UpdatedAt, time.Second)
		assert.Nil(t, got.DeletedAt) // shouldn't be set
	}
}

func (suite *Suite) TestNewsWithUser(t *testing.T) {
	s := suite.Storage(t)
	ns := s.News(suite.ctx)
	us := s.User(suite.ctx)

	user := OneOff[radio.User](genUser())
	// we want to make a new user so set the id to zero
	user.ID = 0

	// insert our user
	uid, err := us.Create(user)
	require.NoError(t, err)
	user.ID = uid

	post := createDummyNewsPost("news with user")
	// set out user id
	post.User.ID = user.ID

	id, err := ns.Create(post)
	if assert.NoError(t, err) {
		assert.NotZero(t, id)
	}

	// now try to grab the news back and see if we get the user along with it
	got, err := ns.Get(id)
	if assert.NoError(t, err) {
		assert.NotNil(t, got)
	}

	assert.EqualExportedValues(t, user, got.User)
}

func createDummyNewsPost(pre string) radio.NewsPost {
	now := time.Now()

	return radio.NewsPost{
		ID:     0,
		Title:  pre + "title",
		Header: pre + "header",
		Body:   pre + "body",
		User: radio.User{
			ID: 1,
		},
		DeletedAt: nil,
		CreatedAt: now,
		UpdatedAt: &now,
		Private:   false,
	}
}
