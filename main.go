package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	klog "k8s.io/klog"
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
	app.Copyright = "(c) 2019 CenterEdge Software"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "log-level, l",
			Usage:  "Set the log level (panic, fatal, error, warn, info, debug, trace)",
			Value:  "warn",
			EnvVar: "LOG_LEVEL",
		},
		cli.StringFlag{
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

	app.Commands = []cli.Command{
		{
			Name:    "monitor",
			Aliases: []string{"m"},
			Usage:   "Monitor a Kubernetes service",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "service, svc",
					Usage:  "Kubernetes service to monitor for this pod",
					EnvVar: "SHAWARMA_SERVICE",
				},
				cli.StringFlag{
					Name:   "pod, p",
					Usage:  "Kubernetes pod to monitor",
					EnvVar: "MY_POD_NAME",
				},
				cli.StringFlag{
					Name:   "namespace, n",
					Value:  "default",
					Usage:  "Kubernetes namespace to monitor",
					EnvVar: "MY_POD_NAMESPACE",
				},
				cli.StringFlag{
					Name:   "url, u",
					Value:  "http://localhost/applicationstate",
					Usage:  "URL which receives a POST on state change",
					EnvVar: "SHAWARMA_URL",
				},
				cli.BoolFlag{
					Name:   "disable-notifier, d",
					Usage:  "Enable/Disable state change notification",
					EnvVar: "SHAWARMA_DISABLE_STATE_NOTIFIER",
				},
				cli.IntFlag{
					Name:   "listen-port, l",
					Value:  8099,
					Usage:  "Default port to be used to start the http server",
					EnvVar: "SHAWARMA_LISTEN_PORT",
				},
			},
			Action: func(c *cli.Context) error {
				info := monitorInfo{
					Namespace:            c.String("namespace"),
					PodName:              c.String("pod"),
					ServiceName:          c.String("service"),
					URL:                  c.String("url"),
					DisableStateNotifier: c.Bool("disable-notifier"),
					PathToConfig:         c.GlobalString("kubeconfig"),
				}

				// In case of empty environment variable, pull default here too
				if info.URL == "" {
					info.URL = "http://localhost/applicationstate"
				}

				// Start server in a Go routine thread
				go httpServer(c.String("listen-port"))

				return monitorService(&info)
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}
