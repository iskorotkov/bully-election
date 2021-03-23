package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/iskorotkov/bully-election/pkg/services"
	"github.com/iskorotkov/bully-election/pkg/states"
	"go.uber.org/zap"
)

type MetricsServer struct {
	fsm    *states.FSM
	sd     *services.ServiceDiscovery
	logger *zap.Logger
}

func NewServer(fsm *states.FSM, sd *services.ServiceDiscovery, logger *zap.Logger) *MetricsServer {
	return &MetricsServer{
		fsm:    fsm,
		sd:     sd,
		logger: logger,
	}
}

func (m *MetricsServer) Handle(rw http.ResponseWriter, r *http.Request) {
	logger := m.logger.Named("handle")

	resp := struct {
		Name   string `json:"name"`
		Leader string `json:"leader,omitempty"`
		State  string `json:"state"`
	}{
		Name:   m.sd.Self().Name,
		Leader: m.sd.Leader().Name,
		State:  string(m.fsm.State().Role()),
	}

	b, err := json.Marshal(resp)
	if err != nil {
		msg := "couldn't marshal response to json"
		logger.Error(msg,
			zap.Any("response", msg),
			zap.Error(err))
		http.Error(rw, msg, http.StatusInternalServerError)
	}

	rw.Header().Add("Content-Type", "application/json")
	fmt.Fprint(rw, string(b))
}
