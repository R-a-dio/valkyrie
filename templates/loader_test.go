package templates_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/templates"
	"golang.org/x/tools/txtar"
)

func randomFS(name string) *mocks.FSMock {
	return &mocks.FSMock{
		OpenFunc: func(name string) (fs.File, error) {
			return &mocks.FileMock{
				StatFunc: func() (fs.FileInfo, error) {
					return &mocks.FileInfoMock{
						NameFunc: func() string {
							return name
						},
						SizeFunc: func() int64 {
							return 1
						},
						ModeFunc: func() fs.FileMode {
							return fs.FileMode(1)
						},
						ModTimeFunc: func() time.Time {
							return time.Now()
						},
						IsDirFunc: func() bool {
							return false
						},
						SysFunc: func() any {
							return nil
						},
					}, nil
				},
				ReadFunc: func(bytes []byte) (int, error) {
					return -1, nil
				},
				CloseFunc: func() error { return nil },
			}, nil
		},
	}

}

// create a mock FS from a txtarchive
func txtarFS(a *txtar.Archive) fstest.MapFS {
	mfs := make(fstest.MapFS)
	for _, f := range a.Files {
		mfs[f.Name] = &fstest.MapFile{
			Data:    f.Data,
			Mode:    fs.ModePerm,
			ModTime: time.Now(),
			Sys:     nil,
		}
	}
	return mfs
}

func txtarFSFromBytes(b []byte) fstest.MapFS {
	return txtarFS(txtar.Parse(b))
}

func txtarFSFromString(b string) fstest.MapFS {
	return txtarFSFromBytes([]byte(b))
}

func TestLoadThemes(t *testing.T) {
	type args struct {
		fsys fs.FS
	}
	tests := []struct {
		name    string
		args    args
		want    templates.Themes
		wantErr bool
	}{
		{"always error", args{&mocks.FSMock{
			OpenFunc: func(name string) (fs.File, error) {
				return nil, errors.New("fucked")
			},
		}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := templates.LoadThemes(tt.args.fsys, templates.TemplateFuncs())
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadThemes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadThemes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func FuzzLoadThemes(f *testing.F) {
	testcases := []string{"wessie", "vin", "ed"}
	for _, tc := range testcases {
		f.Add(tc)
	}
	f.Fuzz(func(t *testing.T, name string) {
		rfs := randomFS(name)
		themes, err := templates.LoadThemes(rfs, templates.TemplateFuncs())
		if err == nil || themes != nil {
			t.Errorf("there should probably be an error for %s", name)
		}
	})
}

func TestExecuteTemplate(t *testing.T) {
	type args struct {
		fsys  fs.FS
		theme string
	}
	tests := []struct {
		name       string
		args       args
		wantErr    bool
		shouldExec bool
	}{
		{"empty", args{
			fsys: txtarFSFromString(`
-- base.tmpl --
{{ define "base" }}
base
{{ template "empty" }}
{{ template "empty_part" }}
{{ end }}
-- default-light/default.tmpl --
{{ define "empty" }}
{{ end }}
-- default-light/partials/empty.tmpl --
{{ define "empty_part" }}
empty
{{ end }}
-- admin-light/default.tmpl --
null
`),
			theme: "default-light",
		}, false, true},
		{"admin", args{
			fsys: txtarFSFromString(`
-- base.tmpl --
{{ define "base" }}
admin-base
{{ template "admin" }}
{{ template "admin_partial" }}
{{ end }}
-- default-light/default.tmpl --
{{ define "empty" }}{{ end }}
-- admin-light/default.tmpl --
{{ define "admin" }}{{ end }}
-- admin-light/partials/admin.tmpl --
{{ define "admin_partial" }}{{ end }}
-- admin-dark/default.tmpl --
{{ define "admin" }}{{ end }}
`),
			theme: "admin-dark",
		}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := templates.FromFS(tt.args.fsys, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromFS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.shouldExec {
				exec := got.Executor()
				err = exec.ExecuteTemplate(context.Background(), tt.args.theme, "default", "base", io.Discard, nil)
				if err != nil {
					t.Errorf("template did not execute: %v", err)
					return
				}
			}
		})
	}
}
