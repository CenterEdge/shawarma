package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	activeStatus   = "active"
	inactiveStatus = "inactive"

	retryAttempts = 3
)

type stateChangeDto struct {
	Status         string   `json:"status"`
	ActiveServices []string `json:"activeServices"`
}

var retryInterval, _ = time.ParseDuration("1s")

var state = stateChangeDto{
	Status:         inactiveStatus,
	ActiveServices: []string{},
}

func setStateChange(monitorState *monitorState, logger *zap.Logger) {
	if monitorState.isActive {
		state.Status = activeStatus
	} else {
		state.Status = inactiveStatus
	}

	state.ActiveServices = make([]string, 0, len(monitorState.serviceNames))
	for _, serviceName := range monitorState.serviceNames {
		state.ActiveServices = append(state.ActiveServices, serviceName.Name)
	}

	logger.Debug("State changed.",
		zap.String("status", state.Status),
	)
}

func notifyStateChange(url string, logger *zap.Logger) error {
	var err error

	body, err := json.Marshal(&state)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		return err
	}

	for i := 0; i < retryAttempts; i++ {
		client := &http.Client{}
		resp, err := client.Do(req)
		if resp != nil {

			defer resp.Body.Close()

			logger.Debug("Notification result",
				zap.String("status", resp.Status),
			)

			if err == nil {
				return nil
			}
		}

		time.Sleep(retryInterval)
	}

	return err
}
