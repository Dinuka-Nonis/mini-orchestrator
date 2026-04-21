package reconciler

import (
	"context"
	"log"
	"time"

	"github.com/Dinuka-Nonis/mini-orchestrator/internal/store"
)

type Reconciler struct {
	store *store.Store
}

func New(s *store.Store) *Reconciler {
	return &Reconciler{store: s}
}

func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.reconcile()
		case <-ctx.Done():
			return
		}
	}
}

func (r *Reconciler) reconcile() {
	r.handleLostNodes()
	r.requeueFailedPods()
}

func (r *Reconciler) handleLostNodes() {
	nodes, err := r.store.ListNodes()
	if err != nil {
		return
	}
	for _, node := range nodes {
		if node.Status == "lost" {
			continue
		}
		if time.Since(node.LastSeen) > 10*time.Second {
			log.Printf("[reconciler] node %s lost (last seen %s ago)",
				node.ID, time.Since(node.LastSeen).Round(time.Second))
			r.store.UpdateNodeStatus(node.ID, "lost")
			pods, _ := r.store.ListPodsForNode(node.ID)
			count := 0
			for _, pod := range pods {
				if pod.Status == "running" || pod.Status == "scheduled" {
					r.store.ResetPod(pod.ID)
					count++
				}
			}
			if count > 0 {
				log.Printf("[reconciler] requeued %d pods from lost node %s", count, node.ID)
			}
		} else if node.Status == "lost" {
			// node came back
			r.store.UpdateNodeStatus(node.ID, "ready")
			log.Printf("[reconciler] node %s is back", node.ID)
		}
	}
}

func (r *Reconciler) requeueFailedPods() {
	pods, err := r.store.ListPodsByStatus("failed")
	if err != nil {
		return
	}
	for _, pod := range pods {
		r.store.ResetPod(pod.ID)
		log.Printf("[reconciler] requeued failed pod %s", pod.ID[:8])
	}
}