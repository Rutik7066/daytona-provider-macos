// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/daytonaio/daytona/pkg/models"
	"github.com/docker/docker/api/types/container"
)

func (d *DockerClient) StartTarget(target *models.Target, logWriter io.Writer) error {
	logWriter.Write([]byte("Starting workspace container\n"))
	containerName := d.GetTargetContainerName(target)
	ctx := context.Background()

	c, err := d.apiClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("failed to inspect container when starting project: %w", err)
	}

	if !c.State.Running {
		err = d.apiClient.ContainerStart(ctx, containerName, container.StartOptions{})
		if err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}
	}

	d.OpenWebUI(d.targetOptions.RemoteHostname, logWriter)

	err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	err = d.ExecuteCommand("tmux new-session -s agent -d 'daytona agent --target'", logWriter, sshClient)
	if err != nil {
		logWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", "daytona agent --target", err.Error())))
	}

	return nil
}

func (d *DockerClient) StartWorkspace(workspace *models.Workspace, logWriter io.Writer) error {
	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	err = d.ExecuteCommand("tmux new-session -s agent -d 'daytona agent --target'", logWriter, sshClient)
	if err != nil {
		logWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", "daytona agent --target", err.Error())))
	}

	return nil
}
