package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// Tracks if the pod is currently active
var isActive = false

type monitorInfo struct {
	Namespace            string
	PodName              string
	ServiceName          string
	ServiceLabelSelector string
	URL                  string
	PathToConfig         string
	DisableStateNotifier bool
}

// Tracks the current state
type monitorState struct {
	info *monitorInfo

	// List of endpoints known to be active, when empty this means we should deactivate the application
	endpoints []types.UID
}

func (info *monitorInfo) EnrichLogFields(fields log.Fields) log.Fields {
	fields["pod"] = info.PodName
	fields["ns"] = info.Namespace

	if len(info.ServiceName) > 0 {
		fields["svc"] = info.ServiceName
	}

	if len(info.ServiceLabelSelector) > 0 {
		fields["lbl"] = info.ServiceLabelSelector
	}

	return fields
}

func (info *monitorInfo) ToLogFields() log.Fields {
	return info.EnrichLogFields(log.Fields{})
}

func processEndpoint(state *monitorState, endpoint *v1.Endpoints, isAddOrUpdate bool) {
	foundPod := false

	if isAddOrUpdate {
		for _, subset := range endpoint.Subsets {
			for _, address := range subset.Addresses {
				if address.TargetRef != nil &&
					address.TargetRef.Kind == "Pod" &&
					address.TargetRef.Namespace == state.info.Namespace &&
					address.TargetRef.Name == state.info.PodName {
					foundPod = true
					break
				}
			}

			if foundPod {
				break
			}
		}
	}

	endpointIndex := -1
	for i := range state.endpoints {
		if state.endpoints[i] == endpoint.UID {
			endpointIndex = i
			break
		}
	}

	if foundPod && endpointIndex == -1 {
		state.endpoints = append(state.endpoints, endpoint.UID)
	} else if !foundPod && endpointIndex >= 0 {
		state.endpoints = append(state.endpoints[:endpointIndex], state.endpoints[endpointIndex+1:]...)
	}

	shouldBeActive := len(state.endpoints) > 0
	if shouldBeActive != isActive {
		processStateChange(state.info, shouldBeActive)
	}
}

func processStateChange(info *monitorInfo, newState bool) {
	isActive = newState

	logContext := log.WithFields(info.ToLogFields())

	if newState {
		logContext.Info("Activated")
	} else {
		logContext.Info("Deactivated")
	}

	go func() {
		// Set new State
		setStateChange(newState, info)
		// Notify if is enabled
		if !info.DisableStateNotifier {
			logContext.Debug("Posting state change notification...")
			err := notifyStateChange(info)
			if err != nil {
				logContext.Error(err)
			}
		}
	}()
}

func monitorService(info *monitorInfo) error {
	var config *rest.Config
	var err error
	if info.PathToConfig == "" {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
	} else {
		// creates from a kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", info.PathToConfig)
	}
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// Maintain the state here for use in the callback below
	state := monitorState{
		info:      info,
		endpoints: []types.UID{},
	}

	for stopRequested := false; !stopRequested; {
		watchList := cache.NewFilteredListWatchFromClient(
			clientset.CoreV1().RESTClient(),
			"endpoints",
			info.Namespace,
			func(options *metav1.ListOptions) {
				if len(info.ServiceName) > 0 {
					options.FieldSelector = "metadata.name=" + info.ServiceName
				}

				options.LabelSelector = info.ServiceLabelSelector
			},
		)

		_, controller := cache.NewInformer(
			watchList,
			&v1.Endpoints{},
			time.Second*0,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					endpoint := obj.(*v1.Endpoints)

					log.Debugf("endpoint %s added", endpoint.Name)
					processEndpoint(&state, endpoint, true)
				},
				DeleteFunc: func(obj interface{}) {
					endpoint := obj.(*v1.Endpoints)

					log.Debugf("endpoint %s deleted", endpoint.Name)
					processEndpoint(&state, endpoint, false)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					endpoint := newObj.(*v1.Endpoints)

					log.Debugf("endpoint %s changed", endpoint.Name)
					processEndpoint(&state, endpoint, true)
				},
			},
		)

		stop := make(chan struct{})

		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-term // wait for SIGINT or SIGTERM
			log.Debug("Shutdown signal received")
			stopRequested = true
			close(stop) // trigger the stop channel
		}()

		log.Debug("Starting controller")
		controller.Run(stop)
		log.Debug("Controller exited")

		if !stopRequested {
			log.Warn("Fail out of controller.Run, restarting...")
		}
	}

	return nil
}
