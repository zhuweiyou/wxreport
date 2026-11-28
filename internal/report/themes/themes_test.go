package themes

import (
	"slices"
	"testing"
)

func TestDefaultAndRegistry(t *testing.T) {
	if got := Default; got != "moments" {
		t.Fatalf("Default = %q, want moments", got)
	}

	names := Names()
	for _, want := range []string{"minimal", "moments"} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %v, missing %q", names, want)
		}
	}
}

func TestGet(t *testing.T) {
	got, err := Get("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != Default {
		t.Fatalf("Get(\"\").Name = %q, want %q", got.Name, Default)
	}

	if _, err := Get("not-exist"); err == nil {
		t.Fatal("Get(not-exist) expected error")
	}
}
