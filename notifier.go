package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
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

func setStateChange(monitorState *monitorState, logContext *log.Entry) {
	if monitorState.isActive {
		state.Status = activeStatus
	} else {
		state.Status = inactiveStatus
	}

	state.ActiveServices = monitorState.endpoints

	logContext.WithFields(log.Fields{
		"status": state.Status,
	}).Debug("State changed.")
}

func notifyStateChange(url string, logContext *log.Entry) error {
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

			logContext.Debug("Notification result ", resp.Status)

			if err == nil {
				return nil
			}
		}

		time.Sleep(retryInterval)
	}

	return err
}
