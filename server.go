package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// Handlers
func deploymentStatus(w http.ResponseWriter, req *http.Request) {
	st, err := json.Marshal(&state)
	if err != nil {
		panic("Json encoding issue: " + err.Error())
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, string(st))
}

func _health(w http.ResponseWriter, req *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `{"health": "ok"}`)
}

// Http Server
func httpServer(port string) {

	// Endpoints Handlers
	http.HandleFunc("/deploymentstatus", deploymentStatus)
	http.HandleFunc("/_health", _health)

	log.Info(fmt.Sprintf("Starting HTTP Server on port %s", port))

	// Listener
	err := http.ListenAndServe(fmt.Sprintf("%s:%s", "localhost", port), nil)
	if err != nil {
		panic("Error: " + err.Error())
	}

}
