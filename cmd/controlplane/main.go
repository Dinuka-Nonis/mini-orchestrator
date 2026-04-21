package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/Dinuka-Nonis/mini-orchestrator/internal/api"
	"github.com/Dinuka-Nonis/mini-orchestrator/internal/scheduler"
	"github.com/Dinuka-Nonis/mini-orchestrator/internal/store"
)

func main() {
	db := store.New("./state.db")
	sched := scheduler.New(db)
	router := api.New(db)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go sched.Run(ctx)
	log.Println("[controlplane] scheduler started")

	srv := &http.Server{Addr: ":8080", Handler: router}
	go srv.ListenAndServe()
	log.Println("[controlplane] api listening on :8080")

	<-ctx.Done()
	srv.Shutdown(context.Background())
}