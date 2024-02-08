package templates_test

import (
	"io/fs"
	"log"
	"reflect"
	"testing"

	"github.com/R-a-dio/valkyrie/mocks"
	"github.com/R-a-dio/valkyrie/templates"
)

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
		{"always error", args{&mocks.ErrorFS{}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := templates.LoadThemes(tt.args.fsys)
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
		f.Add(tc) // Use f.Add to provide a seed corpus
	}
	f.Fuzz(func(t *testing.T, name string) {
		rfs := &mocks.RandomFS{Name: name}
		log.Println(name)
		themes, err := templates.LoadThemes(rfs)
		if err == nil || themes != nil {
			t.Errorf("there should probably be an error for %s", name)
		}
	})
}
