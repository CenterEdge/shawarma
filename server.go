package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

// Handlers
func deploymentState(w http.ResponseWriter, req *http.Request) {
	bytes, err := json.Marshal(&state)
	if err != nil {
		panic("Json encoding issue: " + err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(bytes); err != nil {
		panic("Write issue: " + err.Error())
	}
}

func _health(w http.ResponseWriter, req *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `{"health": "ok"}`)
}

// Http Server
func httpServer(port uint16, logger *zap.Logger) {

	// Endpoints Handlers
	http.HandleFunc("/deploymentstate", deploymentState)
	http.HandleFunc("/_health", _health)

	logger.Info("Starting HTTP Server",
		zap.Uint16("port", port))

	// Listener
	err := http.ListenAndServe(fmt.Sprintf("%s:%d", "localhost", port), nil)
	if err != nil {
		panic("Error: " + err.Error())
	}

}
