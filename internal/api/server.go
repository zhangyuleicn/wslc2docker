// Package api 实现 wslc2docker 的 Docker Engine API 兼容服务：监听 unix socket
// （+ 可选 TCP），按 endpoints.yaml 的数据驱动路由分发到对应处理器。
package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/wslc2docker/wslc2docker/internal/backend"
	"github.com/wslc2docker/wslc2docker/internal/config"
)

// Server 是 wslc2docker 的 HTTP 服务核心。
type Server struct {
	mu        sync.RWMutex
	cfg       *config.Config
	cfgPath   string
	backend   backend.Backend
	listeners []net.Listener
	httpSrv   *http.Server
	// onReload 在 SIGHUP 重新加载后被调用（可选钩子，便于日志）。
	logFn func(format string, args ...interface{})
}

// New 构造 Server。
func New(cfg *config.Config, cfgPath string, be backend.Backend) *Server {
	s := &Server{cfg: cfg, cfgPath: cfgPath, backend: be}
	s.httpSrv = &http.Server{Handler: s}
	return s
}

// SetLogger 设置日志回调。
func (s *Server) SetLogger(f func(string, ...interface{})) { s.logFn = f }

func (s *Server) logf(format string, args ...interface{}) {
	if s.logFn != nil {
		s.logFn(format, args...)
	}
}

// Backend 暴露后端（处理器使用）。
func (s *Server) Backend() backend.Backend { return s.backend }

// Cfg 返回当前配置（处理器做 enabled 判断时用，已在上层判断）。
func (s *Server) Cfg() *config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Listen 创建 unix socket（+ 可选 TCP）并开始服务。非阻塞（内部 goroutine 服务）。
func (s *Server) Listen() error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	// unix socket：先清理可能残留的旧文件
	if cfg.Socket != "" {
		_ = os.Remove(cfg.Socket)
		l, err := net.Listen("unix", cfg.Socket)
		if err != nil {
			return fmt.Errorf("listen unix %s: %w", cfg.Socket, err)
		}
		_ = os.Chmod(cfg.Socket, 0660) // root 仅权限，与 dockerd 一致
		s.listeners = append(s.listeners, l)
		s.logf("listening on unix socket %s", cfg.Socket)
		go func() { _ = s.httpSrv.Serve(l) }()
	}

	if cfg.TCP != "" {
		l, err := net.Listen("tcp", cfg.TCP)
		if err != nil {
			return fmt.Errorf("listen tcp %s: %w", cfg.TCP, err)
		}
		s.listeners = append(s.listeners, l)
		s.logf("listening on tcp %s", cfg.TCP)
		go func() { _ = s.httpSrv.Serve(l) }()
	}
	return nil
}

// Reload 重新读取 endpoints.yaml 并热替换配置（SIGHUP 调用）。
func (s *Server) Reload() error {
	newCfg, err := config.Load(s.cfgPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = newCfg
	s.mu.Unlock()
	s.logf("config reloaded from %s", s.cfgPath)
	return nil
}

// Shutdown 优雅停止并清理 unix socket 文件。
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.httpSrv.Shutdown(ctx)
	for _, l := range s.listeners {
		if u, ok := l.(*net.UnixListener); ok {
			_ = os.Remove(u.Addr().String())
		}
		_ = l.Close()
	}
	return err
}

// ServeHTTP 是唯一入口：按 method+path 匹配端点，校验 enabled，分发到处理器。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	ep, params, ok := cfg.Match(r.Method, r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("wslc2docker: %s %s not found", r.Method, r.URL.Path))
		return
	}
	if !ep.Enabled {
		writeError(w, http.StatusNotImplemented,
			fmt.Sprintf("endpoint #%d %s %s is disabled in endpoints.yaml", ep.ID, ep.Method, ep.Path))
		return
	}
	h, ok := handlers[ep.ID]
	if !ok {
		writeError(w, http.StatusNotImplemented,
			fmt.Sprintf("endpoint #%d %s %s not implemented yet (plan %s)", ep.ID, ep.Method, ep.Path, ep.Plan))
		return
	}
	if err := h(r.Context(), w, r, params, s); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
}
