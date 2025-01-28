// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/daytonaio/daytona/cmd/daytona/config"
	"github.com/daytonaio/daytona/pkg/models"
	"github.com/daytonaio/daytona/pkg/ports"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	log "github.com/sirupsen/logrus"
)

func (d *DockerClient) initWorkspaceContainer(target *models.Target, logWriter io.Writer) error {
	ctx := context.Background()
	mounts := []mount.Mount{}

	configPath, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("error getting config dir: %w", err)
	}

	winStorage := filepath.Join(configPath, "server", "local-runner", "providers", "macos-provider", "macos")
	err = os.MkdirAll(winStorage, 0755)
	if err != nil {
		return err
	}

	mounts = append(mounts, mount.Mount{
		Type:   mount.TypeBind,
		Source: winStorage,
		Target: "/storage",
	})

	var availablePort *uint16
	portBindings := make(map[nat.Port][]nat.PortBinding)
	portBindings["22/tcp"] = []nat.PortBinding{
		{
			HostIP:   "0.0.0.0",
			HostPort: "10022",
		},
	}
	portBindings["2222/tcp"] = []nat.PortBinding{
		{
			HostIP:   "0.0.0.0",
			HostPort: "2222",
		},
	}
	portBindings["8006/tcp"] = []nat.PortBinding{
		{
			HostIP:   "0.0.0.0",
			HostPort: "8006",
		},
	}

	if d.IsLocalMacTarget(target.TargetConfig.ProviderInfo.Name, target.TargetConfig.Options, target.TargetConfig.ProviderInfo.RunnerId) {
		p, err := ports.GetAvailableEphemeralPort()
		if err != nil {
			log.Error(err)
		} else {
			availablePort = &p
			portBindings["2280/tcp"] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", *availablePort),
				},
			}
		}
	}

	c, err := d.apiClient.ContainerCreate(ctx, GetContainerCreateConfig(target, availablePort), &container.HostConfig{
		Privileged: true,
		Mounts:     mounts,
		ExtraHosts: []string{
			"host.docker.internal:host-gateway",
		},
		PortBindings: portBindings,
		Resources: container.Resources{
			Devices: []container.DeviceMapping{
				{
					PathOnHost:      "/dev/kvm",
					PathInContainer: "/dev/kvm",
				},
				{
					PathOnHost:      "/dev/net/tun",
					PathInContainer: "/dev/net/tun",
				},
			},
		},
		CapAdd: []string{
			"NET_ADMIN",
			"SYS_ADMIN",
		},
	}, nil, nil, d.GetTargetContainerName(target))
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	err = d.apiClient.ContainerStart(ctx, c.ID, container.StartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	for {
		c, err := d.apiClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("failed to inspect container when creating project: %w", err)
		}

		if c.State.Running {
			break
		}

		time.Sleep(1 * time.Second)
	}

	logWriter.Write([]byte("Visit http://localhost:8006 and Set up MacOS \n"))
	logWriter.Write([]byte("Set USERNAME and PASSWORD  to daytona\n"))
	logWriter.Write([]byte("Please turn on Remote Login to continue.....\n"))
	time.Sleep(15 * time.Second)

	d.OpenWebUI(d.targetOptions.RemoteHostname, logWriter)

	err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
	if err != nil {
		return fmt.Errorf("failed to wait for mac to boot: %w", err)
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %w", err)
	}
	defer sshClient.Close()

	for key, env := range target.EnvVars {
		err = d.ExecuteCommand(fmt.Sprintf("echo 'export %s=\"%s\"' >> ~/.zshrc", key, env), nil, sshClient)
		if err != nil {
			logWriter.Write([]byte(fmt.Sprintf("failed to set env variable %s to %s: %s\n", key, env, err.Error())))
		}
	}

	// Setting env
	err = d.ExecuteCommand("source ~/.zshrc", nil, sshClient)
	if err != nil {
		logWriter.Write([]byte("failed to set env variable DAYTONA_AGENT_LOG_FILE_PATH to C:\\Users\\daytona\\.daytona-agent.log\n"))
	}

	// disable pass propmt for sudo
	noPropmt := `echo 'daytona' | sudo -S bash -c 'echo "daytona ALL=(ALL) NOPASSWD:ALL" | sudo EDITOR="tee -a" visudo'`
	err = d.ExecuteCommand(noPropmt, nil, sshClient)
	if err != nil {
		logWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", noPropmt, err.Error())))
	}

	// Installing git && allow port 2222 in firewall
	commands := []string{
		`NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" -y`,
		"echo >> /Users/daytona/.zprofile",
		`echo 'eval "$(/usr/local/bin/brew shellenv)"' >> /Users/daytona/.zprofile`,
		`eval "$(/usr/local/bin/brew shellenv)"`,
		"brew install git",
		"echo 'pass in proto tcp from any to any port 2222' | sudo tee -a /etc/pf.conf",
		"echo 'pass out proto tcp from any to any port 2222' | sudo tee -a /etc/pf.conf",
		"sudo pfctl -f /etc/pf.conf",
		"sudo pfctl -E",
	}

	for _, command := range commands {
		err = d.ExecuteCommand(command, logWriter, sshClient)
		if err != nil {
			logWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", command, err.Error())))
		}
	}

	//auto log in
	autoLogin := "sudo defaults write /Library/Preferences/com.apple.loginwindow autoLoginUser daytona"
	err = d.ExecuteCommand(autoLogin, nil, sshClient)
	if err != nil {
		logWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", autoLogin, err.Error())))
	}

	// install Daytona
	logWriter.Write([]byte("Installing Daytona Agent...\n"))
	err = d.ExecuteCommand("(curl -sf -L https://download.daytona.io/daytona/install.sh | sudo bash)", logWriter, sshClient)
	if err != nil {
		logWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", autoLogin, err.Error())))
	}

	return nil
}

func GetContainerCreateConfig(target *models.Target, toolboxApiHostPort *uint16) *container.Config {
	envVars := []string{
		fmt.Sprintf("ARGUMENTS=%s", "-device e1000,netdev=net0  -netdev user,id=net0,hostfwd=tcp::22-:22,hostfwd=tcp::2222-:2222 "),
		fmt.Sprintf("RAM_SIZE=%s", "4G"),
	}

	for key, value := range target.EnvVars {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	labels := map[string]string{
		"daytona.target.id":   target.Id,
		"daytona.target.name": target.Name + "-daytona-macos",
	}

	if toolboxApiHostPort != nil {
		labels["daytona.toolbox.api.hostPort"] = fmt.Sprintf("%d", *toolboxApiHostPort)
	}

	exposedPorts := nat.PortSet{}
	if toolboxApiHostPort != nil {
		exposedPorts["2280/tcp"] = struct{}{}
	}

	exposedPorts["22/tcp"] = struct{}{}
	exposedPorts["2222/tcp"] = struct{}{}

	return &container.Config{
		Hostname: target.Name,
		Image:    "dockurr/macos:latest",
		Labels:   labels,
		User:     "root",
		Entrypoint: []string{
			"/usr/bin/tini",
			"-s",
			"/run/entry.sh",
		},
		Env:          envVars,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: exposedPorts,
		StopTimeout:  &[]int{120}[0],
	}
}
