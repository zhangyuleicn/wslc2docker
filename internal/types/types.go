// Package types 定义 wslc2docker 返回给 Docker 客户端的响应结构。
//
// 这些结构体是 Docker Engine API（Moby）对应结构的本地镜像，字段/JSON 标签
// 与真实 dockerd 保持一致，以便 1Panel / Portainer / docker CLI 零修改解析。
// 如需 100% schema 保真，可后续整体替换为 github.com/moby/moby/api/types。
package types

// Port 是 /containers/json 中的端口映射项。
type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort int    `json:"PrivatePort,omitempty"`
	PublicPort  int    `json:"PublicPort,omitempty"`
	Type        string `json:"Type,omitempty"`
}

// Container 是 GET /containers/json 的单条记录。
type Container struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names,omitempty"`
	Image      string            `json:"Image,omitempty"`
	ImageID    string            `json:"ImageID,omitempty"`
	Command    string            `json:"Command,omitempty"`
	Created    int64             `json:"Created,omitempty"`
	Ports      []Port            `json:"Ports,omitempty"`
	SizeRw     int64             `json:"SizeRw,omitempty"`
	SizeRootFs int64             `json:"SizeRootFs,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
	State      string            `json:"State,omitempty"`
	Status     string            `json:"Status,omitempty"`
	HostConfig struct {
		NetworkMode string `json:"NetworkMode,omitempty"`
	} `json:"HostConfig,omitempty"`
}

// ContainerState 容器运行状态（inspect）。
type ContainerState struct {
	Status     string `json:"Status,omitempty"`
	Running    bool   `json:"Running,omitempty"`
	Paused     bool   `json:"Paused,omitempty"`
	Restarting bool   `json:"Restarting,omitempty"`
	OOMKilled  bool   `json:"OOMKilled,omitempty"`
	Dead       bool   `json:"Dead,omitempty"`
	Pid        int    `json:"Pid,omitempty"`
	ExitCode   int    `json:"ExitCode,omitempty"`
	StartedAt  string `json:"StartedAt,omitempty"`
	FinishedAt string `json:"FinishedAt,omitempty"`
}

// ContainerConfig 容器配置（inspect / image）。
type ContainerConfig struct {
	Hostname     string              `json:"Hostname,omitempty"`
	Image        string              `json:"Image,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Entrypoint   []string            `json:"Entrypoint,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
	Tty          bool                `json:"Tty,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
}

// Mount 挂载点（inspect）。
type Mount struct {
	Type     string `json:"Type,omitempty"`
	Source   string `json:"Source,omitempty"`
	Target   string `json:"Target,omitempty"`
	ReadOnly bool   `json:"ReadOnly,omitempty"`
	Driver   string `json:"Driver,omitempty"`
}

// PortBinding 端口绑定。
type PortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

// NetworkInfo inspect 中的网络信息。
type NetworkInfo struct {
	IPAddress string `json:"IPAddress,omitempty"`
	Gateway   string `json:"Gateway,omitempty"`
}

// NetworkSettings 网络设置（inspect）。
type NetworkSettings struct {
	Networks map[string]*NetworkInfo  `json:"Networks,omitempty"`
	Ports    map[string][]PortBinding `json:"Ports,omitempty"`
}

// ContainerInspect 是 GET /containers/{id}/json 的完整响应。
type ContainerInspect struct {
	ID              string           `json:"Id"`
	Created         string           `json:"Created,omitempty"`
	Path            string           `json:"Path,omitempty"`
	Args            []string         `json:"Args,omitempty"`
	State           *ContainerState  `json:"State,omitempty"`
	Image           string           `json:"Image,omitempty"`
	ResolvConfPath  string           `json:"ResolvConfPath,omitempty"`
	Name            string           `json:"Name,omitempty"`
	RestartCount    int              `json:"RestartCount,omitempty"`
	Driver          string           `json:"Driver,omitempty"`
	Platform        string           `json:"Platform,omitempty"`
	Config          *ContainerConfig `json:"Config,omitempty"`
	Mounts          []Mount          `json:"Mounts,omitempty"`
	NetworkSettings *NetworkSettings `json:"NetworkSettings,omitempty"`
}

// Image 是 GET /images/json 的单条记录。
type Image struct {
	ID          string            `json:"Id"`
	RepoTags    []string          `json:"RepoTags,omitempty"`
	RepoDigests []string          `json:"RepoDigests,omitempty"`
	Created     int64             `json:"Created,omitempty"`
	Size        int64             `json:"Size,omitempty"`
	SharedSize  int64             `json:"SharedSize,omitempty"`
	VirtualSize int64             `json:"VirtualSize,omitempty"`
	Labels      map[string]string `json:"Labels,omitempty"`
	Containers  int               `json:"Containers,omitempty"`
}

// RootFS 镜像根文件系统。
type RootFS struct {
	Type   string   `json:"Type,omitempty"`
	Layers []string `json:"Layers,omitempty"`
}

// ImageMetadata 镜像元数据。
type ImageMetadata struct {
	LastTagTime string `json:"LastTagTime,omitempty"`
}

// ImageInspect 是 GET /images/{id}/json 的完整响应。
type ImageInspect struct {
	ID            string           `json:"Id"`
	RepoTags      []string         `json:"RepoTags,omitempty"`
	RepoDigests   []string         `json:"RepoDigests,omitempty"`
	Comment       string           `json:"Comment,omitempty"`
	Created       string           `json:"Created,omitempty"`
	DockerVersion string           `json:"DockerVersion,omitempty"`
	Author        string           `json:"Author,omitempty"`
	Architecture  string           `json:"Architecture,omitempty"`
	Os            string           `json:"Os,omitempty"`
	Size          int64            `json:"Size,omitempty"`
	VirtualSize   int64            `json:"VirtualSize,omitempty"`
	Config        *ContainerConfig `json:"Config,omitempty"`
	RootFS        *RootFS          `json:"RootFS,omitempty"`
	Metadata      *ImageMetadata   `json:"Metadata,omitempty"`
}

// Info 是 GET /info 的响应。
type Info struct {
	ID                string `json:"ID,omitempty"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersPaused  int    `json:"ContainersPaused"`
	ContainersStopped int    `json:"ContainersStopped"`
	Images            int    `json:"Images"`
	Driver            string `json:"Driver,omitempty"`
	ServerVersion     string `json:"ServerVersion,omitempty"`
	OperatingSystem   string `json:"OperatingSystem,omitempty"`
	Architecture      string `json:"Architecture,omitempty"`
	OSType            string `json:"OSType,omitempty"`
	Name              string `json:"Name,omitempty"`
	KernelVersion     string `json:"KernelVersion,omitempty"`
	Experimental      bool   `json:"Experimental"`
	Debug             bool   `json:"Debug"`
	NCPU              int    `json:"NCPU,omitempty"`
	MemTotal          int64  `json:"MemTotal,omitempty"`
}

// VolumeUsageData 卷用量。
type VolumeUsageData struct {
	Size     int64 `json:"Size"`
	RefCount int   `json:"RefCount"`
}

// Volume 卷（inspect / system df）。
type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver,omitempty"`
	Mountpoint string            `json:"Mountpoint,omitempty"`
	Labels     map[string]string `json:"Labels,omitempty"`
	UsageData  *VolumeUsageData  `json:"UsageData,omitempty"`
}

// IPAMConfig IPAM 配置。
type IPAMConfig struct {
	Subnet  string `json:"Subnet,omitempty"`
	Gateway string `json:"Gateway,omitempty"`
}

// IPAM IPAM 信息。
type IPAM struct {
	Driver string       `json:"Driver,omitempty"`
	Config []IPAMConfig `json:"Config,omitempty"`
}

// NetworkContainer 网络中的容器。
type NetworkContainer struct {
	Name        string `json:"Name,omitempty"`
	EndpointID  string `json:"EndpointID,omitempty"`
	MacAddress  string `json:"MacAddress,omitempty"`
	IPv4Address string `json:"IPv4Address,omitempty"`
	IPv6Address string `json:"IPv6Address,omitempty"`
}

// Network 网络（inspect）。
type Network struct {
	Name       string                       `json:"Name"`
	ID         string                       `json:"Id"`
	Created    string                       `json:"Created,omitempty"`
	Scope      string                       `json:"Scope,omitempty"`
	Driver     string                       `json:"Driver,omitempty"`
	EnableIPv6 bool                         `json:"EnableIPv6,omitempty"`
	IPAM       *IPAM                        `json:"IPAM,omitempty"`
	Containers map[string]*NetworkContainer `json:"Containers,omitempty"`
	Labels     map[string]string            `json:"Labels,omitempty"`
}

// BuildCache 构建缓存项（system df）。
type BuildCache struct {
	ID string `json:"ID,omitempty"`
}

// DiskUsage 是 GET /system/df 的响应。
type DiskUsage struct {
	LayersSize int64        `json:"LayersSize"`
	Containers []Container  `json:"Containers"`
	Images     []Image      `json:"Images"`
	Volumes    []Volume     `json:"Volumes"`
	BuildCache []BuildCache `json:"BuildCache,omitempty"`
}

// ExecInspect 是 GET /exec/{id}/json 的响应。
type ExecInspect struct {
	ID           string   `json:"ID"`
	ContainerID  string   `json:"ContainerID,omitempty"`
	Running      bool     `json:"Running,omitempty"`
	Entrypoint   string   `json:"Entrypoint,omitempty"`
	Cmd          []string `json:"Cmd,omitempty"`
	Tty          bool     `json:"Tty,omitempty"`
	AttachStdin  bool     `json:"AttachStdin,omitempty"`
	AttachStdout bool     `json:"AttachStdout,omitempty"`
	AttachStderr bool     `json:"AttachStderr,omitempty"`
}

// IDResponse 通用 ID 响应（create / exec create）。
type IDResponse struct {
	ID string `json:"Id"`
}

// Message 通用消息响应。
type Message struct {
	Message string `json:"message,omitempty"`
}

// Version 是 GET /version 的响应。
type Version struct {
	Platform struct {
		Name string `json:"Name"`
	} `json:"Platform"`
	Components []struct {
		Name    string `json:"Name"`
		Version string `json:"Version"`
		Details string `json:"Details,omitempty"`
	} `json:"Components,omitempty"`
	Version       string `json:"Version"`
	ApiVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
	KernelVersion string `json:"KernelVersion,omitempty"`
	Experimental  bool   `json:"Experimental"`
	BuildTime     string `json:"BuildTime,omitempty"`
}

// PruneReport 通用 prune 响应。
type PruneReport struct {
	ContainersDeleted []string `json:"ContainersDeleted,omitempty"`
	ImagesDeleted     []string `json:"ImagesDeleted,omitempty"`
	SpaceReclaimed    int64    `json:"SpaceReclaimed"`
}
