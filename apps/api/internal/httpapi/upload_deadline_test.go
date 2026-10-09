package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowPost sends a body in two parts, 300 ms apart, to a server whose read limit is 150 ms.
func slowPost(t *testing.T, handler http.HandlerFunc) (int, string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.ReadTimeout = 150 * time.Millisecond
	srv.Start()
	defer srv.Close()

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("first-part|"))
		time.Sleep(300 * time.Millisecond)
		_, _ = pw.Write([]byte("second-part"))
		_ = pw.Close()
	}()
	req, err := http.NewRequest(http.MethodPost, srv.URL, pr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestAllowSlowRequestLetsASlowUploadFinish(t *testing.T) {
	read := func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestTimeout)
			return
		}
		_, _ = w.Write(data)
	}

	// Without the extension the server's own limit cuts the slow body.
	if code, body := slowPost(t, read); code == http.StatusOK && body == "first-part|second-part" {
		t.Fatal("the control case did not hit the read limit; the test proves nothing")
	}

	extended := func(w http.ResponseWriter, r *http.Request) {
		allowSlowRequest(w, 5*time.Second, 5*time.Second)
		read(w, r)
	}
	code, body := slowPost(t, extended)
	if code != http.StatusOK || body != "first-part|second-part" {
		t.Fatalf("slow upload = %d %q", code, body)
	}
}
