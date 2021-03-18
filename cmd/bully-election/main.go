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
	defer func() {
		if p := recover(); p != nil {
			log.Fatal(p)
		}
	}()

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

	client := network.NewClient(time.Second*3, logger.Named("client"))
	defer func() {
		if err := client.Close(); err != nil {
			logger.Warn("service discovery close failed",
				zap.Error(err))
		}
	}()

	sd := services.NewServiceDiscovery(time.Second*3, client, logger.Named("service-discovery"))
	state := states.Start(states.Config{
		ElectionTimeout:  time.Second,
		VictoryTimeout:   time.Second,
		ServiceDiscovery: sd,
		Logger:           logger.Named("states"),
	})

	for {
		var err error

		select {
		case msg := <-client.OnElection():
			state = state.OnElection(msg.Source)
		case msg := <-client.OnAlive():
			state = state.OnAlive(msg.Source)
		case msg := <-client.OnVictory():
			state = state.OnVictory(msg.Source)
		default:
			state, err = state.Tick(interval)
			if err != nil {
				log.Println(err)
			}

			time.Sleep(interval)
		}
	}
}
