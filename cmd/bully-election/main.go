package main

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/iskorotkov/bully-election/pkg/comms"
	"github.com/iskorotkov/bully-election/pkg/metrics"
	"github.com/iskorotkov/bully-election/pkg/services"
	"github.com/iskorotkov/bully-election/pkg/states"
	_ "go.uber.org/automaxprocs"
	"go.uber.org/zap"
)

type config struct {
	Development         bool   `env:"DEVELOPMENT" envDefault:"false"`
	KubernetesNamespace string `env:"KUBERNETES_NAMESPACE,required"`

	// Timeouts.
	PingTimeout       time.Duration `env:"PING_TIMEOUT" envDefault:"5s"`
	ElectionTimeout   time.Duration `env:"ELECTION_TIMEOUT" envDefault:"5s"`
	LeadershipTimeout time.Duration `env:"LEADERSHIP_TIMEOUT" envDefault:"5s"`
	RefreshTimeout    time.Duration `env:"REFRESH_TIMEOUT" envDefault:"5s"`
	SelfInfoTimeout   time.Duration `env:"SELF_INFO_TIMEOUT" envDefault:"5s"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"5s"`

	// Intervals.
	RefreshInterval  time.Duration `env:"REFRESH_INTERVAL" envDefault:"5s"`
	SelfInfoInverval time.Duration `env:"SELF_INFO_INTERVAL" envDefault:"5s"`
	TickInterval     time.Duration `env:"TICK_INTERVAL" envDefault:"5s"`

	// FSM.
	WaitBeforeAutoElection time.Duration `env:"WAIT_BEFORE_AUTO_ELECTION" envDefault:"5s"`
	WaitForOtherElection   time.Duration `env:"WAIT_FOR_OTHER_ELECTION" envDefault:"5s"`
	WaitForLeaderResponse  time.Duration `env:"WAIT_FOR_LEADER_RESPONSE" envDefault:"5s"`
	WaitBeforeNextPing     time.Duration `env:"WAIT_BEFORE_NEXT_PING" envDefault:"5s"`
}

func main() {
	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("couldn't parse env vars: %v", err)
	}

	log.Printf("using the configuration: %+v", cfg)

	var (
		logger *zap.Logger
		err    error
	)
	if cfg.Development {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		log.Fatalf("couldn't create logger: %v", err)
	}

	defer func(logger *zap.Logger) {
		err := logger.Sync()
		if err != nil {
			log.Fatalf("couldn't sync logger: %v", err)
		}
	}(logger)

	defer func() {
		if p := recover(); p != nil {
			logger.Fatal("panic occurred",
				zap.Any("panic", p))
		}
	}()

	commClient := comms.NewClient(logger.Named("client"))
	defer commClient.Close()

	sd, err := services.NewServiceDiscovery(services.Config{
		Namespace: cfg.KubernetesNamespace,

		// Timeouts.
		PingTimeout:       cfg.PingTimeout,
		ElectionTimeout:   cfg.ElectionTimeout,
		LeadershipTimeout: cfg.LeadershipTimeout,
		RefreshTimeout:    cfg.RefreshTimeout,
		SelfInfoTimeout:   cfg.SelfInfoTimeout,

		// Intervals.
		RefreshInterval:  cfg.RefreshInterval,
		SelfInfoInverval: cfg.SelfInfoInverval,

		Client: commClient,
		Logger: logger.Named("service-discovery"),
	})
	if err != nil {
		logger.Fatal("couldn't create service discovery",
			zap.Error(err))
	}

	fsm := states.NewFSM(states.Config{
		WaitBeforeAutoElection: cfg.WaitBeforeAutoElection,
		WaitForOtherElection:   cfg.WaitForOtherElection,
		WaitForLeaderResponse:  cfg.WaitForLeaderResponse,
		WaitBeforeNextPing:     cfg.WaitBeforeNextPing,
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

	go func() {
		for {
			tickInterval := cfg.TickInterval

			fsm.Tick(tickInterval)
			time.Sleep(tickInterval)
		}
	}()

	go func() {
		for {
			select {
			case msg := <-commServer.OnElectionRequest():
				fsm.OnElectionMessage(msg.From)
			case msg := <-commServer.OnVictoryRequest():
				fsm.OnVictoryMessage(msg.From)
			case msg := <-commClient.OnAliveResponse():
				fsm.OnAliveResponse(msg.From)
			case msg := <-commClient.OnElectionResponse():
				fsm.OnElectionResponse(msg.From)
			}
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("server shutdown failed",
			zap.Error(err))
	}
}
