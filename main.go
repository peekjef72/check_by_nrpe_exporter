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

	kingpin "github.com/alecthomas/kingpin/v2"
)

// **************
const (
	// Constant values
	metricsPublishingPort = ":8082"
)

var (

	// debug_flag = kingpin.Flag("debug", "debug connection checks.").Short('d').Default("false").Bool()
	listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(metricsPublishingPort).String()
	configFile    = kingpin.Flag("config.file", "Exporter configuration file.").Short('c').Default("conf/check_by_nrpe_exporter.yml").String()
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

}
