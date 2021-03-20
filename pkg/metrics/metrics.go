package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/iskorotkov/bully-election/pkg/services"
	"github.com/iskorotkov/bully-election/pkg/states"
	"go.uber.org/zap"
)

type MetricsServer struct {
	fsm    *states.FSM
	sd     *services.ServiceDiscovery
	logger *zap.Logger
}

func NewMetricsServer(fsm *states.FSM, sd *services.ServiceDiscovery, logger *zap.Logger) *MetricsServer {
	return &MetricsServer{
		fsm:    fsm,
		sd:     sd,
		logger: logger.Named("metrics-server"),
	}
}

func (m *MetricsServer) Handle(rw http.ResponseWriter, r *http.Request) {
	logger := m.logger.Named("handle")

	resp := struct {
		Name       string `json:"name"`
		LeaderName string `json:"leaderName"`
		State      string `json:"state"`
	}{
		Name:       m.sd.Self().Name,
		LeaderName: m.sd.Leader().Name,
		State:      reflect.TypeOf(m.fsm.State()).Name(),
	}

	b, err := json.Marshal(resp)
	if err != nil {
		msg := "couldn't marshal response to json"
		logger.Error(msg,
			zap.Any("response", msg),
			zap.Error(err))
		http.Error(rw, msg, http.StatusInternalServerError)
	}

	fmt.Fprint(rw, b)
}
