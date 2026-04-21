package scheduler

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/Dinuka-Nonis/mini-orchestrator/internal/store"
	"github.com/Dinuka-Nonis/mini-orchestrator/types"
)

type Scheduler struct {
	store *store.Store
}

func New(s *store.Store) *Scheduler {
	return &Scheduler{store: s}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.schedulePending()
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) schedulePending() {
	pods, err := s.store.ListPodsByStatus(types.PodPending)
	if err != nil || len(pods) == 0 {
		return
	}
	nodes, err := s.store.ListReadyNodes()
	if err != nil || len(nodes) == 0 {
		log.Printf("[scheduler] no ready nodes available")
		return
	}
	for _, pod := range pods {
		node := s.bestFit(pod, nodes)
		if node == nil {
			log.Printf("[scheduler] no capacity for pod %s (cpu:%d mem:%d)",
				pod.ID[:8], pod.CPU, pod.Memory)
			continue
		}
		if err := s.store.AssignPod(pod.ID, node.ID); err != nil {
			log.Printf("[scheduler] assign failed: %v", err)
			continue
		}
		s.store.UpdateNodeUsage(node.ID, pod.CPU, pod.Memory)
		node.UsedCPU += pod.CPU
		node.UsedMem += pod.Memory
		log.Printf("[scheduler] assigned pod %s → node %s", pod.ID[:8], node.ID)
	}
}

// bestFit picks the node with least remaining capacity that still fits the pod
// this is the bin-packing heuristic — pack tightly, leave big gaps for big pods
func (s *Scheduler) bestFit(pod types.Pod, nodes []*types.Node) *types.Node {
	var best *types.Node
	bestRemain := math.MaxInt

	for _, n := range nodes {
		freeCPU := n.TotalCPU - n.UsedCPU
		freeMem := n.TotalMem - n.UsedMem
		if freeCPU < pod.CPU || freeMem < pod.Memory {
			continue
		}
		remain := freeCPU + freeMem
		if remain < bestRemain {
			best = n
			bestRemain = remain
		}
	}
	return best
}