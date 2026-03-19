package containerd

import (
	"context"
	"fmt"
	"os"

	tasktypes "github.com/containerd/containerd/api/types/task"
	containerdclient "github.com/containerd/containerd/v2/client"
	ctrdcontainers "github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	gocni "github.com/containerd/go-cni"
	ocispecs "github.com/opencontainers/runtime-spec/specs-go"
)

// WorkspaceService is the containerd service interface expected by the workspace package.
// It returns plain value types instead of raw containerd client types.
type WorkspaceService interface {
	PullImage(ctx context.Context, ref string, opts *PullImageOptions) (ImageInfo, error)
	GetImage(ctx context.Context, ref string) (ImageInfo, error)

	CreateContainer(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
	GetContainer(ctx context.Context, id string) (ContainerInfo, error)
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
	DeleteContainer(ctx context.Context, id string, opts *DeleteContainerOptions) error
	ListContainersByLabel(ctx context.Context, key, value string) ([]ContainerInfo, error)

	StartContainer(ctx context.Context, containerID string, opts *StartTaskOptions) error
	StopContainer(ctx context.Context, containerID string, opts *StopTaskOptions) error
	DeleteTask(ctx context.Context, containerID string, opts *DeleteTaskOptions) error
	ListTasks(ctx context.Context, opts *ListTasksOptions) ([]WorkspaceTaskInfo, error)

	SetupNetwork(ctx context.Context, req NetworkSetupRequest) (NetworkResult, error)
	RemoveNetwork(ctx context.Context, req NetworkSetupRequest) error

	CommitSnapshot(ctx context.Context, snapshotter, name, key string) error
	ListSnapshots(ctx context.Context, snapshotter string) ([]SnapshotInfo, error)
	PrepareSnapshot(ctx context.Context, snapshotter, key, parent string) error
	CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
	SnapshotMounts(ctx context.Context, snapshotter, key string) ([]MountInfo, error)
}

// WorkspaceAdapter wraps DefaultService to implement WorkspaceService using value types.
type WorkspaceAdapter struct {
	inner *DefaultService
}

// NewWorkspaceAdapter returns a WorkspaceAdapter wrapping the given DefaultService.
func NewWorkspaceAdapter(svc *DefaultService) *WorkspaceAdapter {
	return &WorkspaceAdapter{inner: svc}
}

func (a *WorkspaceAdapter) PullImage(ctx context.Context, ref string, opts *PullImageOptions) (ImageInfo, error) {
	img, err := a.inner.PullImage(ctx, ref, opts)
	if err != nil {
		return ImageInfo{}, err
	}
	target := img.Target()
	return ImageInfo{
		Name: img.Name(),
		ID:   target.Digest.String(),
	}, nil
}

func (a *WorkspaceAdapter) GetImage(ctx context.Context, ref string) (ImageInfo, error) {
	img, err := a.inner.GetImage(ctx, ref)
	if err != nil {
		return ImageInfo{}, err
	}
	target := img.Target()
	return ImageInfo{
		Name: img.Name(),
		ID:   target.Digest.String(),
	}, nil
}

func (a *WorkspaceAdapter) getContainerInfo(ctx context.Context, c containerdclient.Container) (ContainerInfo, error) {
	innerCtx := a.inner.withNamespace(ctx)
	info, err := c.Info(innerCtx)
	if err != nil {
		return ContainerInfo{}, err
	}
	runtimeName := ""
	if info.Runtime.Name != "" {
		runtimeName = info.Runtime.Name
	}
	return ContainerInfo{
		ID:          info.ID,
		Image:       info.Image,
		Labels:      info.Labels,
		Snapshotter: info.Snapshotter,
		SnapshotKey: info.SnapshotKey,
		Runtime:     RuntimeInfo{Name: runtimeName},
		CreatedAt:   info.CreatedAt,
		UpdatedAt:   info.UpdatedAt,
	}, nil
}

func (a *WorkspaceAdapter) GetContainer(ctx context.Context, id string) (ContainerInfo, error) {
	c, err := a.inner.GetContainer(ctx, id)
	if err != nil {
		return ContainerInfo{}, err
	}
	return a.getContainerInfo(ctx, c)
}

func (a *WorkspaceAdapter) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := a.inner.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		info, err := a.getContainerInfo(ctx, c)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (a *WorkspaceAdapter) ListContainersByLabel(ctx context.Context, key, value string) ([]ContainerInfo, error) {
	containers, err := a.inner.ListContainersByLabel(ctx, key, value)
	if err != nil {
		return nil, err
	}
	infos := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		info, err := a.getContainerInfo(ctx, c)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (a *WorkspaceAdapter) CreateContainer(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error) {
	c, err := a.inner.CreateContainer(ctx, req)
	if err != nil {
		return ContainerInfo{}, err
	}
	return a.getContainerInfo(ctx, c)
}

func (a *WorkspaceAdapter) DeleteContainer(ctx context.Context, id string, opts *DeleteContainerOptions) error {
	return a.inner.DeleteContainer(ctx, id, opts)
}

func (a *WorkspaceAdapter) StartContainer(ctx context.Context, containerID string, opts *StartTaskOptions) error {
	_, err := a.inner.StartTask(ctx, containerID, opts)
	return err
}

func (a *WorkspaceAdapter) StopContainer(ctx context.Context, containerID string, opts *StopTaskOptions) error {
	return a.inner.StopTask(ctx, containerID, opts)
}

func (a *WorkspaceAdapter) DeleteTask(ctx context.Context, containerID string, opts *DeleteTaskOptions) error {
	return a.inner.DeleteTask(ctx, containerID, opts)
}

func (a *WorkspaceAdapter) ListTasks(ctx context.Context, opts *ListTasksOptions) ([]WorkspaceTaskInfo, error) {
	tasks, err := a.inner.ListTasks(ctx, opts)
	if err != nil {
		return nil, err
	}
	result := make([]WorkspaceTaskInfo, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, WorkspaceTaskInfo{
			ContainerID: t.ContainerID,
			ID:          t.ID,
			PID:         t.PID,
			Status:      translateTaskStatus(t.Status),
			ExitCode:    t.ExitStatus,
		})
	}
	return result, nil
}

func translateTaskStatus(s tasktypes.Status) TaskStatus {
	switch s {
	case tasktypes.Status_CREATED:
		return TaskStatusCreated
	case tasktypes.Status_RUNNING:
		return TaskStatusRunning
	case tasktypes.Status_STOPPED:
		return TaskStatusStopped
	case tasktypes.Status_PAUSED:
		return TaskStatusPaused
	default:
		return TaskStatusUnknown
	}
}

func (a *WorkspaceAdapter) SetupNetwork(ctx context.Context, req NetworkSetupRequest) (NetworkResult, error) {
	task, err := a.inner.GetTask(ctx, req.ContainerID)
	if err != nil {
		return NetworkResult{}, fmt.Errorf("get task for network setup: %w", err)
	}
	cniBinDir := req.CNIBinDir
	if cniBinDir == "" {
		cniBinDir = defaultCNIBinDir
	}
	cniConfDir := req.CNIConfDir
	if cniConfDir == "" {
		cniConfDir = defaultCNIConfDir
	}
	ip, err := setupCNINetworkAndGetIP(ctx, task, req.ContainerID, cniBinDir, cniConfDir)
	if err != nil {
		return NetworkResult{}, err
	}
	return NetworkResult{IP: ip}, nil
}

func (a *WorkspaceAdapter) RemoveNetwork(ctx context.Context, req NetworkSetupRequest) error {
	cniBinDir := req.CNIBinDir
	if cniBinDir == "" {
		cniBinDir = defaultCNIBinDir
	}
	cniConfDir := req.CNIConfDir
	if cniConfDir == "" {
		cniConfDir = defaultCNIConfDir
	}
	task, err := a.inner.GetTask(ctx, req.ContainerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get task for network removal: %w", err)
	}
	return RemoveNetwork(ctx, task, req.ContainerID)
}

func (a *WorkspaceAdapter) CommitSnapshot(ctx context.Context, snapshotter, name, key string) error {
	return a.inner.CommitSnapshot(ctx, snapshotter, name, key)
}

func (a *WorkspaceAdapter) ListSnapshots(ctx context.Context, snapshotter string) ([]SnapshotInfo, error) {
	snaps, err := a.inner.ListSnapshots(ctx, snapshotter)
	if err != nil {
		return nil, err
	}
	result := make([]SnapshotInfo, 0, len(snaps))
	for _, s := range snaps {
		result = append(result, SnapshotInfo{
			Name:    s.Name,
			Parent:  s.Parent,
			Kind:    s.Kind.String(),
			Created: s.Created,
			Updated: s.Updated,
			Labels:  s.Labels,
		})
	}
	return result, nil
}

func (a *WorkspaceAdapter) PrepareSnapshot(ctx context.Context, snapshotter, key, parent string) error {
	return a.inner.PrepareSnapshot(ctx, snapshotter, key, parent)
}

func (a *WorkspaceAdapter) CreateContainerFromSnapshot(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error) {
	c, err := a.inner.CreateContainerFromSnapshot(ctx, req)
	if err != nil {
		return ContainerInfo{}, err
	}
	return a.getContainerInfo(ctx, c)
}

func (a *WorkspaceAdapter) SnapshotMounts(ctx context.Context, snapshotter, key string) ([]MountInfo, error) {
	mounts, err := a.inner.SnapshotMounts(ctx, snapshotter, key)
	if err != nil {
		return nil, err
	}
	result := make([]MountInfo, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, MountInfo{
			Type:    m.Type,
			Source:  m.Source,
			Options: m.Options,
		})
	}
	return result, nil
}

// setupCNINetworkAndGetIP sets up CNI networking and returns the assigned IP.
func setupCNINetworkAndGetIP(ctx context.Context, task containerdclient.Task, containerID, cniBinDir, cniConfDir string) (string, error) {
	if task == nil {
		return "", ErrInvalidArgument
	}
	if containerID == "" {
		containerID = task.ID()
	}
	pid := task.Pid()
	if pid == 0 {
		return "", fmt.Errorf("task pid not available for %s", containerID)
	}
	if _, err := os.Stat(cniConfDir); err != nil {
		return "", fmt.Errorf("cni config dir missing: %s: %w", cniConfDir, err)
	}
	if _, err := os.Stat(cniBinDir); err != nil {
		return "", fmt.Errorf("cni bin dir missing: %s: %w", cniBinDir, err)
	}
	netnsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
	if _, err := os.Stat(netnsPath); err != nil {
		return "", fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{cniBinDir}),
		gocni.WithPluginConfDir(cniConfDir),
	)
	if err != nil {
		return "", err
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return "", err
	}
	result, err := cni.Setup(ctx, containerID, netnsPath)
	if err == nil {
		return extractCNIResultIP(result), nil
	}
	if !isDuplicateAllocationError(err) {
		return "", err
	}
	if rmErr := cni.Remove(ctx, containerID, netnsPath); rmErr != nil {
		return "", rmErr
	}
	result, err = cni.Setup(ctx, containerID, netnsPath)
	if err != nil {
		return "", err
	}
	return extractCNIResultIP(result), nil
}

// containerSpecToSpecOpts converts a ContainerSpec to a list of oci.SpecOpts.
func containerSpecToSpecOpts(spec ContainerSpec) []oci.SpecOpts {
	var opts []oci.SpecOpts
	if len(spec.Cmd) > 0 {
		opts = append(opts, oci.WithProcessArgs(spec.Cmd...))
	}
	if len(spec.Env) > 0 {
		opts = append(opts, oci.WithEnv(spec.Env))
	}
	if spec.WorkDir != "" {
		opts = append(opts, oci.WithProcessCwd(spec.WorkDir))
	}
	if spec.User != "" {
		opts = append(opts, oci.WithUser(spec.User))
	}
	if len(spec.Mounts) > 0 {
		mounts := spec.Mounts // capture for closure
		opts = append(opts, func(_ context.Context, _ oci.Client, _ *ctrdcontainers.Container, s *ocispecs.Spec) error {
			for _, m := range mounts {
				s.Mounts = append(s.Mounts, ocispecs.Mount{
					Destination: m.Destination,
					Type:        m.Type,
					Source:      m.Source,
					Options:     m.Options,
				})
			}
			return nil
		})
	}
	return opts
}

// extractCNIResultIP extracts the first non-loopback IP from a CNI result.
func extractCNIResultIP(result *gocni.Result) string {
	if result == nil {
		return ""
	}
	for _, cfg := range result.Interfaces {
		for _, ipCfg := range cfg.IPConfigs {
			if ipCfg.IP != nil {
				ip := ipCfg.IP.String()
				if ip != "" && ip != "127.0.0.1" && ip != "::1" {
					return ip
				}
			}
		}
	}
	return ""
}
