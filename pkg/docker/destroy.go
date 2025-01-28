// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/daytonaio/daytona/pkg/models"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func (d *DockerClient) DestroyTarget(target *models.Target, targetDir string, logWriter io.Writer) error {
	logWriter.Write([]byte("Destroying workspace container....\n"))
	ctx := context.Background()

	containerName := d.GetTargetContainerName(target)

	c, err := d.apiClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return err
	}

	if !c.State.Running {
		err := d.apiClient.ContainerStart(ctx, containerName, container.StartOptions{})
		if err != nil {
			return err
		}
	}

	err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("rm -rf %s", targetDir)
	err = d.ExecuteCommand(cmd, logWriter, sshClient)
	if err != nil {
		return err
	}

	err = d.apiClient.ContainerRemove(ctx, containerName, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: false,
	})

	if err != nil && !client.IsErrNotFound(err) {
		return err
	}

	return nil
}

func (d *DockerClient) DestroyWorkspace(workspace *models.Workspace, workspaceDir string, logWriter io.Writer) error {
	containerName := d.GetTargetContainerName(&workspace.Target)

	c, err := d.apiClient.ContainerInspect(context.TODO(), containerName)
	if err != nil {
		return err
	}
	if !c.State.Running {
		err = d.apiClient.ContainerStart(context.TODO(), containerName, container.StartOptions{})
		if err != nil {
			return err
		}
	}
	err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("rm -rf %s", workspaceDir)
	err = d.ExecuteCommand(cmd, logWriter, sshClient)
	if err != nil {
		return err
	}

	return nil
}
