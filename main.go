package main

import (
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-net/arp"
	"github.com/rancher/rancher-net/backend/ipsec"
	"github.com/rancher/rancher-net/server"
	"github.com/rancher/rancher-net/store"
)

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "log",
		},
		cli.StringFlag{
			Name: "pid-file",
		},
		cli.StringFlag{
			Name:  "file, f",
			Value: "config.json",
		},
		cli.StringFlag{
			Name:  "ipsec-config, c",
			Value: ".",
			Usage: "Configuration directory",
		},
		cli.StringFlag{
			Name: "charon-log",
		},
		cli.BoolFlag{
			Name: "debug",
		},
		cli.StringFlag{
			Name:  "listen",
			Value: ":8111",
		},
		cli.StringFlag{
			Name: "local-ip, i",
		},
	}
	app.Action = func(ctx *cli.Context) {
		if err := appMain(ctx); err != nil {
			logrus.Fatal(err)
		}
	}

	app.Run(os.Args)
}

func waitForFile(file string) string {
	for i := 0; i < 60; i++ {
		if _, err := os.Stat(file); err == nil {
			return file
		} else {
			logrus.Infof("Waiting for file %s", file)
			time.Sleep(1 * time.Second)
		}
	}
	logrus.Fatalf("Failed to find %s", file)
	return ""
}

func appMain(ctx *cli.Context) error {
	logFile := ctx.GlobalString("log")
	if logFile != "" {
		if output, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666); err != nil {
			logrus.Fatalf("Failed to log to file %s: %v", logFile, err)
		} else {
			logrus.SetOutput(output)
		}
	}

	pidFile := ctx.GlobalString("pid-file")
	if pidFile != "" {
		logrus.Infof("Writing pid %d to %s", os.Getpid(), pidFile)
		if err := ioutil.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
			logrus.Fatalf("Failed to write pid file %s: %v", pidFile, err)
		}
	}

	if ctx.GlobalBool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
	}

	db := store.NewSimpleStore(waitForFile(ctx.GlobalString("file")), ctx.GlobalString("local-ip"))
	overlay := ipsec.NewOverlay(ctx.GlobalString("ipsec-config"), db)
	overlay.Start(ctx.GlobalString("charon-log"))
	if err := overlay.Reload(); err != nil {
		return err
	}

	done := make(chan error)
	go func() {
		done <- arp.ListenAndServe(db, "eth0")
	}()

	go func() {
		s := server.Server{
			Backend: overlay,
		}
		done <- s.ListenAndServe(ctx.GlobalString("listen"))
	}()

	return <-done
}
