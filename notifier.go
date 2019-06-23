package main

import (
  "bytes"
  "encoding/json"
  "net/http"
  "time"

  log "github.com/sirupsen/logrus"
)

const (
  activeState = "active"
  inactiveState = "inactive"

  retryAttempts = 3
)

type stateChangeDto struct {
  State string `json:"state"`
}

var retryInterval, _ = time.ParseDuration("1s")

func notifyStateChange(info *monitorInfo, newState bool) error {
  var err error

  state := stateChangeDto{}

  if newState {
    state.State = activeState
  } else {
    state.State = inactiveState
  }

  body, err := json.Marshal(&state)
  if err != nil {
    return err
  }

  req, err := http.NewRequest(http.MethodPost, info.URL, bytes.NewBuffer(body))
  if err != nil {
    return err
  }

  for i := 0; i < retryAttempts; i++ {
    client := &http.Client{}
    resp, err := client.Do(req)
    defer resp.Body.Close()

    log.WithFields(log.Fields{
      "svc": info.ServiceName,
      "pod": info.PodName,
      "ns": info.Namespace,
    }).Debug("Notification result ", resp.Status)

    if err == nil {
      return nil
    }

    time.Sleep(retryInterval)
  }

  return err;
}
