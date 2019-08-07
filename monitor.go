package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Tracks if the pod is currently active
var isActive = false

type monitorInfo struct {
	Namespace   string
	PodName     string
	ServiceName string
	URL         string
}

func processEndpoint(info *monitorInfo, endpoint *v1.Endpoints) {
	foundPod := false

	for _, subset := range endpoint.Subsets {
		for _, address := range subset.Addresses {
			if address.TargetRef != nil &&
				address.TargetRef.Kind == "Pod" &&
				address.TargetRef.Namespace == info.Namespace &&
				address.TargetRef.Name == info.PodName {
				foundPod = true
				break
			}
		}

		if foundPod {
			break
		}
	}

	if (foundPod && !isActive) || (!foundPod && isActive) {
		processStateChange(info, foundPod)
	}
}

func processStateChange(info *monitorInfo, newState bool) {
	isActive = newState

	logContext := log.WithFields(log.Fields{
		"svc": info.ServiceName,
		"pod": info.PodName,
		"ns":  info.Namespace,
	})

	if newState {
		logContext.Info("Activated")
	} else {
		logContext.Info("Deactivated")
	}

	go func() {
		err := notifyStateChange(info, newState)

		if err != nil {
			logContext.Error(err)
		}
	}()
}

func monitorService(info *monitorInfo) error {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	for stopRequested := false; !stopRequested; {
		watchList := cache.NewListWatchFromClient(
			clientset.CoreV1().RESTClient(),
			"endpoints",
			info.Namespace,
			fields.SelectorFromSet(fields.Set{
				"metadata.name": info.ServiceName,
			}),
		)

		_, controller := cache.NewInformer(
			watchList,
			&v1.Endpoints{},
			time.Second*0,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					endpoint := obj.(*v1.Endpoints)

					log.Debugf("endpoint %s added", endpoint.Name)
					processEndpoint(info, endpoint)
				},
				DeleteFunc: func(obj interface{}) {
					endpoint := obj.(*v1.Endpoints)

					log.Debugf("endpoint %s deleted\n", endpoint.Name)

					if isActive {
						processStateChange(info, false)
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					endpoint := newObj.(*v1.Endpoints)

					log.Debugf("endpoint %s changed\n", endpoint.Name)
					processEndpoint(info, endpoint)
				},
			},
		)

		stop := make(chan struct{})

		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-term // wait for SIGINT or SIGTERM
			stopRequested = true
			stop <- struct{}{} // trigger the stop channel
		}()

		controller.Run(stop)

		if !stopRequested {
			log.Warn("Fail out of controller.Run, restarting...")
		}
	}

	return nil
}
