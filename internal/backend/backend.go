// Package backend 定义 wslc2docker 的「翻译后端」接口：把 Docker Engine API 的语义
// 落地到具体引擎（当前为 WSLC 的 wslc.exe CLI；另提供 fake 后端用于自测与演示）。
package backend

import (
	"context"
	"io"

	"github.com/wslc2docker/wslc2docker/internal/types"
)

// ContainerCreateSpec 是 POST /containers/create 入参经转换后的内部形态。
type ContainerCreateSpec struct {
	Name          string
	Image         string
	Cmd           []string
	Entrypoint    []string
	Env           []string
	WorkingDir    string
	Labels        map[string]string
	Tty           bool
	OpenStdin     bool
	AttachStdin   bool
	ExposedPorts  map[string]struct{} // 形如 "80/tcp"
	PortBindings  map[string][]types.PortBinding
	Binds         []string
	Mounts        []types.Mount
	RestartPolicy string
}

// ExecSpec 是 exec 创建/启动的内部形态。
type ExecSpec struct {
	Cmd          []string
	Tty          bool
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
}

// LogsOptions 是 GET /containers/{id}/logs 的选项。
type LogsOptions struct {
	ShowStdout bool
	ShowStderr bool
	Follow     bool
	Timestamps bool
	Since      string
	Until      string
	Tail       string
}

// Backend 是 wslc2docker 抽象的容器引擎后端。
type Backend interface {
	// Name 返回后端标识（"wslc" / "fake"）。
	Name() string

	// Ping 健康检查（恒 OK）。
	Ping(ctx context.Context) error

	// Version 返回伪装成 dockerd 的版本信息。
	Version(ctx context.Context) (*types.Version, error)

	// ContainerList 列举容器；all=true 含已停止。
	ContainerList(ctx context.Context, all bool) ([]types.Container, error)

	// ImageList 列举镜像。
	ImageList(ctx context.Context) ([]types.Image, error)

	// ContainerCreate 创建容器，返回引擎侧 ID/Name。
	ContainerCreate(ctx context.Context, spec *ContainerCreateSpec) (id string, err error)

	// 容器生命周期。
	ContainerStart(ctx context.Context, id string) error
	ContainerStop(ctx context.Context, id string, timeoutSec int) error
	ContainerRestart(ctx context.Context, id string, timeoutSec int) error
	ContainerKill(ctx context.Context, id, signal string) error
	ContainerRemove(ctx context.Context, id string, force bool) error

	// ContainerInspect 返回完整容器详情（Moby schema）。
	ContainerInspect(ctx context.Context, id string) (*types.ContainerInspect, error)

	// ContainerLogs 返回日志流（调用方负责拷贝到响应）。
	ContainerLogs(ctx context.Context, id string, opts LogsOptions) (io.Reader, error)

	// ImagePull 拉取镜像，返回进度 JSON 流。
	ImagePull(ctx context.Context, ref string) (io.Reader, error)

	// ImageInspect 返回完整镜像详情。
	ImageInspect(ctx context.Context, id string) (*types.ImageInspect, error)

	// ImageRemove 删除镜像。
	ImageRemove(ctx context.Context, id string, force bool) error

	// ContainerPrune 清理停止的容器。
	ContainerPrune(ctx context.Context) error

	// Info 聚合系统信息。
	Info(ctx context.Context) (*types.Info, error)

	// SystemDF 聚合磁盘占用（/system/df）。
	SystemDF(ctx context.Context) (*types.DiskUsage, error)

	// NetworkInspect / VolumeInspect 单资源详情。
	NetworkInspect(ctx context.Context, id string) (*types.Network, error)
	VolumeInspect(ctx context.Context, name string) (*types.Volume, error)

	// Exec 会话。
	ExecCreate(ctx context.Context, containerID string, spec *ExecSpec) (execID string, err error)
	ExecStart(ctx context.Context, execID string, spec *ExecSpec) (io.Reader, error)
	ExecInspect(ctx context.Context, execID string) (*types.ExecInspect, error)
}
