package learning

import "testing"

func TestMissingCitedPaths(t *testing.T) {
	exists := func(rel string) bool {
		return rel == "internal/db/query.go" || rel == "ok.go"
	}

	t.Run("all valid", func(t *testing.T) {
		missing := MissingCitedPaths([]string{"ok.go", "internal/db/query.go"}, exists)
		if len(missing) != 0 {
			t.Fatalf("want none missing, got %v", missing)
		}
		if IsCitationStale(missing) {
			t.Fatal("should not be stale")
		}
	})

	t.Run("one missing", func(t *testing.T) {
		missing := MissingCitedPaths([]string{"ok.go", "gone.go"}, exists)
		if len(missing) != 1 || missing[0] != "gone.go" {
			t.Fatalf("got %v", missing)
		}
		if !IsCitationStale(missing) {
			t.Fatal("should be stale")
		}
	})

	t.Run("all missing", func(t *testing.T) {
		missing := MissingCitedPaths([]string{"a.go", "b.go"}, exists)
		if len(missing) != 2 {
			t.Fatalf("got %v", missing)
		}
	})

	t.Run("empty cites", func(t *testing.T) {
		if MissingCitedPaths(nil, exists) != nil {
			t.Fatal("nil cites should be no-op")
		}
		if MissingCitedPaths([]string{}, exists) != nil {
			t.Fatal("empty cites should be no-op")
		}
	})
}
