package helperclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// A log tail near the 64 KB helper limit, full of characters that JSON escapes,
// must still decode. This failed before the response limit was raised.
func TestLargeLogReplyDecodes(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "h.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	big := strings.Repeat(`GET /path?q="x" 200 \t`, 3000) // ~66 KB of quotes and backslashes
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(response{OK: true, Output: big})
	})}
	go srv.Serve(ln)
	defer srv.Close()

	out, err := New(sock).Output(context.Background(), "logs.tail", map[string]string{})
	if err != nil {
		t.Fatalf("large reply rejected: %v", err)
	}
	if out != big {
		t.Fatal("reply was altered in transit")
	}
}
