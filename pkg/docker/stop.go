// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"

	"github.com/daytonaio/daytona/pkg/models"
)

func (d *DockerClient) StopTarget(target *models.Target, logWriter io.Writer) error {
	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}
	return d.ExecuteCommand("shutdown -h now", logWriter, sshClient)
}
