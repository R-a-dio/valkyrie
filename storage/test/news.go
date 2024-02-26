package storagetest

import (
	"math/rand"
	"reflect"
	"testing/quick"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/errors"
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

func (suite *Suite) TestNewsStorageDeleteNoExist() {
	ns := suite.Storage.News(suite.ctx)

	// some random id that shouldn't exist
	err := ns.Delete(getNewsID())
	suite.NotNil(err)
	// make sure the right error kind is returned
	suite.True(errors.Is(errors.NewsUnknown, err))
}

func (suite *Suite) TestNewsStorageDeleteExist() {
	ns := suite.Storage.News(suite.ctx)

	post := createDummyNewsPost("delete test")
	id, err := ns.Create(post)
	if !suite.NoError(err) {
		return
	}
	// update our ID
	post.ID = id

	// delete it, we don't actually delete news posts though and they're just
	// marked as deleted by the DeletedAt field
	err = ns.Delete(id)
	if !suite.NoError(err) {
		return
	}

	// so grab it back from storage and see if DeletedAt is set
	got, err := ns.Get(id)
	if suite.NoError(err) {
		suite.NotNil(got.DeletedAt)
	}
}

func (suite *Suite) TestNewsSimpleCreateAndGet() {
	ns := suite.Storage.News(suite.ctx)

	post := createDummyNewsPost("simple create test")

	id, err := ns.Create(post)
	if suite.NoError(err) {
		suite.NotZero(id)
	}
	// update our id
	post.ID = id

	// then try and grab it back from storage and compare it
	got, err := ns.Get(id)
	if suite.NoError(err) {
		// check normal fields
		suite.EqualExportedValues(post, *got)
		// check time fields
		suite.WithinDuration(post.CreatedAt, got.CreatedAt, time.Second)
		suite.WithinDuration(*post.UpdatedAt, *got.UpdatedAt, time.Second)
		suite.Nil(got.DeletedAt) // shouldn't be set
	}
}

func (suite *Suite) TestNewsWithUser() {
	ns := suite.Storage.News(suite.ctx)
	us := suite.Storage.User(suite.ctx)

	user := OneOff[radio.User](genUser())
	// we want to make a new user so set the id to zero
	user.ID = 0

	// insert our user
	user, err := us.UpdateUser(user)
	suite.NoError(err)

	post := createDummyNewsPost("news with user")
	// set out user id
	post.User.ID = user.ID

	id, err := ns.Create(post)
	if suite.NoError(err) {
		suite.NotZero(id)
	}

	// now try to grab the news back and see if we get the user along with it
	got, err := ns.Get(id)
	if suite.NoError(err) {
		suite.NotNil(got)
	}

	suite.EqualExportedValues(user, got.User)
}

func createDummyNewsPost(pre string) radio.NewsPost {
	now := time.Now()

	return radio.NewsPost{
		ID:     0,
		Title:  pre + "title",
		Header: pre + "header",
		Body:   pre + "body",
		User: &radio.User{
			ID: 1,
		},
		DeletedAt: nil,
		CreatedAt: now,
		UpdatedAt: &now,
		Private:   false,
	}
}
