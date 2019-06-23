package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.JSONFormatter{})

	app := cli.NewApp()
	app.Name = "Blue/Green Monitor"
	app.Usage = "Sidecar for monitoring a Kubernetes service and notifying the main application when it is live"
	app.Copyright = "(c) 2019 CenterEdge Software"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "log-level, l",
			Usage:  "Set the log level (panic, fatal, error, warn, info, debug, trace)",
			Value:  "warn",
			EnvVar: "LOG_LEVEL",
		},
	}
	app.Before = func(c *cli.Context) error {
		level, err := log.ParseLevel(c.String("log-level"))
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
			},
			Action: func(c *cli.Context) error {
				info := monitorInfo{
					Namespace:   c.String("namespace"),
					PodName:     c.String("pod"),
					ServiceName: c.String("service"),
					URL:         c.String("url"),
				}

				return monitorService(&info)
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
