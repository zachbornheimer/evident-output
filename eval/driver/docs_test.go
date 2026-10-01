package driver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zachbornheimer/evident-output/eval/driver"
)

func corpusServer() *fakeServer {
	return &fakeServer{replies: map[string]driver.ToolResult{
		"evident_output_list_sections": {Structured: json.RawMessage(`{"sections":[{"id":"a"},{"id":"b"}]}`)},
		"evident_output_get_documentation": {Structured: json.RawMessage(
			`{"sections":[{"id":"a","title":"A","body":"alpha"},{"id":"b","title":"B","body":"beta"}]}`)},
	}}
}

func TestLoadCorpus_FetchesOnceThenServesFromCache(t *testing.T) {
	cache := &memoryStore{}
	first := corpusServer()
	corpus, err := driver.LoadCorpus(context.Background(), first, cache)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if !strings.Contains(corpus, "alpha") || !strings.Contains(corpus, "beta") {
		t.Errorf("corpus lacks section bodies: %q", corpus)
	}
	second := &fakeServer{}
	again, err := driver.LoadCorpus(context.Background(), second, cache)
	if err != nil || again != corpus {
		t.Fatalf("cached load = %q, %v", again, err)
	}
	if len(second.calls) != 0 {
		t.Errorf("cache hit still called the server: %v", second.calls)
	}
}

func TestDirBlobStore_RoundTripsAndMissesCleanly(t *testing.T) {
	store := driver.DirBlobStore{Dir: filepath.Join(t.TempDir(), "nested", "sha")}
	if _, ok, err := store.Get("k"); ok || err != nil {
		t.Fatalf("miss = %v, %v", ok, err)
	}
	if err := store.Put("k", []byte("v")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, ok, err := store.Get("k"); !ok || err != nil || string(got) != "v" {
		t.Fatalf("hit = %q, %v, %v", got, ok, err)
	}
}

// fakeMCPPeer answers initialize, tools/list and tools/call over pipes.
func fakeMCPPeer(t *testing.T) (io.Reader, io.Writer) {
	t.Helper()
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	go func() {
		defer serverToClientW.Close()
		scanner := bufio.NewScanner(clientToServerR)
		for scanner.Scan() {
			var req struct {
				ID     *int   `json:"id"`
				Method string `json:"method"`
			}
			_ = json.Unmarshal(scanner.Bytes(), &req)
			if req.ID == nil {
				continue
			}
			result := `{}`
			switch req.Method {
			case "tools/list":
				result = `{"tools":[{"name":"x","description":"d","inputSchema":{"type":"object"}}]}`
			case "tools/call":
				result = `{"content":[{"type":"text","text":"hello"}],"structuredContent":{"k":1}}`
			}
			_, _ = io.WriteString(serverToClientW, `{"jsonrpc":"2.0","id":`+itoa(*req.ID)+`,"result":`+result+"}\n")
		}
	}()
	return serverToClientR, clientToServerW
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestStdioMCP_HandshakeListAndCall(t *testing.T) {
	in, out := fakeMCPPeer(t)
	server, err := driver.NewStdioMCP(context.Background(), in, out, func() error { return nil })
	if err != nil {
		t.Fatalf("NewStdioMCP: %v", err)
	}
	tools, err := server.Tools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "x" {
		t.Fatalf("Tools = %+v, %v", tools, err)
	}
	result, err := server.Call(context.Background(), "x", nil)
	if err != nil || result.Text != "hello" || string(result.Structured) != `{"k":1}` {
		t.Fatalf("Call = %+v, %v", result, err)
	}
}

// Against the real server binary: the corpus really comes through the stdio
// protocol, and the tool list really contains every forwarded tool.
func TestSpawnMCP_RealServerServesCorpusAndTools(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the MCP server binary")
	}
	binary := filepath.Join(t.TempDir(), "evident-output-mcp")
	build := exec.Command("go", "build", "-o", binary, "./cmd/evident-output-mcp")
	build.Dir = filepath.Join("..", "..") // the library module, which owns the server's dependencies
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build MCP server: %v\n%s", err, out)
	}
	t.Setenv("EVO_MCP_NO_AUTO_UPDATE", "1")
	server, err := driver.SpawnMCP(context.Background(), binary)
	if err != nil {
		t.Fatalf("SpawnMCP: %v", err)
	}
	defer server.Close()
	box := driver.Toolbox{Server: server}
	defs, err := box.Definitions(context.Background())
	if err != nil || len(defs) < 7 {
		t.Fatalf("Definitions = %d tools, %v", len(defs), err)
	}
	corpus, err := driver.LoadCorpus(context.Background(), server, &memoryStore{})
	if err != nil || len(corpus) < 1000 {
		t.Fatalf("corpus = %d bytes, %v", len(corpus), err)
	}
}

func TestFixtureAPISummary_ListsExportedSignaturesWithDocsAndNoBodies(t *testing.T) {
	fixture := fstest.MapFS{"fixture.go": &fstest.MapFile{Data: []byte(`// Package fixture is a stub.
package fixture

import "context"

// Landed lists branches.
func Landed(context.Context) ([]string, error) { return []string{"secret-body"}, nil }

func hidden() {}

// Store holds things.
type Store struct{}

// Get reads one.
func (*Store) Get() string { return "x" }
`)}}
	summary, err := driver.FixtureAPISummary(fixture)
	if err != nil {
		t.Fatalf("FixtureAPISummary: %v", err)
	}
	for _, want := range []string{"Landed lists branches.", "func Landed(context.Context) ([]string, error)", "type Store struct{}", "func (*Store) Get() string"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary lacks %q:\n%s", want, summary)
		}
	}
	for _, banned := range []string{"secret-body", "hidden"} {
		if strings.Contains(summary, banned) {
			t.Errorf("summary leaks %q:\n%s", banned, summary)
		}
	}
}
