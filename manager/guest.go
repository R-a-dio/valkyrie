package manager

import (
	"context"
	"strings"
	"sync"
	"time"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/R-a-dio/valkyrie/config"
	"github.com/R-a-dio/valkyrie/errors"
	"github.com/rs/zerolog"
)

const (
	GUEST_PASSWORD_LENGTH  = 20
	GUEST_PREFIX           = "guest_"
	GUEST_EXPIRE_LOOP_TICK = time.Minute * 5
	GUEST_THREAD_LIMIT     = 3
	GUEST_KILL_LIMIT       = 3
)

var _ radio.GuestService = &GuestService{}

type GuestService struct {
	us           radio.UserStorageService
	proxyAddress func() string

	mu         sync.Mutex
	Authorized map[GuestNick]*Guest
}

func NewGuestService(ctx context.Context, cfg config.Config, us radio.UserStorageService) (*GuestService, error) {
	const op errors.Op = "manager/NewGuestService"

	gs := &GuestService{
		us: us,
		proxyAddress: config.Value(cfg, func(c config.Config) string {
			return c.Conf().Manager.GuestProxyAddr.String()
		}),
		Authorized: map[GuestNick]*Guest{},
	}

	if gs.proxyAddress() == "" {
		return nil, errors.E(op, "Manager.GuestProxyAddr is not configured")
	}

	go gs.loopExpire(ctx, time.Duration(cfg.Conf().Manager.GuestAuthPeriod))
	return gs, nil
}

func (gs *GuestService) username(nick GuestNick) (username string) {
	return GUEST_PREFIX + nick
}

func (gs *GuestService) getOrCreateUser(ctx context.Context, username string, passwd string) (*radio.User, error) {
	const op errors.Op = "manager/GuestService.getOrCreateUser"
	user, err := gs.us.User(ctx).Get(username)
	if err == nil {
		// success straight away
		return user, nil
	}

	if !errors.Is(errors.UserUnknown, err) {
		return nil, errors.E(op, err)
	}

	return gs.createUser(ctx, username, passwd)
}

func (gs *GuestService) createUser(ctx context.Context, username string, passwd string) (*radio.User, error) {
	const op errors.Op = "manager/GuestService.createUser"

	us, tx, err := gs.us.UserTx(ctx, nil)
	if err != nil {
		return nil, errors.E(op, err)
	}
	defer tx.Rollback()

	hashed, err := radio.GenerateHashFromPassword(passwd)
	if err != nil {
		return nil, errors.E(op, err)
	}

	// create the user
	user := radio.User{
		Username:        username,
		Password:        hashed,
		UserPermissions: radio.NewUserPermissions(radio.PermActive, radio.PermDJ, radio.PermDatabaseView, radio.PermGuest),
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

func (gs *GuestService) updateUserIP(ctx context.Context, user *radio.User, addr string) error {
	return nil
}

// AddGuest adds the nick given as a guest user
func (gs *GuestService) Auth(ctx context.Context, nick GuestNick) (*radio.User, string, error) {
	const op errors.Op = "manager/GuestService.Auth"
	nick = strings.ToLower(nick)

	gs.mu.Lock()
	defer gs.mu.Unlock()

	passwd, err := radio.GenerateRandomPassword(GUEST_PASSWORD_LENGTH)
	if err != nil {
		return nil, "", errors.E(op, err)
	}

	user, err := gs.getOrCreateUser(ctx, gs.username(nick), passwd)
	if err != nil {
		return nil, "", errors.E(op, err)
	}

	// check if the user IP is still up-to-date, this should always be set to
	// the guest proxy server
	if user.IP != gs.proxyAddress() {
		err := gs.updateUserIP(ctx, user, gs.proxyAddress())
		if err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to set guest users IP address")
		}
	}

	gs.Authorized[nick] = &Guest{
		Nick:     nick,
		User:     user,
		AuthTime: time.Now(),
	}
	return user, "", nil
}

func (gs *GuestService) Deauth(ctx context.Context, nick GuestNick) error {
	const op errors.Op = "manager/GuestService.Deauth"
	nick = strings.ToLower(nick)

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
	const op errors.Op = "manager/GuestService.CanDo"
	nick = strings.ToLower(nick)

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