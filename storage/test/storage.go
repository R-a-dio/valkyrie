package storagetest

import (
	"context"
	"os"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
)

type TestSetup interface {
	Setup(context.Context) (radio.StorageService, error)
	TearDown(context.Context) error
}

func RunTests(t *testing.T, s TestSetup) {
	suite.Run(t, NewSuite(s))
}

func NewSuite(ts TestSetup) suite.TestingSuite {
	return &Suite{ToBeTested: ts}
}

type Suite struct {
	suite.Suite

	ctx        context.Context
	ToBeTested TestSetup
	Storage    radio.StorageService
}

func (suite *Suite) SetupSuite() {
	suite.ctx = zerolog.New(os.Stdout).Level(zerolog.DebugLevel).WithContext(context.Background())
	suite.ctx = context.WithValue(suite.ctx, testingKey{}, suite.T())

	s, err := suite.ToBeTested.Setup(suite.ctx)
	if err != nil {
		suite.T().Fatal("failed to setup ToBeTested:", err)
	}
	suite.Storage = s
}

func (suite *Suite) TearDownSuite() {
	err := suite.ToBeTested.TearDown(suite.ctx)
	if err != nil {
		suite.T().Fatal("failed to teardown ToBeTested:", err)
	}
}

type testingKey struct{}

func CtxT(ctx context.Context) *testing.T {
	return ctx.Value(testingKey{}).(*testing.T)
}
