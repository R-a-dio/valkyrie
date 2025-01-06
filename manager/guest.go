package manager

import (
	"context"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
)

const (
	GUEST_PREFIX           = "guest_"
	GUEST_EXPIRE_LOOP_TICK = time.Minute * 5
	GUEST_THREAD_LIMIT     = 3
	GUEST_KILL_LIMIT       = 3
)

var _ radio.GuestService = &GuestService{}

type GuestService struct {
	us         radio.UserStorageService
	mu         sync.Mutex
	Authorized map[GuestNick]*Guest
}

func NewGuestService(ctx context.Context, cfg config.Config, us radio.UserStorageService) *GuestService {
	gs := &GuestService{
		us:         us,
		Authorized: map[GuestNick]*Guest{},
	}

	// TODO: add timeout from config
	go gs.loopExpire(ctx, time.Hour*24)
	return gs
}

func (gs *GuestService) username(nick GuestNick) (username string) {
	return GUEST_PREFIX + nick
}

func (gs *GuestService) getOrCreateUser(ctx context.Context, username string) (*radio.User, error) {
	const op errors.Op = "manager/GuestSystem.getOrCreateUser"
	user, err := gs.us.User(ctx).Get(username)
	if err == nil {
		// success straight away
		return user, nil
	}

	if !errors.Is(errors.UserUnknown, err) {
		return nil, errors.E(op, err)
	}

	return gs.createUser(ctx, username)
}

func (gs *GuestService) createUser(ctx context.Context, username string) (*radio.User, error) {
	const op errors.Op = "manager/GuestSystem.createUser"

	us, tx, err := gs.us.UserTx(ctx, nil)
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer tx.Rollback()

	// create the user
	user := radio.User{
		Username:        username,
		UserPermissions: radio.NewUserPermissions(radio.PermActive, radio.PermDJ, radio.PermDatabaseView),
		CreatedAt:       time.Now(),
	}

	user.ID, err = gs.us.User(ctx).Create(user)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// and the DJ
	user.DJ = radio.DJ{
		Name:    username,
		Visible: false,
	}

	user.DJ.ID, err = us.CreateDJ(user, user.DJ)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// commit the transaction
	if err = tx.Commit(); err != nil {
		return nil, errors.E(op, err)
	}
	return &user, nil
}

// AddGuest adds the nick given as a guest user
func (gs *GuestService) Auth(ctx context.Context, nick GuestNick) (*radio.User, error) {
	const op errors.Op = "manager/GuestSystem.Auth"
	gs.mu.Lock()
	defer gs.mu.Unlock()

	user, err := gs.getOrCreateUser(ctx, gs.username(nick))
	if err != nil {
		return nil, errors.E(op, err)
	}

	gs.Authorized[nick] = &Guest{
		Nick:     nick,
		User:     user,
		AuthTime: time.Now(),
	}
	return user, nil
}

func (gs *GuestService) Deauth(ctx context.Context, nick GuestNick) error {
	const op errors.Op = "manager/GuestSystem.Deauth"
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// check if nick even exists in the list
	if _, ok := gs.Authorized[nick]; !ok {
		return nil
	}

	// remove the nick from the authorization list
	delete(gs.Authorized, nick)

	// retrieve the user from storage
	user, err := gs.us.User(ctx).Get(gs.username(nick))
	if err != nil {
		return errors.E(op, err)
	}

	// remove their active status
	delete(user.UserPermissions, radio.PermActive)
	// and update storage with it
	_, err = gs.us.User(ctx).Update(*user)
	if err != nil {
		return errors.E(op, err)
	}

	return nil
}

func (gs *GuestService) CanDo(ctx context.Context, nick GuestNick, action radio.GuestAction) (ok bool, err error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	guest, ok := gs.Authorized[nick]
	if !ok {
		return false, nil
	}

	// some actions are limited per auth
	switch action {
	case radio.GuestKill:
		if guest.KillAttempts >= GUEST_KILL_LIMIT {
			return false, nil
		}
		guest.KillAttempts++
	case radio.GuestThread:
		if guest.ThreadSets >= GUEST_THREAD_LIMIT {
			return false, nil
		}
		guest.ThreadSets++
	}
	return true, nil
}

func (gs *GuestService) loopExpire(ctx context.Context, timeout time.Duration) {
	ticker := time.NewTicker(GUEST_EXPIRE_LOOP_TICK)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gs.doExpire(timeout)
		case <-ctx.Done():
			return
		}
	}
}

func (gs *GuestService) doExpire(timeout time.Duration) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	for nick, guest := range gs.Authorized {
		if time.Since(guest.AuthTime) < timeout {
			continue
		}

		delete(gs.Authorized, nick)
	}
}

type GuestNick = string

type Guest struct {
	Nick GuestNick

	User *radio.User
	// AuthTime is the time this guest got authorized
	AuthTime time.Time
	// ThreadSets is the amount of times this guest has used their .thread privilege
	ThreadSets int
	// KillAttempts is the amount of times this guest has used their .kill privilege
	KillAttempts int
}
