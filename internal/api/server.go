package api

import (
	"net/http"
	"time"

	"github.com/Dinuka-Nonis/mini-orchestrator/internal/store"
	"github.com/Dinuka-Nonis/mini-orchestrator/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func New(s *store.Store) *gin.Engine {
	r := gin.Default()

	r.POST("/pods", func(c *gin.Context) {
		var pod types.Pod
		if err := c.BindJSON(&pod); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		pod.ID = uuid.NewString()
		pod.Status = types.PodPending
		pod.CreatedAt = time.Now()
		if err := s.CreatePod(&pod); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, pod)
	})

	r.GET("/pods", func(c *gin.Context) {
		pods, err := s.ListPods()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, pods)
	})

	r.DELETE("/pods/:id", func(c *gin.Context) {
		if err := s.UpdatePodStatus(c.Param("id"), types.PodStopped); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/nodes/register", func(c *gin.Context) {
    var node types.Node
    if err := c.BindJSON(&node); err != nil {
        c.JSON(400, gin.H{"error": err.Error()}); return
    }
    node.LastSeen = time.Now()
    s.UpsertNode(&node)
    c.JSON(200, gin.H{"ok": true})
	})

	r.POST("/nodes/:id/heartbeat", func(c *gin.Context) {
		s.TouchNode(c.Param("id"))
		c.JSON(200, gin.H{"ok": true})
	})

	r.GET("/nodes/:id/pods", func(c *gin.Context) {
		pods, _ := s.ListPodsForNode(c.Param("id"))
		c.JSON(200, pods)
	})

	r.PATCH("/pods/:id/status", func(c *gin.Context) {
		var body struct {
			Status      types.PodStatus `json:"status"`
			ContainerID string          `json:"container_id"`
		}
		c.BindJSON(&body)
		if body.Status == types.PodRunning && body.ContainerID != "" {
			s.UpdatePodRunning(c.Param("id"), body.ContainerID)
		} else {
			s.UpdatePodStatus(c.Param("id"), body.Status)
		}
		c.JSON(200, gin.H{"ok": true})
	})

	r.GET("/nodes", func(c *gin.Context) {
		nodes, err := s.ListNodes()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, nodes)
	})
	return r
}