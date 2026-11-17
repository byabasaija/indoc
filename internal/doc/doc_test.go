package doc

import "testing"

func TestNormalizeAndValidate(t *testing.T) {
	d := &Document{
		ProjectName: "  Hospital API  ",
		Overview:    Page{Markdown: "Admits patients and schedules visits."},
		Guides: []Page{
			{Title: "Getting Started", Markdown: "Run the server."},
			{Title: "Getting Started", Markdown: "Auth."},
		},
		Reference: []RefSection{{
			Kind: KindRoutes,
			Entries: []RefEntry{{
				Name:   "/health",
				Detail: "GET",
				Source: "main.py:4",
			}},
		}},
	}

	if err := Normalize(d); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if d.ProjectName != "Hospital API" {
		t.Fatalf("project name %q", d.ProjectName)
	}
	if d.Overview.ID != "overview" || d.Overview.Title != "Overview" {
		t.Fatalf("overview = %+v", d.Overview)
	}
	if d.Guides[0].ID != "getting-started" || d.Guides[1].ID != "getting-started-2" {
		t.Fatalf("guide ids = %q %q", d.Guides[0].ID, d.Guides[1].ID)
	}
	if d.Reference[0].ID != "routes" || d.Reference[0].Title != "Routes" {
		t.Fatalf("reference = %+v", d.Reference[0])
	}
}

func TestValidateRejectsEmptyOverview(t *testing.T) {
	d := &Document{ProjectName: "Demo", Overview: Page{ID: "overview", Title: "Overview"}}
	if err := Validate(d); err == nil {
		t.Fatal("expected overview markdown to be required")
	}
}

func TestValidateAppearanceColors(t *testing.T) {
	d := &Document{
		ProjectName: "Demo",
		Appearance: &Appearance{Colors: &Colors{
			Primary: "#0969da",
			Accent:  "#1f883d",
		}},
		Overview: Page{ID: "overview", Title: "Overview", Markdown: "Hi"},
	}
	if err := Validate(d); err != nil {
		t.Fatalf("valid palette rejected: %v", err)
	}
	d.Appearance.Colors.Primary = "red"
	if err := Validate(d); err == nil {
		t.Fatal("expected invalid palette color to fail")
	}
}

func TestValidateRejectsUnknownReferenceKind(t *testing.T) {
	d := &Document{
		ProjectName: "Demo",
		Overview:    Page{ID: "overview", Title: "Overview", Markdown: "Hi"},
		Reference: []RefSection{{
			ID:      "notes",
			Title:   "Notes",
			Kind:    "notes",
			Entries: []RefEntry{{Name: "x"}},
		}},
	}
	if err := Validate(d); err == nil {
		t.Fatal("expected unknown reference kind to fail")
	}
}

func TestSourcePath(t *testing.T) {
	if got := SourcePath("app/main.py:12"); got != "app/main.py" {
		t.Fatalf("got %q", got)
	}
	if got := SourcePath("C:/repo/main.go:3"); got != "C:/repo/main.go" {
		t.Fatalf("got %q", got)
	}
}
