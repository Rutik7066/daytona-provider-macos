// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"io"

	"github.com/daytonaio/daytona/pkg/models"
	"github.com/docker/docker/api/types/container"
)

func (d *DockerClient) StopTarget(target *models.Target, logWriter io.Writer) error {
	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	err = d.ExecuteCommand("shutdown -h now", logWriter, sshClient)
	if err != nil {
		return err
	}

	containerName := d.GetTargetContainerName(target)

	err = d.apiClient.ContainerStop(context.Background(), containerName, container.StopOptions{})
	if err != nil {
		return err
	}

	return nil
}
