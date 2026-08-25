package composer

import "testing"

func TestAddServiceIsIdempotentByName(t *testing.T) {
	c := NewComposeConfig()
	first := c.AddService("web")
	first.Image = "nginx"

	second := c.AddService("web")
	if second != first {
		t.Fatalf("AddService with an existing name returned a different *ServiceConfig instead of the existing one")
	}
	if len(c.Services) != 1 {
		t.Fatalf("AddService with an existing name created a duplicate entry, got %d services", len(c.Services))
	}
	if second.Image != "nginx" {
		t.Fatalf("AddService with an existing name returned a fresh config instead of the existing one (Image = %q)", second.Image)
	}
}

func TestGetServiceMissing(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("web")
	if c.GetService("db") != nil {
		t.Fatalf("GetService for a name that was never added should return nil")
	}
}

func TestRemoveService(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("web")
	c.AddService("db")

	if !c.RemoveService("web") {
		t.Fatalf("RemoveService on an existing service should report true")
	}
	if c.GetService("web") != nil {
		t.Fatalf("service still present after RemoveService")
	}
	if c.RemoveService("web") {
		t.Fatalf("RemoveService on an already-removed service should report false")
	}
	if len(c.Services) != 1 {
		t.Fatalf("expected 1 remaining service, got %d", len(c.Services))
	}
}

func TestRenameServiceFixesDependsOnReferences(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("db")
	web := c.AddService("web")
	web.DependsOn = []DependsOnEntry{{Service: "db", Condition: CondServiceStarted}}

	if !c.RenameService("db", "database") {
		t.Fatalf("RenameService should succeed for an existing service with no name collision")
	}
	if c.GetService("db") != nil {
		t.Fatalf("old name should no longer resolve after rename")
	}
	if c.GetService("database") == nil {
		t.Fatalf("new name should resolve after rename")
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0].Service != "database" {
		t.Fatalf("depends_on reference was not updated to the new name, got %+v", web.DependsOn)
	}
}

func TestRenameServiceRejectsCollision(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("web")
	c.AddService("db")

	if c.RenameService("web", "db") {
		t.Fatalf("RenameService should refuse to rename onto an existing service name")
	}
	if c.GetService("web") == nil {
		t.Fatalf("original service should be untouched after a rejected rename")
	}
}

func TestMoveService(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("a")
	c.AddService("b")
	c.AddService("c")

	if !c.MoveService(0, 2) {
		t.Fatalf("MoveService with valid indices should report true")
	}
	// Moving index 0 ("a") to index 2 removes it first ([b, c]), then
	// inserts before the (now shifted-left-by-one) target index 1:
	// [b, a, c] — matching MoveService's j-- adjustment for j > i.
	got := c.ServiceNames()
	want := []string{"b", "a", "c"}
	if len(got) != len(want) {
		t.Fatalf("ServiceNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ServiceNames() = %v, want %v", got, want)
		}
	}
}

func TestMoveServiceRejectsOutOfRange(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("a")

	if c.MoveService(0, 5) {
		t.Fatalf("MoveService with an out-of-range index should report false")
	}
	if c.MoveService(-1, 0) {
		t.Fatalf("MoveService with a negative index should report false")
	}
}
