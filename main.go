package main

import (

	// "strings"

	// "encoding/json"
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"

	// "strings"

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

type route struct {
	// method  string
	regex   *regexp.Regexp
	handler http.HandlerFunc
}

//	func newRoute(method, pattern string, handler http.HandlerFunc) route {
//		return route{method, regexp.MustCompile("^" + pattern + "$"), handler}
//	}
func newRoute(pattern string, handler http.HandlerFunc) route {
	return route{regexp.MustCompile("^" + pattern + "$"), handler}
}

type ctxKey struct {
}
type ctxValue struct {
	config *Config
	path   string
}

func BuildHandler(config *Config) http.Handler {
	var routes = []route{
		newRoute("/api/poller(?:/(.*))?", PollersHandler),
		newRoute("/api/check(?:/(.*))?", ChecksHandler),
		newRoute("/api/trycheck(?:/(.*))?", TriesHandler),
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// var allow []string
		for _, route := range routes {
			matches := route.regex.FindStringSubmatch(req.URL.Path)
			if len(matches) > 0 {
				// if req.Method != route.method {
				// 	allow = append(allow, route.method)
				// 	continue
				// }
				ctxval := &ctxValue{
					config: config,
					path:   matches[1],
				}
				ctx := context.WithValue(req.Context(), ctxKey{}, ctxval)
				route.handler(w, req.WithContext(ctx))
				return
			}
		}
		// if len(allow) > 0 {
		// 	w.Header().Set("Allow", strings.Join(allow, ", "))
		// 	http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		// 	return
		// }
		err := fmt.Errorf("not found")
		HandleError(http.StatusNotFound, err, config, w, req)
	})
}

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

	// http.Handle("/check", ChecksHandlerFor(config))
	// http.Handle("/pollers/*", PollersHandlerFor(config))
	level.Info(logger).Log("msg", "Listening on address", "address", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, BuildHandler(config)); err != nil {
		level.Error(logger).Log("msg", "Error starting HTTP server", "errmsg", err)
		os.Exit(1)
	}

}
