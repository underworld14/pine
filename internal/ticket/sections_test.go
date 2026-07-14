package ticket

import (
	"reflect"
	"testing"
)

func TestRelatedFiles(t *testing.T) {
	body := `# Description

Something.

# Related Files

- src/login.tsx
- ` + "`internal/cli/import.go`" + `
- @internal/server/git.go
* apps/web/src/lib/x.ts

not a bullet
-   

# Other

- ignored.md
`
	got := RelatedFiles(body)
	want := []string{"src/login.tsx", "internal/cli/import.go", "internal/server/git.go", "apps/web/src/lib/x.ts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RelatedFiles = %#v, want %#v", got, want)
	}
	if RelatedFiles("# No section\n") != nil {
		t.Fatal("missing section should return nil")
	}
}

func TestAttachmentRefs(t *testing.T) {
	body := `# Attachments

- docs/shot.png
- ` + "`notes/a.md`" + `
![diagram](assets/diag.svg)
See also [spec](specs/api.md) and ![](assets/diag.svg)
-   

plain text line
`
	got := AttachmentRefs(body)
	want := []string{"docs/shot.png", "notes/a.md", "assets/diag.svg", "specs/api.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AttachmentRefs = %#v, want %#v", got, want)
	}
	if AttachmentRefs("# Related Files\n- x.go\n") != nil {
		t.Fatal("wrong section should return nil")
	}
}
