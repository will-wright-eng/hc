package git

import "testing"

func TestResolveChains_Simple(t *testing.T) {
	raw := map[string]string{
		"a.go": "b.go",
	}
	rm := resolveChains(raw)
	if got := rm.Resolve("a.go"); got != "b.go" {
		t.Errorf("Resolve(a.go) = %q, want b.go", got)
	}
}

func TestResolveChains_Chain(t *testing.T) {
	raw := map[string]string{
		"a.go": "b.go",
		"b.go": "c.go",
	}
	rm := resolveChains(raw)
	if got := rm.Resolve("a.go"); got != "c.go" {
		t.Errorf("Resolve(a.go) = %q, want c.go", got)
	}
	if got := rm.Resolve("b.go"); got != "c.go" {
		t.Errorf("Resolve(b.go) = %q, want c.go", got)
	}
}

func TestResolveChains_LongChain(t *testing.T) {
	raw := map[string]string{
		"a.go": "b.go",
		"b.go": "c.go",
		"c.go": "d.go",
	}
	rm := resolveChains(raw)
	if got := rm.Resolve("a.go"); got != "d.go" {
		t.Errorf("Resolve(a.go) = %q, want d.go", got)
	}
}

func TestResolveChains_Cycle(t *testing.T) {
	raw := map[string]string{
		"a.go": "b.go",
		"b.go": "a.go",
	}
	// Should not infinite loop; result is deterministic per entry.
	rm := resolveChains(raw)
	got := rm.Resolve("a.go")
	if got != "a.go" && got != "b.go" {
		t.Errorf("Resolve(a.go) = %q, expected a.go or b.go", got)
	}
}

func TestResolveChains_Disjoint(t *testing.T) {
	raw := map[string]string{
		"a.go": "b.go",
		"x.go": "y.go",
	}
	rm := resolveChains(raw)
	if got := rm.Resolve("a.go"); got != "b.go" {
		t.Errorf("Resolve(a.go) = %q, want b.go", got)
	}
	if got := rm.Resolve("x.go"); got != "y.go" {
		t.Errorf("Resolve(x.go) = %q, want y.go", got)
	}
}

func TestResolve_NotRenamed(t *testing.T) {
	rm := RenameMap{"a.go": "b.go"}
	if got := rm.Resolve("z.go"); got != "z.go" {
		t.Errorf("Resolve(z.go) = %q, want z.go", got)
	}
}

func TestResolve_NilMap(t *testing.T) {
	var rm RenameMap
	if got := rm.Resolve("a.go"); got != "a.go" {
		t.Errorf("nil Resolve(a.go) = %q, want a.go", got)
	}
}
