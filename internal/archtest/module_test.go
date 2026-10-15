package archtest

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
)

// loadGraph reads the real module's closure, standard library and third-party
// modules included, since "does pkg/ reach cobra" needs the external edges.
// Test-only imports are not in it: the rules guard what ships.
func loadGraph(t *testing.T) Graph {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", "-json", module+"/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	g := Graph{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode go list: %v", err)
		}
		g[p.ImportPath] = p.Imports
	}
	return g
}

func TestModuleDependencyDirection(t *testing.T) {
	g := loadGraph(t)
	// Guard against a vacuous pass: the forbidden things and the layers they
	// are forbidden from must really be in the graph.
	for _, want := range []string{
		"github.com/spf13/cobra",
		"github.com/modelcontextprotocol/go-sdk/mcp",
		module + "/cmd/ava",
		module + "/internal/monitor",
	} {
		if _, ok := g[want]; !ok {
			t.Fatalf("graph lacks %s; rules would pass over nothing", want)
		}
	}
	for _, v := range Check(g, Rules) {
		t.Error(v)
	}
}
