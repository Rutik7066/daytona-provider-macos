// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"io"
	"strings"

	"github.com/daytonaio/daytona/pkg/git"
	"github.com/daytonaio/daytona/pkg/models"
	"github.com/daytonaio/daytona/pkg/ssh"
	"github.com/docker/docker/api/types/container"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

func (d *DockerClient) CreateTarget(target *models.Target, targetDir string, logWriter io.Writer, sshClient *ssh.Client) error {
	err := d.initWorkspaceContainer(target, logWriter)
	if err != nil {
		return err
	}
	return err
}

func (d *DockerClient) CreateWorkspace(opts *CreateWorkspaceOptions) error {
	opts.LogWriter.Write([]byte("Cloning project repository\n"))
	ctx := context.Background()
	c, err := d.apiClient.ContainerInspect(ctx, d.GetTargetContainerName(&opts.Workspace.Target))
	if err != nil {
		return err
	}

	if !c.State.Running {
		err = d.apiClient.ContainerStart(ctx, d.GetTargetContainerName(&opts.Workspace.Target), container.StartOptions{})
		if err != nil {
			return err
		}
	}

	err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	var auth *http.BasicAuth
	if opts.Gpc != nil {
		auth = &http.BasicAuth{
			Username: opts.Gpc.Username,
			Password: opts.Gpc.Token,
		}
	}
	gitService := git.Service{
		WorkspaceDir: opts.WorkspaceDir,
	}

	cloneCmd := gitService.CloneRepositoryCmd(opts.Workspace.Repository, auth)
	cmd := strings.Join(cloneCmd, " ")
	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}
	err = d.ExecuteCommand(cmd, opts.LogWriter, sshClient)
	if err != nil {
		return err
	}
	opts.LogWriter.Write([]byte("Project cloned successfully\n"))
	return nil
}
