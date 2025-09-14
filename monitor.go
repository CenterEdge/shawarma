package main

import (
	"sync"
	"time"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Monitor struct {
	Config MonitorConfig
	Logger *zap.Logger

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

func (config *MonitorConfig) CreateChildLogger(logger *zap.Logger) *zap.Logger {
	// Start with a length of 2, but allocated capacity of 3 to avoid reallocations
	// when we add a service name or labels
	fields := make([]zap.Field, 2, 3)
	fields[0] = zap.String("pod", config.PodName)
	fields[1] = zap.String("ns", config.Namespace)

	if len(config.ServiceName) > 0 {
		fields = append(fields, zap.String("svc", config.ServiceName))
	}
	if len(config.ServiceLabelSelector) > 0 {
		fields = append(fields, zap.String("lbl", config.ServiceLabelSelector))
	}

	return logger.With(fields...)
}

func NewMonitor(config MonitorConfig, logger *zap.Logger) Monitor {
	return Monitor{
		Config: config,
		Logger: logger,
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
		childLogger := monitor.Config.CreateChildLogger(monitor.Logger)

		if shouldBeActive != monitor.state.isActive {
			monitor.state.isActive = shouldBeActive

			if shouldBeActive {
				childLogger.Info("Activated")
			} else {
				childLogger.Info("Deactivated")
			}
		} else {
			childLogger.Info("Endpoints changed")
		}

		monitor.state.isActive = shouldBeActive
		monitor.stateChange <- monitor.state
	}
}

func (monitor *Monitor) processStateChange(state monitorState) {
	childLogger := monitor.Config.CreateChildLogger(monitor.Logger)

	// Set new State
	setStateChange(&state, childLogger)

	// Notify if is enabled
	if !monitor.Config.DisableStateNotifier {
		childLogger.Debug("Posting state change notification...")
		err := notifyStateChange(monitor.Config.URL, childLogger)
		if err != nil {
			childLogger.Error("Error processing state change",
				zap.Error(err))
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

					monitor.Logger.Debug("endpoint added",
						zap.String("endpoint", endpoint.Name))
					monitor.processEndpoint(endpoint, true)
				},
				DeleteFunc: func(obj interface{}) {
					endpoint := obj.(*v1.Endpoints)

					monitor.Logger.Debug("endpoint deleted",
						zap.String("endpoint", endpoint.Name))
					monitor.processEndpoint(endpoint, false)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					endpoint := newObj.(*v1.Endpoints)

					monitor.Logger.Debug("endpoint changed",
						zap.String("endpoint", endpoint.Name))
					monitor.processEndpoint(endpoint, true)
				},
			},
		)

		monitor.Logger.Debug("Starting controller")
		controller.Run(monitor.stop)
		monitor.Logger.Debug("Controller exited")

		if !monitor.stopRequested {
			monitor.Logger.Warn("Fail out of controller.Run, restarting...")
		}
	}

	return nil
}

func (monitor *Monitor) Stop() {
	monitor.stopRequested = true
	close(monitor.stop)
}
