package whatsapp

import (
	"context"
	"fmt"
	"github.com/fsouza/go-dockerclient"
	"log"
	"os"
	"strconv"
	"time"
)

type DockerManager struct {
	client *docker.Client
}

type ClientContainer struct {
	ID       string
	Host     string
	Port     int
	Endpoint string // http://host:port
}

func NewDockerManager() (*DockerManager, error) {
	c, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("erro ao inicializar docker client: %w", err)
	}
	return &DockerManager{client: c}, nil
}

func (dm *DockerManager) EnsureImage(ctx context.Context, image string) error {
	// tenta inspecionar; se não existir, puxa
	_, err := dm.client.InspectImage(image)
	if err == nil {
		return nil
	}
	log.Printf("[DockerManager] Pulling image %s ...", image)
	opts := docker.PullImageOptions{
		Repository:   image,
		OutputStream: os.Stdout,
	}
	// se tag estiver contida em image (e.g. repo:tag) PullImage funciona
	if err := dm.client.PullImage(opts, docker.AuthConfiguration{}); err != nil {
		return fmt.Errorf("erro ao puxar imagem %s: %w", image, err)
	}
	return nil
}

// StartContainer cria e inicia um container com porta aleatória no host
func (dm *DockerManager) StartContainer(ctx context.Context, image, namePrefix string, labels map[string]string, envs []string) (*ClientContainer, error) {
	// Gera uma porta livre no host
	hostPort, err := getFreePort()
	if err != nil {
		return nil, fmt.Errorf("erro ao escolher porta livre: %w", err)
	}

	name := fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano())
	internalPort := docker.Port("8080/tcp")

	// Mapeia porta interna do container para a externa
	portBindings := map[docker.Port][]docker.PortBinding{
		internalPort: {{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", hostPort)}},
	}

	// Garante que a imagem existe localmente
	if err := dm.EnsureImage(ctx, image); err != nil {
		return nil, err
	}

	container, err := dm.client.CreateContainer(docker.CreateContainerOptions{
		Name: name,
		Config: &docker.Config{
			Image:        image,
			ExposedPorts: map[docker.Port]struct{}{internalPort: {}},
			Env:          envs,
			Labels:       labels,
		},
		HostConfig: &docker.HostConfig{
			PortBindings: portBindings,
			AutoRemove:   true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao criar container: %w", err)
	}

	if err := dm.client.StartContainer(container.ID, nil); err != nil {
		return nil, fmt.Errorf("erro ao iniciar container: %w", err)
	}

	host := dm.getDockerHost()
	endpoint := fmt.Sprintf("http://%s:%d", host, hostPort)
	log.Printf("[Docker] ✅ Container %s iniciado (ID=%s) - Porta %d - Endpoint: %s", name, container.ID, hostPort, endpoint)

	return &ClientContainer{
		ID:       container.ID,
		Host:     host,
		Port:     hostPort,
		Endpoint: endpoint,
	}, nil
}

// FindContainerByLabel retorna container ativo (ou parado) com o label específico
func (dm *DockerManager) FindContainerByLabel(ctx context.Context, labelKey, labelValue string) (*ClientContainer, error) {
	containers, err := dm.client.ListContainers(docker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"label": {fmt.Sprintf("%s=%s", labelKey, labelValue)},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, nil
	}

	c := containers[0]
	if c.State != "running" {
		if err := dm.client.StartContainer(c.ID, nil); err != nil {
			return nil, fmt.Errorf("erro ao iniciar container existente %s: %w", c.ID, err)
		}
	}

	inspect, err := dm.client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: c.ID})
	if err != nil {
		return nil, err
	}

	host := dm.getDockerHost()
	port := 0
	if bindings, ok := inspect.NetworkSettings.Ports["8080/tcp"]; ok && len(bindings) > 0 {
		port, _ = strconv.Atoi(bindings[0].HostPort)
	}

	return &ClientContainer{
		ID:       c.ID,
		Host:     host,
		Port:     port,
		Endpoint: fmt.Sprintf("http://%s:%d", host, port),
	}, nil
}
func (dm *DockerManager) StopContainer(ctx context.Context, id string) error {
	timeout := 5
	err := dm.client.StopContainer(id, uint(timeout))
	if err != nil {
		log.Printf("[Docker] erro ao parar container %s: %v", id, err)
		return err
	}
	log.Printf("[Docker] Container %s parado", id)
	return nil
}

func (dm *DockerManager) RemoveContainer(ctx context.Context, id string) error {
	opts := docker.RemoveContainerOptions{ID: id, RemoveVolumes: true, Force: true}
	if err := dm.client.RemoveContainer(opts); err != nil {
		return err
	}
	return nil
}

func (dm *DockerManager) getDockerHost() string {
	// normalmente, se rodando localmente, use localhost; se remoto, configure DOCKER_BRIDGE_HOST
	if h := os.Getenv("DOCKER_BRIDGE_HOST"); h != "" {
		return h
	}
	return "52.23.179.22"
}
