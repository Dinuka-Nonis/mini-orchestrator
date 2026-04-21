package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "github.com/Dinuka-Nonis/mini-orchestrator/internal/store"
    "github.com/Dinuka-Nonis/mini-orchestrator/internal/api"
)

func main() {
    db := store.New("./state.db")
    router := api.New(db)

    srv := &http.Server{Addr: ":8080", Handler: router}

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    go srv.ListenAndServe()
    <-ctx.Done()
    srv.Shutdown(context.Background())
}