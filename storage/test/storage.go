package storagetest

import (
	"context"
	"os"
	"sync"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
)

type TestSetup interface {
	Setup(context.Context) error
	CreateStorage(ctx context.Context, name string) (radio.StorageService, error)
	TearDown(context.Context) error
}

func RunTests(t *testing.T, s TestSetup) {
	suite.Run(t, NewSuite(s))
}

func NewSuite(ts TestSetup) suite.TestingSuite {
	return &Suite{
		ToBeTested: ts,
		storageMap: make(map[string]radio.StorageService),
	}
}

type Suite struct {
	suite.Suite

	ctx        context.Context
	ToBeTested TestSetup

	storageMu  sync.Mutex
	storageMap map[string]radio.StorageService
}

func (suite *Suite) SetupSuite() {
	suite.ctx = zerolog.New(os.Stdout).Level(zerolog.ErrorLevel).WithContext(context.Background())
	suite.ctx = PutT(suite.ctx, suite.T())

	err := suite.ToBeTested.Setup(suite.ctx)
	if err != nil {
		suite.T().Fatal("failed to setup ToBeTested:", err)
	}
}

func (suite *Suite) TearDownSuite() {
	err := suite.ToBeTested.TearDown(suite.ctx)
	if err != nil {
		suite.T().Fatal("failed to teardown ToBeTested:", err)
	}
}

func (suite *Suite) BeforeTest(suiteName, testName string) {
	s, err := suite.ToBeTested.CreateStorage(suite.ctx, suiteName+testName)
	if err != nil {
		suite.T().Error("failed to setup test", err)
		return
	}

	suite.storageMu.Lock()
	suite.storageMap[suite.T().Name()] = s
	suite.storageMu.Unlock()
}

func (suite *Suite) Storage(t *testing.T) radio.StorageService {
	suite.storageMu.Lock()
	defer suite.storageMu.Unlock()

	return suite.storageMap[t.Name()]
}

type testingKey struct{}

func CtxT(ctx context.Context) testing.TB {
	return ctx.Value(testingKey{}).(testing.TB)
}

func PutT(ctx context.Context, t testing.TB) context.Context {
	return context.WithValue(ctx, testingKey{}, t)
}
