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
	Status string `json:"status"`
}

var retryInterval, _ = time.ParseDuration("1s")

var state = stateChangeDto{Status: inactiveStatus}

func setStateChange(newStatus bool, info *monitorInfo) {
	if newStatus {
		state.Status = activeStatus
	} else {
		state.Status = inactiveStatus
	}

	log.WithFields(log.Fields{
		"svc": info.ServiceName,
		"pod": info.PodName,
		"ns":  info.Namespace,
	}).Debug("State status set to: ", state.Status)

}

func notifyStateChange(info *monitorInfo) error {
	var err error

	body, err := json.Marshal(&state)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, info.URL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		return err
	}

	for i := 0; i < retryAttempts; i++ {
		client := &http.Client{}
		resp, err := client.Do(req)
		if resp != nil {

			defer resp.Body.Close()

			log.WithFields(log.Fields{
				"svc": info.ServiceName,
				"pod": info.PodName,
				"ns":  info.Namespace,
			}).Debug("Notification result ", resp.Status)

			if err == nil {
				return nil
			}
		}

		time.Sleep(retryInterval)
	}

	return err
}
