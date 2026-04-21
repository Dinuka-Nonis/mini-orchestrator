package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Dinuka-Nonis/mini-orchestrator/types"
)

type Agent struct {
	nodeID      string
	controlAddr string
	docker      *http.Client
	httpClient  *http.Client
	starting    map[string]bool
	mu          sync.Mutex
}

func New(nodeID, controlAddr string) (*Agent, error) {
	dockerClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
			},
		},
		Timeout: 60 * time.Second,
	}
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
		starting:    make(map[string]bool),
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
			// skip if already being started in a goroutine
			a.mu.Lock()
			if a.starting[pod.ID] {
				a.mu.Unlock()
				continue
			}
			a.starting[pod.ID] = true
			a.mu.Unlock()
			go a.startPod(pod)
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
	defer func() {
		a.mu.Lock()
		delete(a.starting, pod.ID)
		a.mu.Unlock()
	}()

	log.Printf("[agent] pulling image %s...", pod.Image)
	pullURL := "/v1.41/images/create?fromImage=" + pod.Image + "&tag=latest"
	resp, err := a.dockerPost(pullURL, "")
	if err != nil {
		log.Printf("[agent] pull failed: %v", err)
		a.patchStatus(pod.ID, types.PodFailed)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	log.Printf("[agent] pull done for %s", pod.Image)

	createBody := fmt.Sprintf(`{
		"Image": "%s:latest",
		"Labels": {"orchestrator.pod.id": "%s"},
		"HostConfig": {
			"NanoCpus": %d,
			"Memory": %d
		}
	}`, pod.Image, pod.ID, int64(pod.CPU)*1_000_000, int64(pod.Memory)*1024*1024)

	resp, err = a.dockerPost("/v1.41/containers/create?name=pod-"+pod.ID, createBody)
	if err != nil {
		log.Printf("[agent] create HTTP error: %v", err)
		a.patchStatus(pod.ID, types.PodFailed)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode == 409 {
		// container already exists and is running — just report it as running
		existingID := a.findContainerID(pod.ID)
		if existingID != "" {
			log.Printf("[agent] container already exists for pod %s, marking running", pod.ID[:8])
			a.patchStarted(pod.ID, existingID)
		}
		return
	}

	if resp.StatusCode > 201 {
		log.Printf("[agent] create failed (status %d): %+v", resp.StatusCode, result)
		a.patchStatus(pod.ID, types.PodFailed)
		return
	}

	containerID, _ := result["Id"].(string)

	startResp, err := a.dockerPost("/v1.41/containers/"+containerID+"/start", "")
	if err != nil {
		log.Printf("[agent] start HTTP error: %v", err)
		a.patchStatus(pod.ID, types.PodFailed)
		return
	}
	startResp.Body.Close()

	log.Printf("[agent] started pod %s → container %s", pod.ID[:8], containerID[:12])
	a.patchStarted(pod.ID, containerID)
}

// findContainerID looks up an already-running container by pod label
func (a *Agent) findContainerID(podID string) string {
	resp, err := a.docker.Get("http://localhost/v1.41/containers/json?filters=" +
		`{"label":["orchestrator.pod.id=` + podID + `"]}`)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var containers []struct {
		ID string `json:"Id"`
	}
	json.NewDecoder(resp.Body).Decode(&containers)
	if len(containers) > 0 {
		return containers[0].ID
	}
	return ""
}

func (a *Agent) stopPod(pod types.Pod) {
	if pod.ContainerID == "" {
		return
	}
	resp, err := a.dockerPost("/v1.41/containers/"+pod.ContainerID+"/stop", "")
	if err == nil {
		resp.Body.Close()
	}
	log.Printf("[agent] stopped pod %s", pod.ID[:8])
}

func (a *Agent) heartbeat() {
	a.post("/nodes/"+a.nodeID+"/heartbeat", map[string]interface{}{
		"used_cpu": 0,
		"used_mem": 0,
	})
}

func (a *Agent) patchStatus(podID string, status types.PodStatus) {
	a.patch("/pods/"+podID+"/status", map[string]string{"status": string(status)})
}

func (a *Agent) patchStarted(podID, containerID string) {
	body := map[string]interface{}{
		"status":       string(types.PodRunning),
		"container_id": containerID,
	}
	b, _ := json.Marshal(body)
	log.Printf("[agent] patchStarted body: %s", string(b))
	a.patch("/pods/"+podID+"/status", body)
}

func (a *Agent) patch(path string, body interface{}) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("PATCH", a.controlAddr+path, bytes.NewReader(b))
	if err != nil {
		log.Printf("[agent] PATCH %s error: %v", path, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		log.Printf("[agent] PATCH %s error: %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		log.Printf("[agent] PATCH %s status %d: %v", path, resp.StatusCode, result)
	}
}

func (a *Agent) post(path string, body interface{}) {
	b, _ := json.Marshal(body)
	resp, err := a.httpClient.Post(a.controlAddr+path, "application/json", bytes.NewReader(b))
	if err != nil {
		log.Printf("[agent] POST %s error: %v", path, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		log.Printf("[agent] POST %s status %d: %v", path, resp.StatusCode, result)
	}
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