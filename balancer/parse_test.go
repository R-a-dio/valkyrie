package balancer

import "testing"

func TestParseXML(t *testing.T) {
	x := []byte(
		`<?xml version="1.0" encoding="UTF-8"?>
<playlist xmlns="http://xspf.org/ns/0/" version="1">
  <title/>
  <creator/>
  <trackList>
    <track>
      <location>http://stream.r-a-d.io:8000/main.mp3</location>
      <title>test - test</title>
      <annotation>Stream Title: R/a/dio
Stream Description: Your weeaboo station
Content Type:audio/mpeg
Current Listeners: 1337
Peak Listeners: 144
Stream Genre: various</annotation>
    </track>
  </trackList>
</playlist>`)
	listeners, err := parsexml(x)
	if err != nil {
		t.Errorf("parsexml could not parse valid xml: %w", err)
		return
	}
	if listeners != 1337 {
		t.Errorf("parsexml was incorrect, got: %d, want: %d", listeners, 1337)
		return
	}
}
