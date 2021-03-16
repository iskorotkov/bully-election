package main

import (
	"log"
	"time"
)

func main() {
	defer func() {
		if p := recover(); p != nil {
			log.Fatal(p)
		}
	}()

	if mustBeLeader() {
		becomeLeader()
	}

	for {
		ok := pingLeader()
		if !ok || electionFailed() {
			if mustBeLeader() {
				becomeLeader()
			} else {
				if leader := findNewLeader(); leader != nil {
					rememberLeader(leader)
					startElectionCountdown()
				} else {
					becomeLeader()
				}
			}
		}

		time.Sleep(time.Second)
	}
}

func pingLeader() bool {

}

func mustBeLeader() bool {

}

func ownID() string {

}

func allIDs() []string {

}

func becomeLeader() {

}

func findNewLeader() *Instance {

}

func rememberLeader(leader *Instance) {

}

func startElectionCountdown() {

}

func onElectionCompleted() {

}

func electionFailed() bool {

}
