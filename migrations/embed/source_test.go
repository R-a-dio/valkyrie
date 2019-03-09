package migrations

import (
	"testing"

	st "github.com/golang-migrate/migrate/v4/source/testing"
)

func TestSourceOpen(t *testing.T) {
	_, err := becky{}.Open("")
	if err != nil {
		t.Fatal(err)
	}
}

func TestSourceTesting(t *testing.T) {
	m := map[string]string{
		"0001_first.up.sql":    "",
		"0001_first.down.sql":  "",
		"0003_second.up.sql":   "",
		"0004_third.up.sql":    "",
		"0004_third.down.sql":  "",
		"0005_fourth.down.sql": "",
		"0007_fifth.up.sql":    "",
		"0007_fifth.down.sql":  "",
	}

	d, err := becky{}.init(m)
	if err != nil {
		t.Fatal(err)
	}

	st.Test(t, d)
}
