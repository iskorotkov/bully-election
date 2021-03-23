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
	tickInterval = time.Second
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

	commClient := comms.NewClient(logger.Named("client"))
	defer commClient.Close()

	sd, err := services.NewServiceDiscovery(services.Config{
		Namespace: namespace,

		// Timeouts.
		PingTimeout:       time.Second * 5,
		ElectionTimeout:   time.Second * 5,
		LeadershipTimeout: time.Second * 5,
		RefreshTimeout:    time.Second * 5,
		SelfInfoTimeout:   time.Second * 5,

		// Intervals.
		RefreshInterval:  time.Millisecond * 100,
		SelfInfoInverval: time.Millisecond * 100,

		Client: commClient,
		Logger: logger.Named("service-discovery"),
	})
	if err != nil {
		logger.Fatal("couldn't create service dicovery",
			zap.Error(err))
	}

	fsm := states.NewFSM(states.Config{
		WaitBeforeAutoElection: time.Second * 5,
		WaitForOtherElection:   time.Second * 5,
		ServiceDiscovery:       sd,
		Logger:                 logger.Named("fsm"),
	})

	commServer := comms.NewServer(logger.Named("comm-server"))
	defer commServer.Close()

	server := &http.Server{
		Addr: ":80",
	}

	go func() {
		metricsServer := metrics.NewServer(fsm, sd, logger.Named("metrics-server"))

		http.HandleFunc("/", commServer.Handle)
		http.HandleFunc("/metrics", metricsServer.Handle)

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
			fsm.OnElection(msg.From)
		case msg := <-commClient.OnResponse():
			fsm.OnAlive(msg.From)
		case msg := <-commServer.OnVictory():
			fsm.OnVictory(msg.From)
		default:
			err := fsm.Tick(tickInterval)
			if err != nil {
				logger.Error("error occurred during FSM tick",
					zap.Error(err))
			}

			time.Sleep(tickInterval)
		}
	}
}
