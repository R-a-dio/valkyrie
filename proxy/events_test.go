package proxy

import (
	"context"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/rs/xid"
	"github.com/stretchr/testify/require"
)

func TestEventProxyStatus(t *testing.T) {
	ctx := context.Background()
	eps := newEventProxyStatus()

	newsc := func(id radio.UserID) *SourceClient {
		return &SourceClient{
			ID: radio.SourceID{ID: xid.New()},
			User: radio.User{
				ID: id,
			},
		}
	}
	t.Run("nil arguments", func(t *testing.T) {
		eps.AddSource(ctx, nil)
		eps.RemoveSource(ctx, nil)
		eps.UpdateLive(ctx, nil)
	})

	t.Run("add and remove", func(t *testing.T) {
		sc := newsc(5)

		eps.AddSource(ctx, sc)
		require.NotNil(t, eps.UserInfo[sc.User.ID])

		eps.RemoveSource(ctx, sc)
		require.Nil(t, eps.UserInfo[sc.User.ID])
	})

	t.Run("orphans", func(t *testing.T) {
		t.Run("remove and add", func(t *testing.T) {
			sc := newsc(10)
			eps.RemoveSource(ctx, sc)

			eps.AddSource(ctx, sc)
			require.Nil(t, eps.UserInfo[sc.User.ID])
		})

		t.Run("live and add", func(t *testing.T) {
			sc := newsc(50)
			eps.UpdateLive(ctx, sc)
			eps.AddSource(ctx, sc)

			require.True(t, eps.UserInfo[sc.User.ID].Conns[0].IsLive)
			eps.RemoveSource(ctx, sc)
		})

		t.Run("remove, live and add", func(t *testing.T) {
			sc := newsc(55)
			eps.RemoveSource(ctx, sc)
			eps.UpdateLive(ctx, sc)
			eps.AddSource(ctx, sc)
			require.Nil(t, eps.UserInfo[sc.User.ID])
		})
	})

	require.Empty(t, eps.orphans, "orphans should be empty after tests")
	require.Empty(t, eps.UserInfo, "users should be empty after tests")
}
