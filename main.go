package main

import (

	// "strings"

	// "encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-kit/log/level"
	// "github.com/go-resty/resty/v2"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// **************
const (
	// Constant values
	metricsPublishingPort = ":8082"
)

var (

	// debug_flag = kingpin.Flag("debug", "debug connection checks.").Short('d').Default("false").Bool()
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(metricsPublishingPort).String()
	configFile    = kingpin.Flag("config.file", "Exporter configuration file.").Short('c').Default("conf/check_centreon.yml").String()
)

// ***********************************************************************************************
func main() {
	logConfig := promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, &logConfig)
	kingpin.Version(version.Print("check_centreon")).VersionFlag.Short('V')
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(&logConfig)

	config, err := Load(*configFile, logger)
	if err != nil {
		level.Error(logger).Log("msg", fmt.Sprintf("Error reading config: %s", err))
		os.Exit(1)
	}

	level.Info(logger).Log("msg", "start test")

	http.Handle("/check", ChecksHandlerFor(config))
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "errmsg", err)
		os.Exit(1)
	}

	// target := &TargetConfig{
	// 	Name:   "test",
	// 	Scheme: "http",
	// 	// Host:   "c2lsupproxy01.c2.dav.fr",
	// 	Host: "dal-i-tech02.dassault-avion.inf",
	// 	Port: "9275",
	// 	// BaseUrl:   "/export",
	// 	BasicAuth: false,
	// 	ProxyUrl:  "",
	// 	VerifySSL: false,
	// }

	/*
		segment := "C2LegacyTEST"

		poller, ok := config.Pollers[segment]
		if !ok {
			level.Error(logger).Log("msg", fmt.Sprintf("can't find centreon nrpe_exporter: %s", segment))
			os.Exit(1)
		}
		client := newClient(&poller, logger)
		if client == nil {
			level.Error(logger).Log("errmsg", "invalid config")
			os.Exit(1)
		}
		target_host := "dal-i-tech02.dassault-avion.inf:5666"
		// to set into resty response
		//	timeout := 10
		command := "check_service"
		// service := "sshd.service"
		service := "nrpe_exporter.service"
		cmd_params := fmt.Sprintf("-s %s", service)

		params := make(map[string]string)
		params["ssl"] = "true"
		params["command"] = command
		params["result_message"] = "true"
		params["target"] = target_host
		params["params"] = cmd_params

		resp, data, err := client.Get("export", params, false)
		// ok
		if err != nil {
			level.Error(logger).Log("errmsg", err)
		} else if resp.StatusCode() != 200 {
			level.Error(logger).Log("errmsg", fmt.Sprintf("invalid result http code: %s", resp.Status()))
		} else if data != nil {
			if cmd, ok := data[command].(map[string]any); ok {
				cmd["service"] = service
			}
			res, _ := json.MarshalIndent(data, "", "   ")
			os.Stdout.Write(res)
		}
	*/

}
