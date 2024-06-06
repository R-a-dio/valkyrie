package storagetest

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"

	radio "github.com/R-a-dio/valkyrie"
	"github.com/rs/zerolog"
)

type TestSetup interface {
	Setup(context.Context) error
	CreateStorage(ctx context.Context) radio.StorageService
	TearDown(context.Context) error
}

func RunTests(t *testing.T, s TestSetup) {
	ctx := zerolog.New(os.Stdout).Level(zerolog.ErrorLevel).WithContext(context.Background())
	ctx = PutT(ctx, t)

	// make sure TearDown is always called
	defer func() {
		err := s.TearDown(ctx)
		if err != nil {
			t.Error("failed to teardown", err)
		}
	}()

	// do test Setup
	err := s.Setup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	suite := NewSuite(ctx, s)

	tests := gatherAllTests(suite)

	t.Run("Storage", func(t *testing.T) {
		for name, fn := range tests {
			fn := fn

			t.Run(name, func(t *testing.T) {
				err := suite.BeforeTest(t.Name())
				if err != nil {
					t.Error("failed test setup:", err)
					return
				}
				defer suite.AfterTest(t.Name())

				fn(t)
			})
		}
	})
}

type testFn func(t *testing.T)

func gatherAllTests(suite *Suite) map[string]testFn {
	var tests = map[string]testFn{}
	rv := reflect.ValueOf(suite)
	for i := 0; i < rv.NumMethod(); i++ {
		mv := rv.Method(i)
		mt := mv.Type()

		if mt.NumIn() != 1 || mt.NumOut() != 0 {
			continue
		}

		if mt.In(0).String() != "*testing.T" {
			continue
		}

		tests[rv.Type().Method(i).Name] = func(t *testing.T) {
			mv.Call([]reflect.Value{reflect.ValueOf(t)})
		}
	}
	return tests
}

func NewSuite(ctx context.Context, ts TestSetup) *Suite {
	return &Suite{
		ctx:        ctx,
		ToBeTested: ts,
		storageMap: make(map[string]radio.StorageService),
	}
}

type Suite struct {
	ctx        context.Context
	ToBeTested TestSetup

	storageMu  sync.Mutex
	storageMap map[string]radio.StorageService
}

func (suite *Suite) BeforeTest(testName string) error {
	return nil
}

func (suite *Suite) AfterTest(testName string) error {
	return nil
}

func (suite *Suite) Storage(t *testing.T) radio.StorageService {
	return suite.ToBeTested.CreateStorage(suite.ctx)
}

type testingKey struct{}

func CtxT(ctx context.Context) testing.TB {
	return ctx.Value(testingKey{}).(testing.TB)
}

func PutT(ctx context.Context, t testing.TB) context.Context {
	return context.WithValue(ctx, testingKey{}, t)
}
