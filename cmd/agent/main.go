package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/Dinuka-Nonis/mini-orchestrator/internal/agent"
)

func main() {
	nodeID := flag.String("id", "node-a", "unique node ID")
	controlAddr := flag.String("control", "http://localhost:8080", "control plane address")
	flag.Parse()

	a, err := agent.New(*nodeID, *controlAddr)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Printf("[agent] starting, connecting to %s", *controlAddr)
	a.Run(ctx)
}