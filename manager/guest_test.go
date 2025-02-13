package manager

import (
	"context"
	"net/url"
	"testing"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/util/eventstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuestExpire(t *testing.T) {
	ctx := context.Background()
	cfg := config.TestConfig()

	us := &mocks.UserStorageMock{
		GetFunc: func(name string) (*radio.User, error) {
			return &radio.User{Username: name}, nil
		},
		UpdateFunc: func(user radio.User) (radio.User, error) {
			return user, nil
		},
	}
	uss := &mocks.UserStorageServiceMock{
		UserFunc: func(contextMoqParam context.Context) radio.UserStorage {
			return us
		},
		UserTxFunc: func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.UserStorage, radio.StorageTx, error) {
			return us, mocks.CommitTx(t), nil
		},
	}

	es := eventstream.NewEventStream[*radio.User](nil)
	m := &mocks.ManagerServiceMock{
		CurrentUserFunc: func(contextMoqParam context.Context) (eventstream.Stream[*radio.User], error) {
			return es.SubStream(contextMoqParam), nil
		},
	}

	gs, err := NewGuestService(ctx, cfg, m, uss)
	require.NoError(t, err)

	nick := "test-user"
	gs.Auth(ctx, nick)

	ok, err := gs.CanDo(ctx, nick, radio.GuestNone)
	if assert.NoError(t, err) {
		assert.True(t, ok)
	}

	assert.Len(t, gs.Authorized, 1)

	// do an expire with zero timeout, so basically everyone should expire
	gs.doExpire(0)
	ok, err = gs.CanDo(ctx, nick, radio.GuestNone)
	if assert.NoError(t, err) {
		assert.False(t, ok)
	}
	assert.Len(t, gs.Authorized, 0)

	// add a crafted guest user in that got authed an hour ago
	gs.Authorized["a-while-ago"] = &Guest{
		AuthTime: time.Now().Add(-time.Hour),
	}

	// expire everyone that went past the 30 minutes since
	gs.doExpire(time.Minute * 30)

	assert.Len(t, gs.Authorized, 0)
}

func TestGuestCanDo(t *testing.T) {
	ctx := context.Background()
	cfg := config.TestConfig()
	us := &mocks.UserStorageMock{
		GetFunc: func(name string) (*radio.User, error) {
			return &radio.User{Username: name}, nil
		},
		UpdateFunc: func(user radio.User) (radio.User, error) {
			return user, nil
		},
	}
	uss := &mocks.UserStorageServiceMock{
		UserFunc: func(contextMoqParam context.Context) radio.UserStorage {
			return us
		},
		UserTxFunc: func(contextMoqParam context.Context, storageTx radio.StorageTx) (radio.UserStorage, radio.StorageTx, error) {
			return us, mocks.CommitTx(t), nil
		},
	}

	es := eventstream.NewEventStream[*radio.User](nil)
	m := &mocks.ManagerServiceMock{
		CurrentUserFunc: func(contextMoqParam context.Context) (eventstream.Stream[*radio.User], error) {
			return es.SubStream(contextMoqParam), nil
		},
	}

	gs, err := NewGuestService(ctx, cfg, m, uss)
	require.NoError(t, err)

	nick := "test"
	ok, err := gs.CanDo(ctx, nick, radio.GuestNone)
	if assert.NoError(t, err) {
		assert.False(t, ok)
	}

	gs.Auth(ctx, nick)
	ok, err = gs.CanDo(ctx, nick, radio.GuestNone)
	if assert.NoError(t, err) {
		assert.True(t, ok)
	}

	ok, err = gs.CanDo(ctx, "not-nick", radio.GuestNone)
	if assert.NoError(t, err) {
		assert.False(t, ok)
	}
}

func TestResolveGuestProxyAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ip, err := resolveGuestProxyAddr(ctx, mustURL("//localhost:80"))
	require.NoError(t, err)
	require.NotZero(t, ip)

	ip, err = resolveGuestProxyAddr(ctx, mustURL("//127.0.0.1:80"))
	require.NoError(t, err)
	require.NotZero(t, ip)
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
