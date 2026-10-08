package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/wslc2docker/wslc2docker/internal/backend"
	"github.com/wslc2docker/wslc2docker/internal/types"
)

// handlerFunc 是单个端点的处理器签名。
type handlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string, s *Server) error

// handlers 把端点 id → 处理器。未在表中的端点由 Server 返回 404；
// 在表中但无处理器的端点返回 501（见 Server.ServeHTTP）。
var handlers = map[int]handlerFunc{
	1:  pingHandler, // GET  /_ping
	2:  pingHandler, // HEAD /_ping
	3:  versionHandler,
	4:  infoHandler,
	6:  systemDFHandler,
	9:  containerListHandler,
	10: containerCreateHandler,
	11: containerInspectHandler,
	13: containerLogsHandler,
	17: execResizeHandler, // 容器 tty 尺寸调整（尽力而为：接受并返回 204）
	18: containerStartHandler,
	19: containerStopHandler,
	20: containerRestartHandler,
	21: containerKillHandler,
	27: containerRemoveHandler,
	34: containerPruneHandler,
	30: execCreateHandler,
	35: execStartHandler,
	36: execResizeHandler,
	37: execInspectHandler,
	38: imageListHandler,
	39: imageCreateHandler, // pull
	40: imageInspectHandler,
	43: imageRemoveHandler,
	56: networkInspectHandler,
	63: volumeInspectHandler,
}

// ---- 通用辅助 ----

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.Message{Message: msg})
}

func streamCopy(w http.ResponseWriter, ct string, rc io.Reader) error {
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	_, err := io.Copy(w, rc)
	if c, ok := rc.(io.Closer); ok {
		_ = c.Close()
	}
	return err
}

// ---- 系统类 ----

func pingHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, _ *Server) error {
	w.Header().Set("Docker-Experimental", "false")
	w.Header().Set("API-Version", "1.56")
	w.Header().Set("OSType", "linux")
	w.Header().Set("Content-Type", "text/plain")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
	return nil
}

func versionHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	v, err := s.Backend().Version(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func infoHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	info, err := s.Backend().Info(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, info)
	return nil
}

func systemDFHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	du, err := s.Backend().SystemDF(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, du)
	return nil
}

// ---- 容器类 ----

func containerListHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	all := r.URL.Query().Get("all") == "true" || r.URL.Query().Get("all") == "1"
	list, err := s.Backend().ContainerList(ctx, all)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

// dockerCreateBody 是 POST /containers/create 请求体的局部解析结构。
type dockerCreateBody struct {
	Image        string              `json:"Image"`
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	Env          []string            `json:"Env"`
	WorkingDir   string              `json:"WorkingDir"`
	Labels       map[string]string   `json:"Labels"`
	Tty          bool                `json:"Tty"`
	OpenStdin    bool                `json:"OpenStdin"`
	AttachStdin  bool                `json:"AttachStdin"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	HostConfig   struct {
		Binds         []string                       `json:"Binds"`
		PortBindings  map[string][]types.PortBinding `json:"PortBindings"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
}

func containerCreateHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	var body dockerCreateBody
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			return fmt.Errorf("decode create body: %w", err)
		}
	}
	spec := &backend.ContainerCreateSpec{
		Name:          r.URL.Query().Get("name"),
		Image:         body.Image,
		Cmd:           body.Cmd,
		Entrypoint:    body.Entrypoint,
		Env:           body.Env,
		WorkingDir:    body.WorkingDir,
		Labels:        body.Labels,
		Tty:           body.Tty,
		OpenStdin:     body.OpenStdin,
		AttachStdin:   body.AttachStdin,
		ExposedPorts:  body.ExposedPorts,
		PortBindings:  body.HostConfig.PortBindings,
		Binds:         body.HostConfig.Binds,
		RestartPolicy: body.HostConfig.RestartPolicy.Name,
	}
	id, err := s.Backend().ContainerCreate(ctx, spec)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, types.IDResponse{ID: id})
	return nil
}

func containerInspectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	ci, err := s.Backend().ContainerInspect(ctx, p["id"])
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, ci)
	return nil
}

func containerLogsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	q := r.URL.Query()
	opts := backend.LogsOptions{
		ShowStdout: q.Get("stdout") != "0" && q.Get("stdout") != "false",
		ShowStderr: q.Get("stderr") != "0" && q.Get("stderr") != "false",
		Follow:     q.Get("follow") == "1" || q.Get("follow") == "true",
		Timestamps: q.Get("timestamps") == "1" || q.Get("timestamps") == "true",
		Since:      q.Get("since"),
		Until:      q.Get("until"),
		Tail:       q.Get("tail"),
	}
	if !opts.ShowStdout && !opts.ShowStderr {
		opts.ShowStdout = true
	}
	rc, err := s.Backend().ContainerLogs(ctx, p["id"], opts)
	if err != nil {
		return err
	}
	return streamCopy(w, "application/vnd.docker.raw-stream", rc)
}

func lifecycleNoContent(ctx context.Context, w http.ResponseWriter, _ map[string]string, s *Server, fn func(backend.Backend) error) error {
	if err := fn(s.Backend()); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func containerStartHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	return lifecycleNoContent(ctx, w, p, s, func(b backend.Backend) error { return b.ContainerStart(ctx, p["id"]) })
}

func containerStopHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	t := 5
	if v := r.URL.Query().Get("t"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			t = n
		}
	}
	return lifecycleNoContent(ctx, w, p, s, func(b backend.Backend) error { return b.ContainerStop(ctx, p["id"], t) })
}

func containerRestartHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	t := 5
	if v := r.URL.Query().Get("t"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			t = n
		}
	}
	return lifecycleNoContent(ctx, w, p, s, func(b backend.Backend) error { return b.ContainerRestart(ctx, p["id"], t) })
}

func containerKillHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	return lifecycleNoContent(ctx, w, p, s, func(b backend.Backend) error { return b.ContainerKill(ctx, p["id"], r.URL.Query().Get("signal")) })
}

func containerRemoveHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	return lifecycleNoContent(ctx, w, p, s, func(b backend.Backend) error { return b.ContainerRemove(ctx, p["id"], force) })
}

func containerPruneHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	if err := s.Backend().ContainerPrune(ctx); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, types.PruneReport{SpaceReclaimed: 0})
	return nil
}

// ---- exec 类 ----

type dockerExecBody struct {
	Cmd          []string `json:"Cmd"`
	Tty          bool     `json:"Tty"`
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Detach       bool     `json:"Detach"`
}

func execCreateHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	var body dockerExecBody
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			return fmt.Errorf("decode exec body: %w", err)
		}
	}
	spec := &backend.ExecSpec{
		Cmd:          body.Cmd,
		Tty:          body.Tty,
		AttachStdin:  body.AttachStdin,
		AttachStdout: body.AttachStdout,
		AttachStderr: body.AttachStderr,
	}
	id, err := s.Backend().ExecCreate(ctx, p["id"], spec)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, types.IDResponse{ID: id})
	return nil
}

func execStartHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	var body dockerExecBody
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	spec := &backend.ExecSpec{
		Cmd:         body.Cmd,
		Tty:         body.Tty,
		AttachStdin: body.AttachStdin,
	}
	rc, err := s.Backend().ExecStart(ctx, p["id"], spec)
	if err != nil {
		return err
	}
	return streamCopy(w, "application/vnd.docker.raw-stream", rc)
}

func execResizeHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	// 终端尺寸调整需要持有 exec 会话的 tty fd；当前会话模型不常驻 fd，
	// 这里接受请求并返回 204（尽力而为），避免面板终端报错。
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func execInspectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	ei, err := s.Backend().ExecInspect(ctx, p["id"])
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, ei)
	return nil
}

// ---- 镜像类 ----

func imageListHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	list, err := s.Backend().ImageList(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, list)
	return nil
}

func imageCreateHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, _ map[string]string, s *Server) error {
	q := r.URL.Query()
	ref := q.Get("fromImage")
	if ref == "" {
		ref = q.Get("repo")
	}
	tag := q.Get("tag")
	if ref == "" && tag == "" {
		// 兼容 POST 体为 "image:tag" 的少数客户端
		ref = strings.TrimSpace(q.Get("fromSrc"))
	}
	if tag != "" && ref != "" && !strings.Contains(ref, ":") {
		ref = ref + ":" + tag
	}
	if ref == "" {
		ref = "library/unknown:latest"
	}
	rc, err := s.Backend().ImagePull(ctx, ref)
	if err != nil {
		return err
	}
	return streamCopy(w, "application/json", rc)
}

func imageInspectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	ii, err := s.Backend().ImageInspect(ctx, p["id"])
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, ii)
	return nil
}

func imageRemoveHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.Backend().ImageRemove(ctx, p["id"], force); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"Untagged": p["id"], "Deleted": []string{p["id"]}})
	return nil
}

// ---- 网络 / 卷 inspect ----

func networkInspectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	n, err := s.Backend().NetworkInspect(ctx, p["id"])
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, n)
	return nil
}

func volumeInspectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, p map[string]string, s *Server) error {
	v, err := s.Backend().VolumeInspect(ctx, p["name"])
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}
