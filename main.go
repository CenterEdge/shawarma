package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	klog "k8s.io/klog/v2"
)

// Set on build
var version string

func main() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{})

	// Ensure klog also outputs to logrus
	klog.SetOutput(log.StandardLogger().WriterLevel(log.WarnLevel))

	app := cli.NewApp()
	app.Name = "Shawarma"
	app.Usage = "Sidecar for monitoring a Kubernetes service and notifying the main application when it is live"
	app.Copyright = "(c) 2019-2022 CenterEdge Software"
	app.Version = version

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "log-level",
			Aliases: []string{"l"},
			Usage:   "Set the log level (panic, fatal, error, warn, info, debug, trace)",
			Value:   "warn",
			EnvVars: []string{"LOG_LEVEL"},
		},
		&cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to a kubeconfig file, if not running in-cluster",
		},
	}
	app.Before = func(c *cli.Context) error {
		// In case of empty environment variable, pull default here too
		levelString := c.String("log-level")
		if levelString == "" {
			levelString = "warn"
		}

		level, err := log.ParseLevel(levelString)
		if err != nil {
			return err
		}

		log.SetLevel(level)

		return nil
	}

	app.Commands = []*cli.Command{
		{
			Name:    "monitor",
			Aliases: []string{"m"},
			Usage:   "Monitor a Kubernetes service",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "service",
					Aliases: []string{"svc"},
					Usage:   "Kubernetes service to monitor for this pod",
					EnvVars: []string{"SHAWARMA_SERVICE"},
				},
				&cli.StringFlag{
					Name:    "service-labels",
					Usage:   "Kubernetes service labels to monitor for this pod, comma-delimited ex. \"label1=value1,label2=value2\"",
					EnvVars: []string{"SHAWARMA_SERVICE_LABELS"},
				},
				&cli.StringFlag{
					Name:    "pod",
					Aliases: []string{"p"},
					Usage:   "Kubernetes pod to monitor",
					EnvVars: []string{"MY_POD_NAME"},
				},
				&cli.StringFlag{
					Name:    "namespace",
					Aliases: []string{"n"},
					Value:   "default",
					Usage:   "Kubernetes namespace to monitor",
					EnvVars: []string{"MY_POD_NAMESPACE"},
				},
				&cli.StringFlag{
					Name:    "url",
					Aliases: []string{"u"},
					Value:   "http://localhost/applicationstate",
					Usage:   "URL which receives a POST on state change",
					EnvVars: []string{"SHAWARMA_URL"},
				},
				&cli.BoolFlag{
					Name:    "disable-notifier",
					Aliases: []string{"d"},
					Usage:   "Enable/Disable state change notification",
					EnvVars: []string{"SHAWARMA_DISABLE_STATE_NOTIFIER"},
				},
				&cli.IntFlag{
					Name:    "listen-port",
					Aliases: []string{"l"},
					Value:   8099,
					Usage:   "Default port to be used to start the http server",
					EnvVars: []string{"SHAWARMA_LISTEN_PORT"},
				},
			},
			Action: func(c *cli.Context) error {
				config := MonitorConfig{
					Namespace:            c.String("namespace"),
					PodName:              c.String("pod"),
					ServiceName:          c.String("service"),
					ServiceLabelSelector: c.String("service-labels"),
					URL:                  c.String("url"),
					DisableStateNotifier: c.Bool("disable-notifier"),
					PathToConfig:         c.String("kubeconfig"),
				}

				if config.ServiceName == "" && config.ServiceLabelSelector == "" {
					return cli.Exit("The service name or labels must be supplied", 1)
				}

				// In case of empty environment variable, pull default here too
				if config.URL == "" {
					config.URL = "http://localhost/applicationstate"
				}

				// Start server in a Go routine thread
				go httpServer(c.String("listen-port"))

				monitor := NewMonitor(config)

				term := make(chan os.Signal, 1)
				signal.Notify(term, syscall.SIGINT, syscall.SIGTERM)

				go func() {
					<-term // wait for SIGINT or SIGTERM
					log.Debug("Shutdown signal received")
					monitor.Stop()
				}()

				return monitor.Start()
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}
