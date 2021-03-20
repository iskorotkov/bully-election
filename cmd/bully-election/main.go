package main

import (
	"log"
	"os"
	"time"

	"github.com/iskorotkov/bully-election/pkg/network"
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

	client := network.NewClient(logger.Named("client"))

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
		Logger:           logger.Named("states"),
	}

	fsm := states.NewFSM(cfg)

	server := network.NewServer(":80", time.Second*3, logger.Named("server"))
	defer func() {
		if err := server.Shutdown(); err != nil {
			logger.Warn("service discovery close failed",
				zap.Error(err))
		}
	}()

	go func() {
		if err := server.ListenAndServe(); err != nil {
			logger.Warn("server stopped with error",
				zap.Error(err))
		}
	}()

	for {
		select {
		case msg := <-server.OnElection():
			fsm.OnElection(msg.Source)
		case msg := <-server.OnAlive():
			fsm.OnAlive(msg.Source)
		case msg := <-server.OnVictory():
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
