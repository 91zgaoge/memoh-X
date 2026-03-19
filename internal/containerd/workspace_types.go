package containerd

import (
	"errors"
	"time"
)

// ErrNotSupported is returned when an operation is not supported on the current backend.
var ErrNotSupported = errors.New("operation not supported on this backend")

// This file adds value-type structs used by the workspace package.
// The upstream codebase refactored the Service interface to return these
// plain types instead of raw containerd client types.

// TaskStatus represents the status of a container task.
type TaskStatus int

const (
	TaskStatusUnknown TaskStatus = iota
	TaskStatusCreated
	TaskStatusRunning
	TaskStatusStopped
	TaskStatusPaused
)

func (s TaskStatus) String() string {
	switch s {
	case TaskStatusCreated:
		return "CREATED"
	case TaskStatusRunning:
		return "RUNNING"
	case TaskStatusStopped:
		return "STOPPED"
	case TaskStatusPaused:
		return "PAUSED"
	default:
		return "UNKNOWN"
	}
}

// ImageInfo is a plain-value summary of a container image.
type ImageInfo struct {
	Name string
	ID   string
	Tags []string
}

// RuntimeInfo holds the runtime name for a container.
type RuntimeInfo struct {
	Name string
}

// ContainerInfo is a plain-value summary of a container.
type ContainerInfo struct {
	ID          string
	Image       string
	Labels      map[string]string
	Snapshotter string
	SnapshotKey string
	Runtime     RuntimeInfo
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkspaceTaskInfo is the value-type task info used by the workspace package.
// (Named to avoid collision with the existing TaskInfo which uses tasktypes.Status.)
type WorkspaceTaskInfo struct {
	ContainerID string
	ID          string
	PID         uint32
	Status      TaskStatus
	ExitCode    uint32
}

// NetworkSetupRequest describes a CNI network setup operation.
type NetworkSetupRequest struct {
	ContainerID string
	PID         uint32
	CNIBinDir   string
	CNIConfDir  string
}

// NetworkResult holds the result of a CNI network setup.
type NetworkResult struct {
	IP string
}

// SnapshotInfo is a plain-value snapshot summary.
type SnapshotInfo struct {
	Name    string
	Parent  string
	Kind    string
	Created time.Time
	Updated time.Time
	Labels  map[string]string
}

// MountInfo is a plain-value mount summary.
type MountInfo struct {
	Type    string
	Source  string
	Target  string
	Options []string
}

// MountSpec describes a container bind-mount.
type MountSpec struct {
	Destination string
	Type        string
	Source      string
	Options     []string
}

// ContainerSpec describes the process and mount configuration for a container.
type ContainerSpec struct {
	Cmd     []string
	Env     []string
	WorkDir string
	User    string
	Mounts  []MountSpec
	DNS     []string
	TTY     bool
}
