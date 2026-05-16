package proxy

import "testing"

func TestParseModeAcceptsSupportedNetworkModes(t *testing.T) {
	for _, input := range []string{"online", "offline", "server-error"} {
		mode, err := ParseMode(input)
		if err != nil {
			t.Fatalf("ParseMode(%q) returned error: %v", input, err)
		}
		if mode.String() != input {
			t.Fatalf("ParseMode(%q) string = %q", input, mode.String())
		}
	}
}

func TestParseModeRejectsUnknownNetworkMode(t *testing.T) {
	if _, err := ParseMode("flaky"); err == nil {
		t.Fatal("ParseMode accepted unsupported mode")
	}
}
