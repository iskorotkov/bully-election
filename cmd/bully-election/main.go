package main

import (
	"log"
	"os"
	"time"

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

	sd := services.NewServiceDiscovery(time.Second*3, logger.Named("service-discovery"))
	cfg := states.Config{
		ElectionTimeout:  time.Second,
		VictoryTimeout:   time.Second,
		ServiceDiscovery: sd,
		Logger:           logger.Named("states"),
	}
	state := states.Start(cfg)

	for {
		var err error
		state, err = state.Tick(interval)
		if err != nil {
			log.Println(err)
		}

		time.Sleep(interval)
	}
}
