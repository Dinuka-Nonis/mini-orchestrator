package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Dinuka-Nonis/mini-orchestrator/types"
)

type Agent struct {
	nodeID      string
	controlAddr string
	docker      *http.Client
	httpClient  *http.Client
}

func New(nodeID, controlAddr string) (*Agent, error) {
	// talk to Docker via Unix socket directly — no SDK needed
	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
			},
		},
		Timeout: 30 * time.Second,
	}
	// verify Docker is reachable
	resp, err := dockerClient.Get("http://localhost/v1.41/info")
	if err != nil {
		return nil, fmt.Errorf("cannot reach Docker socket: %w", err)
	}
	resp.Body.Close()

	return &Agent{
		nodeID:      nodeID,
		controlAddr: controlAddr,
		docker:      dockerClient,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
	}, nil
}

func (a *Agent) Run(ctx context.Context) {
	a.register()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.sync()
			a.heartbeat()
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) register() {
	node := types.Node{
		ID:       a.nodeID,
		Addr:     a.controlAddr,
		TotalCPU: 4000,
		TotalMem: 8192,
		Status:   "ready",
	}
	a.post("/nodes/register", node)
	log.Printf("[agent] registered as %s", a.nodeID)
}

func (a *Agent) sync() {
	pods := a.fetchAssignedPods()
	for _, pod := range pods {
		switch pod.Status {
		case types.PodScheduled:
			a.startPod(pod)
		case types.PodStopped:
			a.stopPod(pod)
		}
	}
}

func (a *Agent) fetchAssignedPods() []types.Pod {
	resp, err := a.httpClient.Get(a.controlAddr + "/nodes/" + a.nodeID + "/pods")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var pods []types.Pod
	json.NewDecoder(resp.Body).Decode(&pods)
	return pods
}

func (a *Agent) startPod(pod types.Pod) {
	// pull image
	pullBody := fmt.Sprintf(`{"fromImage":"%s"}`, pod.Image)
	resp, err := a.dockerPost("/v1.41/images/create?fromImage="+pod.Image, pullBody)
	if err != nil {
		log.Printf("[agent] pull failed for %s: %v", pod.Image, err)
	} else {
		resp.Body.Close()
	}

	// create container
	createBody := fmt.Sprintf(`{
		"Image": "%s",
		"Labels": {"orchestrator.pod.id": "%s"},
		"HostConfig": {
			"NanoCpus": %d,
			"Memory": %d
		}
	}`, pod.Image, pod.ID, int64(pod.CPU)*1_000_000, int64(pod.Memory)*1024*1024)

	resp, err = a.dockerPost("/v1.41/containers/create?name=pod-"+pod.ID, createBody)
	if err != nil || resp.StatusCode > 201 {
		log.Printf("[agent] create failed for pod %s", pod.ID)
		a.patchStatus(pod.ID, types.PodFailed)
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	var created struct {
		ID string `json:"Id"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// start container
	resp, err = a.dockerPost("/v1.41/containers/"+created.ID+"/start", "")
	if err != nil || resp.StatusCode > 204 {
		log.Printf("[agent] start failed for pod %s", pod.ID)
		a.patchStatus(pod.ID, types.PodFailed)
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	resp.Body.Close()

	log.Printf("[agent] started pod %s → container %s", pod.ID, created.ID[:12])
	a.patchStarted(pod.ID, created.ID)
}

func (a *Agent) stopPod(pod types.Pod) {
	if pod.ContainerID == "" {
		return
	}
	resp, err := a.dockerPost("/v1.41/containers/"+pod.ContainerID+"/stop", "")
	if err == nil {
		resp.Body.Close()
	}
	log.Printf("[agent] stopped pod %s", pod.ID)
}

func (a *Agent) heartbeat() {
	a.post("/nodes/"+a.nodeID+"/heartbeat", map[string]interface{}{
		"used_cpu": 0,
		"used_mem": 0,
	})
}

func (a *Agent) patchStatus(podID string, status types.PodStatus) {
	a.post("/pods/"+podID+"/status", map[string]string{"status": string(status)})
}

func (a *Agent) patchStarted(podID, containerID string) {
	a.post("/pods/"+podID+"/status", map[string]interface{}{
		"status":       string(types.PodRunning),
		"container_id": containerID,
	})
}

func (a *Agent) post(path string, body interface{}) {
	b, _ := json.Marshal(body)
	a.httpClient.Post(a.controlAddr+path, "application/json", bytes.NewReader(b))
}

func (a *Agent) dockerPost(path, body string) (*http.Response, error) {
	req, err := http.NewRequest("POST", "http://localhost"+path,
		bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return a.docker.Do(req)
}