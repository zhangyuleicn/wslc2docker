// Command wslc2docker 是 wslc2docker 的入口：加载 endpoints.yaml，构造后端，
// 启动 Docker Engine API 兼容服务（unix socket + 可选 TCP），并处理 SIGHUP 热加载
// 与 SIGTERM/SIGINT 优雅退出。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wslc2docker/wslc2docker/internal/api"
	"github.com/wslc2docker/wslc2docker/internal/backend"
	"github.com/wslc2docker/wslc2docker/internal/config"
)

// buildVersion 由 -ldflags "-X main.buildVersion=..." 注入；默认占位。
var buildVersion = "dev"

func main() {
	var (
		socket   = flag.String("socket", "", "unix socket 路径（覆盖 endpoints.yaml 的 socket）")
		cfgPath  = flag.String("config", "/etc/wslc2docker/endpoints.yaml", "endpoints.yaml 路径")
		tcp      = flag.String("tcp", "", "可选 TCP 监听，如 127.0.0.1:2375（覆盖 yaml 的 tcp）")
		backendN = flag.String("backend", "", "后端类型：wslc | fake（覆盖 yaml 的 backend）")
		wslcBin  = flag.String("wslc", "", "wslc 可执行文件名（覆盖 yaml 的 wslc_binary）")
		logLevel = flag.String("log-level", "", "日志级别（覆盖 yaml 的 log_level）")
		showVer  = flag.Bool("version", false, "打印版本并退出")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("wslc2docker", buildVersion)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// 命令行覆盖
	if *socket == "none" {
		cfg.Socket = "" // 跳过 unix socket（仅 TCP）
	} else if *socket != "" {
		cfg.Socket = *socket
	}
	if *tcp != "" {
		cfg.TCP = *tcp
	}
	if *backendN != "" {
		cfg.Backend = *backendN
	}
	if *wslcBin != "" {
		cfg.WslcBinary = *wslcBin
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}

	var be backend.Backend
	switch cfg.Backend {
	case "fake":
		be = backend.NewFake()
	default:
		be = backend.NewWSLC(cfg.WslcBinary, buildVersion)
	}
	log.Printf("wslc2docker %s | backend=%s | socket=%s | tcp=%q", buildVersion, be.Name(), cfg.Socket, cfg.TCP)

	srv := api.New(cfg, *cfgPath, be)
	srv.SetLogger(func(format string, args ...interface{}) {
		log.Printf(format, args...)
	})

	if err := srv.Listen(); err != nil {
		log.Fatalf("listen: %v", err)
	}

	// 信号：SIGHUP 热加载；SIGTERM/SIGINT 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGHUP:
				if err := srv.Reload(); err != nil {
					log.Printf("SIGHUP reload failed: %v", err)
				}
			case syscall.SIGTERM, syscall.SIGINT:
				log.Printf("received %v, shutting down...", sig)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = srv.Shutdown(ctx)
				cancel()
				os.Exit(0)
			}
		}
	}()

	log.Println("wslc2docker ready")
	select {} // 常驻
}
