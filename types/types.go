package types

import "time"

type PodStatus string
const (
    PodPending   PodStatus = "pending"
    PodScheduled PodStatus = "scheduled"
    PodRunning   PodStatus = "running"
    PodFailed    PodStatus = "failed"
    PodStopped   PodStatus = "stopped"
)

type Pod struct {
    ID          string    `json:"id"`
    Image       string    `json:"image"`
    CPU         int       `json:"cpu"`
    Memory      int       `json:"memory"`
    NodeID      string    `json:"node_id"`
    ContainerID string    `json:"container_id"`
    Status      PodStatus `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
}

type Node struct {
    ID        string    `json:"id"`
    Addr      string    `json:"addr"`
    TotalCPU  int       `json:"total_cpu"`
    TotalMem  int       `json:"total_mem"`
    UsedCPU   int       `json:"used_cpu"`
    UsedMem   int       `json:"used_mem"`
    LastSeen  time.Time `json:"last_seen"`
    Status    string    `json:"status"`
}