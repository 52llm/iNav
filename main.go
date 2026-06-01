package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/52llm/iNav/internal/api"
	"github.com/52llm/iNav/internal/config"
	"github.com/52llm/iNav/internal/llm"
	"github.com/52llm/iNav/internal/store"
	"github.com/52llm/iNav/internal/tagger"
	"github.com/52llm/iNav/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	staticFS, err := web.Dist()
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Background tagging worker.
	llmClient := llm.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	worker := tagger.New(st, llmClient)
	go worker.Run(ctx)

	handler := api.NewRouter(api.NewServer(st), cfg.Token, cfg.PublicRead, staticFS)
	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: handler}

	go func() {
		log.Printf("inav listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Println("inav stopped")
	_ = os.Stdout.Sync()
}
