package backend

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/wslc2docker/wslc2docker/internal/types"
)

// FakeBackend 返回样例数据，用于在无 WSLC 的环境自测 / 演示 wslc2docker 的
// HTTP 层与响应塑形。可通过 endpoints.yaml 的 backend: fake 启用。
type FakeBackend struct {
	mu    sync.Mutex
	execs map[string]*execSession
}

// NewFake 构造 fake 后端。
func NewFake() *FakeBackend {
	return &FakeBackend{execs: make(map[string]*execSession)}
}

// Name 实现 Backend。
func (b *FakeBackend) Name() string { return "fake" }

// Ping 实现 Backend。
func (b *FakeBackend) Ping(ctx context.Context) error { return nil }

// Version 实现 Backend。
func (b *FakeBackend) Version(ctx context.Context) (*types.Version, error) {
	v := &types.Version{}
	v.Platform.Name = "wslc2docker (FAKE backend)"
	v.Version = "3.0.1-fake"
	v.ApiVersion = apiVersion
	v.MinAPIVersion = "1.24"
	v.Os = "linux"
	v.Arch = "amd64"
	v.Experimental = false
	v.Components = []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
		Details string `json:"Details,omitempty"`
	}{{Name: "wslc", Version: "3.0.1-fake"}}
	return v, nil
}

// ContainerList 实现 Backend。
func (b *FakeBackend) ContainerList(ctx context.Context, all bool) ([]types.Container, error) {
	return []types.Container{
		{
			ID:      "abc123def456",
			Names:   []string{"/web"},
			Image:   "nginx:latest",
			ImageID: "sha256:fakeimage",
			Command: "nginx -g daemon off;",
			Created: 1700000000,
			State:   "running",
			Status:  "Up 5 minutes",
			Ports:   []types.Port{{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"}},
		},
	}, nil
}

// ImageList 实现 Backend。
func (b *FakeBackend) ImageList(ctx context.Context) ([]types.Image, error) {
	return []types.Image{
		{
			ID:          "sha256:fakeimage",
			RepoTags:    []string{"nginx:latest"},
			RepoDigests: []string{"nginx@sha256:fake"},
			Created:     1690000000,
			Size:        60000000,
			VirtualSize: 60000000,
		},
	}, nil
}

// ContainerCreate 实现 Backend。
func (b *FakeBackend) ContainerCreate(ctx context.Context, spec *ContainerCreateSpec) (string, error) {
	name := spec.Name
	if name == "" {
		name = "w2d-" + randShort()
	}
	return name, nil
}

// ContainerStart 实现 Backend。
func (b *FakeBackend) ContainerStart(ctx context.Context, id string) error { return nil }

// ContainerStop 实现 Backend。
func (b *FakeBackend) ContainerStop(ctx context.Context, id string, timeoutSec int) error { return nil }

// ContainerRestart 实现 Backend。
func (b *FakeBackend) ContainerRestart(ctx context.Context, id string, timeoutSec int) error {
	return nil
}

// ContainerKill 实现 Backend。
func (b *FakeBackend) ContainerKill(ctx context.Context, id, signal string) error { return nil }

// ContainerRemove 实现 Backend。
func (b *FakeBackend) ContainerRemove(ctx context.Context, id string, force bool) error { return nil }

// ContainerInspect 实现 Backend。
func (b *FakeBackend) ContainerInspect(ctx context.Context, id string) (*types.ContainerInspect, error) {
	return &types.ContainerInspect{
		ID:      id,
		Name:    "/" + id,
		Created: "2023-11-14T22:13:20Z",
		Path:    "nginx",
		Args:    []string{"-g", "daemon off;"},
		State: &types.ContainerState{
			Status:    "running",
			Running:   true,
			Pid:       42,
			StartedAt: "2023-11-14T22:13:20Z",
			ExitCode:  0,
		},
		Image: "sha256:fakeimage",
		Config: &types.ContainerConfig{
			Hostname: id,
			Image:    "nginx:latest",
			Tty:      false,
			Env:      []string{"PATH=/usr/local/sbin:/usr/local/bin"},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*types.NetworkInfo{"bridge": {IPAddress: "172.17.0.2"}},
			Ports:    map[string][]types.PortBinding{"80/tcp": {{HostIP: "0.0.0.0", HostPort: "8080"}}},
		},
	}, nil
}

// ContainerLogs 实现 Backend。
func (b *FakeBackend) ContainerLogs(ctx context.Context, id string, opts LogsOptions) (io.Reader, error) {
	return io.NopCloser(strings.NewReader("fake log line 1\nfake log line 2\n")), nil
}

// ImagePull 实现 Backend。
func (b *FakeBackend) ImagePull(ctx context.Context, ref string) (io.Reader, error) {
	progress := fmt.Sprintf(`{"status":"Pulling from library/%s","id":"latest"}
{"status":"Downloaded","id":"latest"}
{"status":"Pull complete","id":"latest"}
`, ref)
	return io.NopCloser(strings.NewReader(progress)), nil
}

// ImageInspect 实现 Backend。
func (b *FakeBackend) ImageInspect(ctx context.Context, id string) (*types.ImageInspect, error) {
	return &types.ImageInspect{
		ID:           id,
		RepoTags:     []string{"nginx:latest"},
		Created:      "2023-11-01T00:00:00Z",
		Architecture: "amd64",
		Os:           "linux",
		Size:         60000000,
		VirtualSize:  60000000,
		RootFS:       &types.RootFS{Type: "layers", Layers: []string{"sha256:fake-layer"}},
	}, nil
}

// ImageRemove 实现 Backend。
func (b *FakeBackend) ImageRemove(ctx context.Context, id string, force bool) error { return nil }

// ContainerPrune 实现 Backend。
func (b *FakeBackend) ContainerPrune(ctx context.Context) error { return nil }

// Info 实现 Backend。
func (b *FakeBackend) Info(ctx context.Context) (*types.Info, error) {
	return &types.Info{
		ID:                "fake-node-1",
		Containers:        1,
		ContainersRunning: 1,
		ContainersStopped: 0,
		Images:            1,
		Driver:            "wslc",
		OSType:            "linux",
		Architecture:      "x86_64",
		Experimental:      false,
	}, nil
}

// SystemDF 实现 Backend。
func (b *FakeBackend) SystemDF(ctx context.Context) (*types.DiskUsage, error) {
	containers, _ := b.ContainerList(ctx, true)
	images, _ := b.ImageList(ctx)
	return &types.DiskUsage{
		LayersSize: 60000000,
		Containers: containers,
		Images:     images,
		Volumes:    []types.Volume{},
	}, nil
}

// NetworkInspect 实现 Backend。
func (b *FakeBackend) NetworkInspect(ctx context.Context, id string) (*types.Network, error) {
	return &types.Network{
		Name:   id,
		ID:     id,
		Driver: "bridge",
		IPAM:   &types.IPAM{Driver: "default", Config: []types.IPAMConfig{{Subnet: "172.17.0.0/16", Gateway: "172.17.0.1"}}},
	}, nil
}

// VolumeInspect 实现 Backend。
func (b *FakeBackend) VolumeInspect(ctx context.Context, name string) (*types.Volume, error) {
	return &types.Volume{Name: name, Driver: "local", Mountpoint: "/mnt/wslc/volumes/" + name}, nil
}

// ExecCreate 实现 Backend。
func (b *FakeBackend) ExecCreate(ctx context.Context, containerID string, spec *ExecSpec) (string, error) {
	id := "w2d-exec-" + randShort()
	b.mu.Lock()
	b.execs[id] = &execSession{containerID: containerID, spec: spec}
	b.mu.Unlock()
	return id, nil
}

// ExecStart 实现 Backend。
func (b *FakeBackend) ExecStart(ctx context.Context, execID string, spec *ExecSpec) (io.Reader, error) {
	return io.NopCloser(strings.NewReader("fake exec output\n")), nil
}

// ExecInspect 实现 Backend。
func (b *FakeBackend) ExecInspect(ctx context.Context, execID string) (*types.ExecInspect, error) {
	b.mu.Lock()
	sess, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("exec session %s not found", execID)
	}
	return &types.ExecInspect{
		ID:           execID,
		ContainerID:  sess.containerID,
		Running:      false,
		Cmd:          sess.spec.Cmd,
		Tty:          sess.spec.Tty,
		AttachStdin:  sess.spec.AttachStdin,
		AttachStdout: sess.spec.AttachStdout,
		AttachStderr: sess.spec.AttachStderr,
	}, nil
}
