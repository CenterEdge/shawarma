package main

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Monitor struct {
	Config MonitorConfig

	mutex sync.Mutex

	stop          chan struct{}
	stopRequested bool

	state       monitorState
	stateChange chan monitorState
}

type MonitorConfig struct {
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
	isActive bool
	// List of endpoints known to be active, when empty this means we should deactivate the application
	endpoints []string
}

func (config *MonitorConfig) EnrichLogFields(fields log.Fields) log.Fields {
	fields["pod"] = config.PodName
	fields["ns"] = config.Namespace

	if len(config.ServiceName) > 0 {
		fields["svc"] = config.ServiceName
	}

	if len(config.ServiceLabelSelector) > 0 {
		fields["lbl"] = config.ServiceLabelSelector
	}

	return fields
}

func (config *MonitorConfig) ToLogFields() log.Fields {
	return config.EnrichLogFields(log.Fields{})
}

func NewMonitor(config MonitorConfig) Monitor {
	return Monitor{
		Config: config,
	}
}

func (monitor *Monitor) processEndpoint(endpoint *v1.Endpoints, isAddOrUpdate bool) {
	monitor.mutex.Lock()
	defer monitor.mutex.Unlock()

	foundPod := false

	if isAddOrUpdate {
		for _, subset := range endpoint.Subsets {
			for _, address := range subset.Addresses {
				if address.TargetRef != nil &&
					address.TargetRef.Kind == "Pod" &&
					address.TargetRef.Namespace == monitor.Config.Namespace &&
					address.TargetRef.Name == monitor.Config.PodName {
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
	for i := range monitor.state.endpoints {
		if monitor.state.endpoints[i] == endpoint.Name {
			endpointIndex = i
			break
		}
	}

	hasChangedEndpoints := false
	if foundPod && endpointIndex == -1 {
		monitor.state.endpoints = append(monitor.state.endpoints, endpoint.Name)
		hasChangedEndpoints = true
	} else if !foundPod && endpointIndex >= 0 {
		monitor.state.endpoints = append(monitor.state.endpoints[:endpointIndex], monitor.state.endpoints[endpointIndex+1:]...)
		hasChangedEndpoints = true
	}

	shouldBeActive := len(monitor.state.endpoints) > 0
	if shouldBeActive != monitor.state.isActive || hasChangedEndpoints {
		logContext := log.WithFields(monitor.Config.ToLogFields())

		if shouldBeActive != monitor.state.isActive {
			monitor.state.isActive = shouldBeActive

			if shouldBeActive {
				logContext.Info("Activated")
			} else {
				logContext.Info("Deactivated")
			}
		} else {
			logContext.Info("Endpoints changed")
		}

		monitor.state.isActive = shouldBeActive
		monitor.stateChange <- monitor.state
	}
}

func (monitor *Monitor) processStateChange(state monitorState) {
	logContext := log.WithFields(monitor.Config.ToLogFields())

	// Set new State
	setStateChange(&state, logContext)

	// Notify if is enabled
	if !monitor.Config.DisableStateNotifier {
		logContext.Debug("Posting state change notification...")
		err := notifyStateChange(monitor.Config.URL, logContext)
		if err != nil {
			logContext.Error(err)
		}
	}
}

func (monitor *Monitor) Start() error {
	var config *rest.Config
	var err error
	if monitor.Config.PathToConfig == "" {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
	} else {
		// creates from a kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", monitor.Config.PathToConfig)
	}
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// Subscribe to state changes
	monitor.stateChange = make(chan monitorState)
	go func() {
		for state := range debounce(100*time.Millisecond, monitor.stateChange) {
			monitor.processStateChange(state)
		}
	}()
	defer close(monitor.stateChange)

	monitor.stop = make(chan struct{})
	for monitor.stopRequested = false; !monitor.stopRequested; {
		watchList := cache.NewFilteredListWatchFromClient(
			clientset.CoreV1().RESTClient(),
			"endpoints",
			monitor.Config.Namespace,
			func(options *metav1.ListOptions) {
				if len(monitor.Config.ServiceName) > 0 {
					options.FieldSelector = "metadata.name=" + monitor.Config.ServiceName
				}

				options.LabelSelector = monitor.Config.ServiceLabelSelector
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
					monitor.processEndpoint(endpoint, true)
				},
				DeleteFunc: func(obj interface{}) {
					endpoint := obj.(*v1.Endpoints)

					log.Debugf("endpoint %s deleted", endpoint.Name)
					monitor.processEndpoint(endpoint, false)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					endpoint := newObj.(*v1.Endpoints)

					log.Debugf("endpoint %s changed", endpoint.Name)
					monitor.processEndpoint(endpoint, true)
				},
			},
		)

		log.Debug("Starting controller")
		controller.Run(monitor.stop)
		log.Debug("Controller exited")

		if !monitor.stopRequested {
			log.Warn("Fail out of controller.Run, restarting...")
		}
	}

	return nil
}

func (monitor *Monitor) Stop() {
	monitor.stopRequested = true
	close(monitor.stop)
}
