package main

import (
	"log"
	"time"

	"github.com/iskorotkov/bully-election/pkg/states"
	_ "go.uber.org/automaxprocs"
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

	var state states.State = &states.Starting{}
	for {
		var err error
		state, err = state.Tick(interval)
		if err != nil {
			log.Println(err)
		}

		time.Sleep(interval)
	}
}
