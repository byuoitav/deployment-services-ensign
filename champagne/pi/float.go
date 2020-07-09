package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v2"
)

const (
	FloatURL          = "http://sandbag.byu.edu:2001/float"
	EnvironmentFile   = "/etc/environment"
	DeploymentFile    = "/tmp/deployment.log"
	SaltMinionFile    = "/etc/salt/minion"
	SaltMinionIDFile  = "/etc/salt/minion_id"
	DockerComposeFile = "/tmp/docker-compose.yml"
)

var (
	ErrDeviceNotFound = errors.New("device not found in production or stage")

	FinalProgressMessages = []string{
		"honestly i'm not sure what it's doing but just give it a minute",
		"if you're having issues, please call 801-422-KENG",
		"dirty mike is finding the boys...",
	}
)

type dcompose struct {
	Services map[string]ms `yaml:"services,omitempty"`
}

type ms struct {
	Ports       []string           `yaml:"ports,omitempty"`
	Command     string             `yaml:"command,omitempty"`
	Environment []string           `yaml:"environment,omitempty"`
	NetworkMode string             `yaml:"network_mode,omitempty"`
	Restart     string             `yaml:"restart,omitempty"`
	TTY         string             `yaml:"tty,omitempty"`
	Logging     map[string]options `yaml:"logging,omitempty"`
	Image       string             `yaml:"image,omitempty"`
}

type options struct {
	MaxSize       string `yaml:"max-size,omitempty"`
	Mode          string `yaml:"mode,omitempty"`
	MaxBufferSize string `yaml:"max-buffer-size,omitempty"`
}

func float() error {
	log.Printf("Floating...")

	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, FloatURL, nil)
	if err != nil {
		return fmt.Errorf("failed to float: %w", err)
	}

	log.Printf("Making GET request to %s", FloatURL)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to float: %w", err)
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to float: %w", err)
	}

	log.Printf("Response %d:\n%s", resp.StatusCode, buf)

	switch resp.StatusCode {
	case http.StatusOK:
		// wait for deployment file and environment file
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()

		log.Printf("Waiting for deployment file...")

		if err := waitForFile(ctx, DeploymentFile, false); err != nil {
			return fmt.Errorf("deployment file never showed up: %s", err)
		}

		log.Printf("Waiting for environment file...")

		if err := waitForFile(ctx, EnvironmentFile, true); err != nil {
			return fmt.Errorf("environment file never showed up: %s", err)
		}

		return source(EnvironmentFile)
	case http.StatusForbidden:
		return fmt.Errorf("failed to float: %w", ErrDeviceNotFound)
	case http.StatusNotFound:
		return fmt.Errorf("failed to float: %s", buf)
	case http.StatusInternalServerError:
		return fmt.Errorf("failed to float: unkown error: %s", buf)
	default:
		return fmt.Errorf("failed to float: unkown status code %d: %s", resp.StatusCode, buf)
	}
}

func saltDeployment() error {
	log.Printf("Starting salt deployment")

	data.Lock()
	data.ProgressMessage = "creating salt minion file"
	data.ProgressPercent = 10

	log.Printf("%s", data.ProgressMessage)
	data.Unlock()

	minionFile, err := os.Create(SaltMinionFile)
	if err != nil {
		return fmt.Errorf("failed to create minion file: %w", err)
	}
	defer minionFile.Close()

	data.Lock()
	data.ProgressMessage = "writing salt minion file"
	data.ProgressPercent = 15

	log.Printf("%s", data.ProgressMessage)
	data.Unlock()

	// write master address
	str := fmt.Sprintf("master: %s\n", os.Getenv("SALT_MASTER_HOST"))
	n, err := minionFile.WriteString(str)
	switch {
	case err != nil:
		return fmt.Errorf("unable to write to minion file: %w", err)
	case n != len(str):
		return fmt.Errorf("unable to write to minion file: wrote %v/%v bytes", n, len(str))
	}

	// write master finger
	str = fmt.Sprintf("master_finger: %s\n", os.Getenv("SALT_MASTER_FINGER"))
	n, err = minionFile.WriteString(str)
	switch {
	case err != nil:
		return fmt.Errorf("unable to write to minion file: %w", err)
	case n != len(str):
		return fmt.Errorf("unable to write to minion file: wrote %v/%v bytes", n, len(str))
	}

	// write startup states
	str = fmt.Sprintf("startup_states: 'highstate'\n")
	n, err = minionFile.WriteString(str)
	switch {
	case err != nil:
		return fmt.Errorf("unable to write to minion file: %w", err)
	case n != len(str):
		return fmt.Errorf("unable to write to minion file: wrote %v/%v bytes", n, len(str))
	}

	data.Lock()
	data.ProgressMessage = "deleting salt minion id file"
	data.ProgressPercent = 30

	log.Printf("%s", data.ProgressMessage)
	data.Unlock()

	// write minion id file
	idFile, err := os.Create(SaltMinionIDFile)
	if err != nil {
		return fmt.Errorf("failed to remove salt minion file: %w", err)
	}
	defer idFile.Close()

	hn, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	n, err = idFile.WriteString(hn)
	switch {
	case err != nil:
		return fmt.Errorf("unable to write to minion id file: %w", err)
	case n != len(hn):
		return fmt.Errorf("unable to write to minion id file: wrote %v/%v bytes", n, len(hn))
	}

	data.Lock()
	data.ProgressMessage = "restarting salt minion"
	data.ProgressPercent = 35

	log.Printf("%s", data.ProgressMessage)
	data.Unlock()

	// restart salt minion
	cmd := exec.Command("systemctl", "restart", "salt-minion")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run %q: %w", "systemctl restart salt-minion", err)
	}

	// wait for deployment stuff to finish
	log.Printf("waiting for deployment to finish (5 minutes).\ncur time: %v", time.Now())

	if err := waitForFile(context.TODO(), DockerComposeFile, true); err != nil {
		return fmt.Errorf("docker compose file never showed up: %s", err)
	}

	dockerCompose, err := ioutil.ReadFile(DockerComposeFile)
	if err != nil {
		return fmt.Errorf("unable to read docker compose file: %s", err)
	}

	var dockers dcompose

	if err = yaml.Unmarshal(dockerCompose, &dockers); err != nil {
		return fmt.Errorf("unable to unmarshal docker-compose.yml: %s", err)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("unable to get docker client: %s", err)
	}

	// get a random final message
	rand.Seed(time.Now().UnixNano())
	idx := rand.Intn(len(FinalProgressMessages))

	timeout := 600
	done := false

	for {
		time.Sleep(7 * time.Second)
		timeout += 7
		data.Lock()

		runningDockers, err := cli.ContainerList(context.TODO(), types.ContainerListOptions{})
		if err != nil {
			return fmt.Errorf("unable to get list of running docker containers: %s", err)
		}

		switch {
		case len(runningDockers) < len(dockers.Services):
			data.ProgressMessage = fmt.Sprintf("downloaded %d/%d applications", len(runningDockers), len(dockers.Services))
		case len(runningDockers) == len(dockers.Services):
			done = true
			break
		default:
			data.ProgressMessage = FinalProgressMessages[idx]
		}

		data.ProgressPercent = int(100 * float32(len(runningDockers)) / float32(len(dockers.Services)))
		data.Unlock()
		if done {
			break
		}
	}

	// schedule a reboot (will shutdown in 1 minute)
	cmd = exec.Command("shutdown", "-r")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run %q: %w", "shutdown -r", err)
	}

	data.Lock()
	data.ProgressMessage = "finished! rebooting in 1 minute."
	data.ProgressPercent = 99
	log.Printf("%s", data.ProgressMessage)
	data.Unlock()

	// so that the ui can refresh
	time.Sleep(30 * time.Second)

	// disable myself (will kill the program!!!!)
	cmd = exec.Command("systemctl", "disable", "pi-setup.service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run %q: %w", "systemctl disable pi-setup.service", err)
	}

	return nil
}
