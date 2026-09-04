package media

import (
	"runtime"
	"testing"
)

func TestDocumentOpenCmdUsesPlatformOpener(t *testing.T) {
	cmd, err := documentOpenCmd("/tmp/demo.gif")
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Skip(err.Error())
		}
		t.Fatal(err)
	}
	if len(cmd.Args) == 0 {
		t.Fatal("empty args")
	}
	want := map[string]string{
		"windows": "rundll32",
		"darwin":  "open",
	}[runtime.GOOS]
	if want == "" {
		want = "xdg-open"
	}
	if cmd.Args[0] != want {
		t.Fatalf("Args[0]=%q want %q (full=%#v)", cmd.Args[0], want, cmd.Args)
	}
}
