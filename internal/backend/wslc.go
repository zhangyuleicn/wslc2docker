package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wslc2docker/wslc2docker/internal/types"
)

// randShort 生成短随机后缀，用于确定性容器名 / execID。
func randShort() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, 8)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

// apiVersion 是 wslc2docker 对外宣称的 Docker Engine API 版本。
const apiVersion = "1.56"

// WSLCBackend 通过 WSL interop 调用 wslc.exe（接口面 A）。
//
// 注意：wslc 的 --format json 输出 schema 以官方 GA 版为准；本文件中的解析为
// 「尽力而为」映射，部署到真实 WSLC 时应以实际输出校准字段名（集中在下方
// 各 parse* 函数，便于单点修正）。
type WSLCBackend struct {
	binary  string
	version string
	mu      sync.Mutex
	execs   map[string]*execSession // execID -> 会话
}

// execSession 记录 exec 创建时的容器与命令，供 start 复用。
type execSession struct {
	containerID string
	spec        *ExecSpec
}

// NewWSLC 构造 WSLC 后端。
func NewWSLC(binary, version string) *WSLCBackend {
	if binary == "" {
		binary = "wslc.exe"
	}
	if version == "" {
		version = "wslc2docker-shim"
	}
	return &WSLCBackend{binary: binary, version: version, execs: make(map[string]*execSession)}
}

// Name 实现 Backend。
func (b *WSLCBackend) Name() string { return "wslc" }

// run 同步执行 wslc 命令并捕获 stdout；stderr 在出错时附到 error。
func (b *WSLCBackend) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.binary, args...)
	var out, eout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &eout
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("wslc %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(eout.String()))
	}
	return out.Bytes(), nil
}

// stream 启动命令并以流式返回 stdout（用于 logs / pull / exec）。
// 返回的 readCloser 在 Close 时会终止底层命令（支持客户端断开即停止 follow）。
func (b *WSLCBackend) stream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, b.binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &cmdStream{Reader: stdout, cmd: cmd}
	go func() { _ = cmd.Wait() }()
	return s, nil
}

type cmdStream struct {
	io.Reader
	cmd  *exec.Cmd
	once sync.Once
}

func (s *cmdStream) Close() error {
	s.once.Do(func() { _ = s.cmd.Process.Kill() })
	return nil
}

// Ping 实现 Backend（恒 OK）。
func (b *WSLCBackend) Ping(ctx context.Context) error { return nil }

// Version 实现 Backend，把 wslc 版本填入 Moby Version 结构。
func (b *WSLCBackend) Version(ctx context.Context) (*types.Version, error) {
	v := &types.Version{}
	v.Platform.Name = "wslc2docker (WSLC shim)"
	v.Version = b.version
	v.ApiVersion = apiVersion
	v.MinAPIVersion = "1.24"
	v.Experimental = false
	v.Os = "linux"
	v.Arch = "amd64"
	v.Components = []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
		Details string `json:"Details,omitempty"`
	}{
		{Name: "wslc", Version: b.version},
	}
	if out, err := b.run(ctx, "version"); err == nil {
		// 尽力从 wslc version 输出中提取版本号（如 "wslc version 3.0.1"）。
		if f := extractVersion(string(out)); f != "" {
			v.Components[0].Version = f
			v.Version = f
		}
	}
	return v, nil
}

// wslcContainer 是 wslc container list --format json 的尽力而为映射结构。
type wslcContainer struct {
	ID      string            `json:"ID"`
	Names   []string          `json:"Names"`
	Name    string            `json:"Name"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Ports   []types.Port      `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

// ContainerList 实现 Backend。
func (b *WSLCBackend) ContainerList(ctx context.Context, all bool) ([]types.Container, error) {
	args := []string{"container", "list", "--format", "json"}
	if all {
		args = append(args, "--all")
	}
	out, err := b.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var raw []wslcContainer
	if len(bytes.TrimSpace(out)) == 0 {
		return []types.Container{}, nil
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse container list: %w", err)
	}
	res := make([]types.Container, 0, len(raw))
	for _, c := range raw {
		names := c.Names
		if len(names) == 0 && c.Name != "" {
			names = []string{"/" + c.Name}
		}
		res = append(res, types.Container{
			ID:      c.ID,
			Names:   names,
			Image:   c.Image,
			ImageID: c.ImageID,
			Command: c.Command,
			Created: c.Created,
			Ports:   c.Ports,
			Labels:  c.Labels,
			State:   c.State,
			Status:  c.Status,
		})
	}
	return res, nil
}

// wslcImage 是 wslc image list --format json 的尽力而为映射结构。
type wslcImage struct {
	ID          string            `json:"ID"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	VirtualSize int64             `json:"VirtualSize"`
	Labels      map[string]string `json:"Labels"`
}

// ImageList 实现 Backend。
func (b *WSLCBackend) ImageList(ctx context.Context) ([]types.Image, error) {
	out, err := b.run(ctx, "image", "list", "--format", "json")
	if err != nil {
		return nil, err
	}
	var raw []wslcImage
	if len(bytes.TrimSpace(out)) == 0 {
		return []types.Image{}, nil
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse image list: %w", err)
	}
	res := make([]types.Image, 0, len(raw))
	for _, im := range raw {
		res = append(res, types.Image{
			ID:          im.ID,
			RepoTags:    im.RepoTags,
			RepoDigests: im.RepoDigests,
			Created:     im.Created,
			Size:        im.Size,
			VirtualSize: im.VirtualSize,
			Labels:      im.Labels,
		})
	}
	return res, nil
}

// ContainerCreate 实现 Backend：用 wslc container create 创建（不启动），返回 name。
func (b *WSLCBackend) ContainerCreate(ctx context.Context, spec *ContainerCreateSpec) (string, error) {
	name := spec.Name
	if name == "" {
		name = "w2d-" + randShort()
	}
	args := []string{"container", "create", "--name", name}
	if spec.WorkingDir != "" {
		args = append(args, "--workdir", spec.WorkingDir)
	}
	for _, e := range spec.Env {
		args = append(args, "--env", e)
	}
	for k, v := range spec.Labels {
		args = append(args, "--label", k+"="+v)
	}
	if spec.Tty {
		args = append(args, "--tty")
	}
	for port := range spec.ExposedPorts {
		// port 形如 80/tcp；仅发布端口，绑定由 PortBindings 决定
		args = append(args, "--expose", strings.TrimSuffix(port, "/tcp"))
	}
	for _, binds := range spec.Binds {
		args = append(args, "--volume", binds)
	}
	for _, m := range spec.Mounts {
		args = append(args, "--mount", fmt.Sprintf("type=%s,src=%s,dst=%s", m.Type, m.Source, m.Target))
	}
	for pub, bindings := range spec.PortBindings {
		for _, bd := range bindings {
			hp := bd.HostPort
			if hp == "" {
				hp = pub
			}
			args = append(args, "--publish", hp+":"+pub)
		}
	}
	if spec.Entrypoint != nil {
		args = append(args, "--entrypoint", strings.Join(spec.Entrypoint, " "))
	}
	if spec.Image == "" {
		return "", fmt.Errorf("container create: missing image")
	}
	args = append(args, spec.Image)
	if len(spec.Cmd) > 0 {
		args = append(args, spec.Cmd...)
	}
	if _, err := b.run(ctx, args...); err != nil {
		return "", err
	}
	return name, nil
}

// ContainerStart 实现 Backend。
func (b *WSLCBackend) ContainerStart(ctx context.Context, id string) error {
	_, err := b.run(ctx, "container", "start", id)
	return err
}

// ContainerStop 实现 Backend（超时默认 5s）。
func (b *WSLCBackend) ContainerStop(ctx context.Context, id string, timeoutSec int) error {
	args := []string{"container", "stop"}
	if timeoutSec > 0 {
		args = append(args, "--time", strconv.Itoa(timeoutSec))
	}
	args = append(args, id)
	_, err := b.run(ctx, args...)
	return err
}

// ContainerRestart 实现 Backend：wslc 无 restart，用 stop+start 模拟。
func (b *WSLCBackend) ContainerRestart(ctx context.Context, id string, timeoutSec int) error {
	if err := b.ContainerStop(ctx, id, timeoutSec); err != nil {
		return err
	}
	return b.ContainerStart(ctx, id)
}

// ContainerKill 实现 Backend。
func (b *WSLCBackend) ContainerKill(ctx context.Context, id, signal string) error {
	args := []string{"kill"}
	if signal != "" {
		args = append(args, "--signal", signal)
	}
	args = append(args, id)
	_, err := b.run(ctx, args...)
	return err
}

// ContainerRemove 实现 Backend。
func (b *WSLCBackend) ContainerRemove(ctx context.Context, id string, force bool) error {
	args := []string{"container", "rm"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, id)
	_, err := b.run(ctx, args...)
	return err
}

// ContainerInspect 实现 Backend：wslc inspect -t container → OCI → Moby 重塑。
func (b *WSLCBackend) ContainerInspect(ctx context.Context, id string) (*types.ContainerInspect, error) {
	out, err := b.run(ctx, "inspect", "-t", "container", id)
	if err != nil {
		return nil, err
	}
	return reshapeContainerInspect(out)
}

// ContainerLogs 实现 Backend。
func (b *WSLCBackend) ContainerLogs(ctx context.Context, id string, opts LogsOptions) (io.Reader, error) {
	args := []string{"logs"}
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Timestamps {
		args = append(args, "--timestamps")
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}
	if opts.Until != "" {
		args = append(args, "--until", opts.Until)
	}
	if opts.Tail != "" && opts.Tail != "all" {
		args = append(args, "--tail", opts.Tail)
	}
	args = append(args, id)
	return b.stream(ctx, args...)
}

// ImagePull 实现 Backend。
func (b *WSLCBackend) ImagePull(ctx context.Context, ref string) (io.Reader, error) {
	return b.stream(ctx, "pull", ref)
}

// ImageInspect 实现 Backend。
func (b *WSLCBackend) ImageInspect(ctx context.Context, id string) (*types.ImageInspect, error) {
	out, err := b.run(ctx, "inspect", "-t", "image", id)
	if err != nil {
		return nil, err
	}
	return reshapeImageInspect(out)
}

// ImageRemove 实现 Backend。
func (b *WSLCBackend) ImageRemove(ctx context.Context, id string, force bool) error {
	args := []string{"image", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, id)
	_, err := b.run(ctx, args...)
	return err
}

// ContainerPrune 实现 Backend。
func (b *WSLCBackend) ContainerPrune(ctx context.Context) error {
	_, err := b.run(ctx, "container", "prune", "--force")
	return err
}

// Info 实现 Backend：聚合各 noun 计数合成。
func (b *WSLCBackend) Info(ctx context.Context) (*types.Info, error) {
	info := &types.Info{Driver: "wslc", OSType: "linux", Architecture: "x86_64", Experimental: false}
	containers, err := b.ContainerList(ctx, true)
	if err == nil {
		info.Containers = len(containers)
		for _, c := range containers {
			switch c.State {
			case "running":
				info.ContainersRunning++
			case "paused":
				info.ContainersPaused++
			default:
				info.ContainersStopped++
			}
		}
	}
	images, err := b.ImageList(ctx)
	if err == nil {
		info.Images = len(images)
	}
	return info, nil
}

// SystemDF 实现 Backend：聚合各 noun list 计算占用。
func (b *WSLCBackend) SystemDF(ctx context.Context) (*types.DiskUsage, error) {
	du := &types.DiskUsage{}
	containers, err := b.ContainerList(ctx, true)
	if err == nil {
		du.Containers = containers
	}
	images, err := b.ImageList(ctx)
	if err == nil {
		du.Images = images
		for _, im := range images {
			du.LayersSize += im.VirtualSize
		}
	}
	return du, nil
}

// NetworkInspect 实现 Backend。
func (b *WSLCBackend) NetworkInspect(ctx context.Context, id string) (*types.Network, error) {
	out, err := b.run(ctx, "inspect", "-t", "network", id)
	if err != nil {
		return nil, err
	}
	return reshapeNetworkInspect(out)
}

// VolumeInspect 实现 Backend。
func (b *WSLCBackend) VolumeInspect(ctx context.Context, name string) (*types.Volume, error) {
	out, err := b.run(ctx, "inspect", "-t", "volume", name)
	if err != nil {
		return nil, err
	}
	return reshapeVolumeInspect(out)
}

// ExecCreate 实现 Backend：记录 exec→container 映射，返回生成的 execID。
func (b *WSLCBackend) ExecCreate(ctx context.Context, containerID string, spec *ExecSpec) (string, error) {
	id := "w2d-exec-" + randShort()
	b.mu.Lock()
	b.execs[id] = &execSession{containerID: containerID, spec: spec}
	b.mu.Unlock()
	return id, nil
}

// ExecStart 实现 Backend：查回容器后执行 wslc exec 并流式返回。
func (b *WSLCBackend) ExecStart(ctx context.Context, execID string, spec *ExecSpec) (io.Reader, error) {
	b.mu.Lock()
	sess, ok := b.execs[execID]
	b.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("exec session %s not found", execID)
	}
	cmd := sess.spec.Cmd
	if len(spec.Cmd) > 0 {
		cmd = spec.Cmd
	}
	tty := sess.spec.Tty || spec.Tty
	interactive := sess.spec.AttachStdin || spec.AttachStdin
	args := []string{"exec"}
	if tty {
		args = append(args, "--tty")
	}
	if interactive {
		args = append(args, "--interactive")
	}
	args = append(args, sess.containerID)
	args = append(args, cmd...)
	return b.stream(ctx, args...)
}

// ExecInspect 实现 Backend。
func (b *WSLCBackend) ExecInspect(ctx context.Context, execID string) (*types.ExecInspect, error) {
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

// ---- 以下为「OCI/通用 JSON → Moby schema」的尽力而为重塑函数 ----

func reshapeContainerInspect(out []byte) (*types.ContainerInspect, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("parse container inspect: %w", err)
	}
	ci := &types.ContainerInspect{}
	ci.ID = str(m, "Id", "ID")
	ci.Name = str(m, "Name")
	ci.Created = str(m, "Created")
	if st, ok := m["State"].(map[string]interface{}); ok {
		ci.State = &types.ContainerState{
			Status:     str(st, "Status"),
			Running:    boolean(st, "Running"),
			Paused:     boolean(st, "Paused"),
			Restarting: boolean(st, "Restarting"),
			OOMKilled:  boolean(st, "OOMKilled"),
			Dead:       boolean(st, "Dead"),
			Pid:        intNum(st, "Pid"),
			ExitCode:   intNum(st, "ExitCode"),
			StartedAt:  str(st, "StartedAt"),
			FinishedAt: str(st, "FinishedAt"),
		}
	}
	if cfg, ok := m["Config"].(map[string]interface{}); ok {
		ci.Config = &types.ContainerConfig{
			Hostname:   str(cfg, "Hostname"),
			Image:      str(cfg, "Image"),
			Cmd:        strSlice(cfg, "Cmd"),
			Entrypoint: strSlice(cfg, "Entrypoint"),
			Env:        strSlice(cfg, "Env"),
			Labels:     strMap(cfg, "Labels"),
			Tty:        boolean(cfg, "Tty"),
		}
	}
	if ns, ok := m["NetworkSettings"].(map[string]interface{}); ok {
		ci.NetworkSettings = &types.NetworkSettings{
			Networks: map[string]*types.NetworkInfo{},
			Ports:    map[string][]types.PortBinding{},
		}
		if def, ok := ns["Networks"].(map[string]interface{}); ok {
			for k, v := range def {
				if vm, ok := v.(map[string]interface{}); ok {
					ci.NetworkSettings.Networks[k] = &types.NetworkInfo{
						IPAddress: str(vm, "IPAddress"),
						Gateway:   str(vm, "Gateway"),
					}
				}
			}
		}
	}
	return ci, nil
}

func reshapeImageInspect(out []byte) (*types.ImageInspect, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("parse image inspect: %w", err)
	}
	ii := &types.ImageInspect{}
	ii.ID = str(m, "Id", "ID")
	ii.RepoTags = strSlice(m, "RepoTags")
	ii.RepoDigests = strSlice(m, "RepoDigests")
	ii.Created = str(m, "Created")
	ii.Architecture = str(m, "Architecture")
	ii.Os = str(m, "Os")
	ii.Author = str(m, "Author")
	if v, ok := m["Size"].(float64); ok {
		ii.Size = int64(v)
	}
	if v, ok := m["VirtualSize"].(float64); ok {
		ii.VirtualSize = int64(v)
	}
	return ii, nil
}

func reshapeNetworkInspect(out []byte) (*types.Network, error) {
	var n types.Network
	if err := json.Unmarshal(out, &n); err != nil {
		return nil, fmt.Errorf("parse network inspect: %w", err)
	}
	return &n, nil
}

func reshapeVolumeInspect(out []byte) (*types.Volume, error) {
	var v types.Volume
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("parse volume inspect: %w", err)
	}
	return &v, nil
}

// ---- 小而稳的 JSON 取值辅助 ----

func str(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func boolean(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func intNum(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func strSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			res := make([]string, 0, len(arr))
			for _, e := range arr {
				if s, ok := e.(string); ok {
					res = append(res, s)
				}
			}
			return res
		}
	}
	return nil
}

func strMap(m map[string]interface{}, key string) map[string]string {
	if v, ok := m[key]; ok {
		if mp, ok := v.(map[string]interface{}); ok {
			res := make(map[string]string, len(mp))
			for k, val := range mp {
				if s, ok := val.(string); ok {
					res[k] = s
				}
			}
			return res
		}
	}
	return nil
}

func extractVersion(s string) string {
	f := strings.Fields(s)
	for i, w := range f {
		if w == "version" && i+1 < len(f) {
			return f[i+1]
		}
	}
	// 兜底：找 x.y.z 形态
	for _, w := range f {
		if len(w) >= 3 && w[0] >= '0' && w[0] <= '9' && strings.Count(w, ".") >= 1 {
			return w
		}
	}
	return ""
}
