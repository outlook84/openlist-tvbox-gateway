package utils

import "testing"

func TestCleanRelativeRejectsTraversalAndURLs(t *testing.T) {
	bad := []string{"../x", "a/../../x", "https://example.com/a", "//example.com/a", "%252e%252e/x"}
	for _, value := range bad {
		if _, err := CleanRelative(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestJoinKeepsPathUnderRoot(t *testing.T) {
	got, err := Join("/Movies", "Action/a.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/Movies/Action/a.mkv" {
		t.Fatalf("got %q", got)
	}
}
