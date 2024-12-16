package audio

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
	
	// stripping tags //
	arg.WriteString("-i ")
	arg.WriteString(fmt.Sprintf(`%q`, file))
	arg.WriteString(" -map_metadata -1 -c:v copy -c:a copy ")
	arg.WriteString("\"temp")
	arg.WriteString(file)
	arg.WriteString("\"")
	placeholder1 := exec.Command("ffmpeg", arg.String())
	arg.Reset()

	fmt.Println(fmt.Sprintf(`%q`, tags[1]))

	// writing tags to new perm file from temp // 
	arg.WriteString("-i ")
	arg.WriteString("\"temp")
	arg.WriteString(file)
	arg.WriteString("\" ")
	arg.WriteString(" -c:a copy")
	for n := 0; n < len(tags); n++ {
		arg.WriteString(" -metadata ")
		arg.WriteString(metadataTags[n])
		arg.WriteString(fmt.Sprintf(`%q`, tags[n]))
	}
	arg.WriteString(" -o ")
	arg.WriteString(fmt.Sprintf(`%q`, file))
	placeholder3 := exec.Command("ffmpeg", arg.String())

	arg.Reset()

	// remove temp file //
	arg.WriteString("rm ")
	arg.WriteString("\"temp")
	arg.WriteString(file)
	arg.WriteString("\" ")
	placeholder2 := exec.Command(arg.String())

}
