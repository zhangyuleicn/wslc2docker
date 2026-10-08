package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wslc2docker/wslc2docker/internal/backend"
	"github.com/wslc2docker/wslc2docker/internal/config"
	"github.com/wslc2docker/wslc2docker/internal/types"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg, err := config.Load("../../endpoints.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	srv := New(cfg, "../../endpoints.yaml", backend.NewFake())
	return srv
}

func do(t *testing.T, srv *Server, method, path, body string) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = readerString(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec, rec.Body.Bytes()
}

type strReader struct{ s string }

func readerString(s string) io.Reader { return &strReader{s: s} }
func (r *strReader) Read(p []byte) (int, error) {
	if r.s == "" {
		return 0, io.EOF
	}
	n := copy(p, r.s)
	r.s = r.s[n:]
	return n, nil
}

func TestPingAndVersion(t *testing.T) {
	srv := newTestServer(t)
	rec, _ := do(t, srv, "GET", "/_ping", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/_ping: want 200 got %d", rec.Code)
	}
	rec, b := do(t, srv, "GET", "/version", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("/version: want 200 got %d", rec.Code)
	}
	var v types.Version
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("version json: %v", err)
	}
	if v.ApiVersion == "" {
		t.Fatal("ApiVersion empty")
	}
}

func TestContainerLifecycle(t *testing.T) {
	srv := newTestServer(t)
	// list
	rec, _ := do(t, srv, "GET", "/containers/json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("containers/json: want 200 got %d", rec.Code)
	}
	// create
	rec, b := do(t, srv, "POST", "/containers/create?name=web", `{"Image":"nginx","Cmd":["nginx","-g","daemon off;"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: want 201 got %d", rec.Code)
	}
	var idr types.IDResponse
	if err := json.Unmarshal(b, &idr); err != nil || idr.ID == "" {
		t.Fatalf("create id: %v (%s)", err, b)
	}
	// inspect
	rec, _ = do(t, srv, "GET", "/containers/web/json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect: want 200 got %d", rec.Code)
	}
	// start
	rec, _ = do(t, srv, "POST", "/containers/web/start", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("start: want 204 got %d", rec.Code)
	}
	// logs
	rec, b = do(t, srv, "GET", "/containers/web/logs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("logs: want 200 got %d", rec.Code)
	}
	if len(b) == 0 {
		t.Fatal("logs empty")
	}
}

func TestImagePullAndList(t *testing.T) {
	srv := newTestServer(t)
	rec, _ := do(t, srv, "GET", "/images/json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("images/json: want 200 got %d", rec.Code)
	}
	rec, _ = do(t, srv, "POST", "/images/create?fromImage=nginx&tag=latest", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("images/create: want 200 got %d", rec.Code)
	}
	rec, _ = do(t, srv, "GET", "/images/nginx:latest/json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("image inspect: want 200 got %d", rec.Code)
	}
}

func TestDisabledAndUnknown(t *testing.T) {
	srv := newTestServer(t)
	// 禁用端点（/events, id 5, enabled:false）→ 501
	rec, _ := do(t, srv, "GET", "/events", "")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("disabled /events: want 501 got %d", rec.Code)
	}
	// 未知路径 → 404
	rec, _ = do(t, srv, "GET", "/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: want 404 got %d", rec.Code)
	}
}

func TestExecFlow(t *testing.T) {
	srv := newTestServer(t)
	rec, b := do(t, srv, "POST", "/containers/web/exec", `{"Cmd":["ls"],"AttachStdout":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("exec create: want 201 got %d", rec.Code)
	}
	var idr types.IDResponse
	if err := json.Unmarshal(b, &idr); err != nil || idr.ID == "" {
		t.Fatalf("exec id: %v", err)
	}
	rec, _ = do(t, srv, "POST", "/exec/"+idr.ID+"/start", `{"Tty":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("exec start: want 200 got %d", rec.Code)
	}
	rec, _ = do(t, srv, "GET", "/exec/"+idr.ID+"/json", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("exec inspect: want 200 got %d", rec.Code)
	}
}

// 确保 fake 后端在 context 取消时不报错（仅编译期保证）。
var _ = context.Background
