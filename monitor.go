package main

import (
	"reflect"
	"slices"
	"time"

	"go.uber.org/zap"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type Monitor struct {
	Config MonitorConfig
	Logger *zap.Logger

	cache *EndpointSliceCache

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
	serviceNames []types.NamespacedName
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
		cache:  NewEndpointSliceCache(),
	}
}

func (monitor *Monitor) processEndpointSlice(endpointSlice *discovery.EndpointSlice, remove bool) {
	changed := monitor.cache.Update(endpointSlice, remove)
	if !changed {
		// No change in the cache, nothing to do
		return
	}

	serviceNames := []types.NamespacedName{}

	for serviceName, endpoints := range monitor.cache.Services() {
		for endpoint := range endpoints {
			// Per spec, ready being nil means ready
			if (endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready) &&
				endpoint.TargetRef != nil {

				if endpoint.TargetRef.Kind == "Pod" &&
					endpoint.TargetRef.Namespace == monitor.Config.Namespace &&
					endpoint.TargetRef.Name == monitor.Config.PodName {

					serviceNames = append(serviceNames, serviceName)
					break
				}
			}
		}
	}

	// Sort service names to have a consistent order
	slices.SortFunc(serviceNames, func(a, b types.NamespacedName) int {
		if a.Namespace < b.Namespace {
			return -1
		} else if a.Namespace > b.Namespace {
			return 1
		} else if a.Name < b.Name {
			return -1
		} else if a.Name > b.Name {
			return 1
		} else {
			return 0
		}
	})

	if reflect.DeepEqual(serviceNames, monitor.state.serviceNames) {
		// No change in the list of services, nothing to do
		return
	}

	shouldBeActive := len(serviceNames) > 0

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

	monitor.state.serviceNames = serviceNames
	monitor.stateChange <- monitor.state
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
			clientset.DiscoveryV1().RESTClient(),
			"endpointslices",
			monitor.Config.Namespace,
			func(options *metav1.ListOptions) {
				labelSelector := monitor.Config.ServiceLabelSelector

				if len(monitor.Config.ServiceName) > 0 {
					if len(labelSelector) > 0 {
						labelSelector += ","
					}

					labelSelector += discovery.LabelServiceName + "=" + monitor.Config.ServiceName
				}

				options.LabelSelector = labelSelector
			},
		)

		_, controller := cache.NewInformerWithOptions(
			cache.InformerOptions{
				ListerWatcher: watchList,
				ObjectType:    &discovery.EndpointSlice{},
				ResyncPeriod:  time.Second * 0,
				Handler: cache.ResourceEventHandlerFuncs{
					AddFunc: func(obj interface{}) {
						endpointSlice := obj.(*discovery.EndpointSlice)

						monitor.Logger.Debug("endpointslice added",
							zap.String("endpoint", endpointSlice.Name))
						monitor.processEndpointSlice(endpointSlice, false)
					},
					DeleteFunc: func(obj interface{}) {
						endpointSlice := obj.(*discovery.EndpointSlice)

						monitor.Logger.Debug("endpointslice deleted",
							zap.String("endpoint", endpointSlice.Name))
						monitor.processEndpointSlice(endpointSlice, true)
					},
					UpdateFunc: func(oldObj, newObj interface{}) {
						endpointSlice := newObj.(*discovery.EndpointSlice)

						monitor.Logger.Debug("endpointslice changed",
							zap.String("endpoint", endpointSlice.Name))
						monitor.processEndpointSlice(endpointSlice, false)
					},
				},
			})

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
