package manager

import (
	"fmt"
	"testing"
)

var metadataTests = []string{
	"桜島麻衣、古賀朋絵、双葉理央、豊浜のどか、梓川かえで、牧之原翔子(CV:瀬戸麻沙美、東山奈央、種﨑敦美、内田真礼、久保ユリカ、水瀬いのり) - Fukashigi no Karte -Instrumental-",
	"",
}

func TestParseMetadata(t *testing.T) {
	for i, meta := range metadataTests {
		full := fmt.Sprintf("StreamTitle='%s';", meta)

		m := parseMetadata([]byte(full))

		t.Log(len(meta))
		if m["StreamTitle"] != meta {
			t.Error(i, m)
		}
	}
}
