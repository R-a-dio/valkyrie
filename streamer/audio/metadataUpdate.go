
cmd commands are 
"ffmpeg -i "orignalfile.extension" -map_metadata -1 -c:v copy -c:a copy "tempfile.extension"" - Wipes Metadata, writes to a tempfile
"mv/move "tempfile.extension" "orignalfile.extension
"ffmpeg -i "tempfile.extension" -metadata artist="Oasis" -metadata title="Falling Down" -metadata album="Dig Out Your Soul" -codec copy "orignalfile.extension""#
(you need to delete the temp file here)


// You can edit this code!
// Click here and start typing.
package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// assuming filename is passed into function with tags
func MetadataAssign(tags []string, file string) {
	// building string sequentially to avoid unwanted whitespace //
	metadataTags := []string{"artist=", "title=", "album="}
	var arg strings.Builder

	fmt.Println(fmt.Sprintf(`%q`, tags[1]))

	arg.WriteString("-i ")
	arg.WriteString(fmt.Sprintf(`%q`, file))
	arg.WriteString(" -map_metadata -1 -c:v copy -c:a copy ")
	arg.WriteString("\"temp")
	arg.WriteString(file)
	arg.WriteString("\"")
	placeholder1 := exec.Command("ffmpeg", arg.String())
	fmt.Println(placeholder1)

	arg.Reset()

	arg.WriteString("mv ")
	arg.WriteString("\"temp")
	arg.WriteString(file)
	arg.WriteString("\" ")
	arg.WriteString(fmt.Sprintf(`%q`, file))
	placeholder2 := exec.Command(arg.String())
	fmt.Println(placeholder2)

	arg.Reset()

	fmt.Println(arg)

	arg.WriteString("-i ")
	arg.WriteString(fmt.Sprintf(`%q`, file))
	arg.WriteString(" -c:a copy")
	for n := 0; n < len(tags); n++ {
		arg.WriteString(" -metadata ")
		arg.WriteString(metadataTags[n])
		arg.WriteString(fmt.Sprintf(`%q`, tags[n]))
	}
	arg.WriteString(" -o ")
	arg.WriteString(fmt.Sprintf(`%q`, file))
	placeholder3 := exec.Command("ffmpeg", arg.String())
	fmt.Println(placeholder3)
}
