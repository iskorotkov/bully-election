package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/iskorotkov/bully-election/pkg/comms"
	"github.com/iskorotkov/bully-election/pkg/metrics"
	"github.com/iskorotkov/bully-election/pkg/services"
	"github.com/iskorotkov/bully-election/pkg/states"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"
)

const (
	interval = time.Second
)

func main() {
	var (
		logger *zap.Logger
		err    error
	)
	if os.Getenv("DEVELOPMENT") != "" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		log.Fatalf("couldn't create logger: %v", err)
	}

	defer logger.Sync()

	defer func() {
		if p := recover(); p != nil {
			logger.Fatal("panic occurred",
				zap.Any("panic", p))
		}
	}()

	namespace := os.Getenv("KUBERNETES_NAMESPACE")
	if namespace == "" {
		logger.Fatal("kubernetes namespace wasn't set")
	}

	client := comms.NewClient(logger.Named("client"))

	sd, err := services.NewServiceDiscovery("app", namespace, time.Second*3,
		client, logger.Named("service-discovery"))
	if err != nil {
		logger.Fatal("couldn't create service dicovery",
			zap.Error(err))
	}

	cfg := states.Config{
		ElectionTimeout:  time.Second,
		VictoryTimeout:   time.Second,
		ServiceDiscovery: sd,
		Logger:           logger.Named("fsm"),
	}

	fsm := states.NewFSM(cfg)

	commServer := comms.NewServer(logger.Named("comm-server"))
	defer commServer.Close()
	http.HandleFunc("/", commServer.Handle)

	metricsServer := metrics.NewServer(fsm, sd, logger.Named("metrics-server"))
	http.HandleFunc("/metrics", metricsServer.Handle)

	server := &http.Server{
		Addr: ":80",
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("error in server occurred",
				zap.Error(err))
		}
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logger.Fatal("server shutdown failed",
				zap.Error(err))
		}
	}()

	for {
		select {
		case msg := <-commServer.OnElection():
			fsm.OnElection(msg.Source)
		case msg := <-commServer.OnAlive():
			fsm.OnAlive(msg.Source)
		case msg := <-commServer.OnVictory():
			fsm.OnVictory(msg.Source)
		default:
			err := fsm.Tick(interval)
			if err != nil {
				logger.Error("error occurred during FSM tick",
					zap.Error(err))
			}

			time.Sleep(interval)
		}
	}
}
