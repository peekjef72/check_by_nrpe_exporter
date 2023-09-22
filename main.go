package main

import (

	// "strings"

	// "encoding/json"
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

// **************
const (
	// Constant values
	metricsPublishingPort = ":8082"
	exeName               = "check_by_nrpe_exporter"
)

var (

	// debug_flag = kingpin.Flag("debug", "debug connection checks.").Short('d').Default("false").Bool()
	// listenAddress = kingpin.Flag("web.listen-address", "The address to listen on for HTTP requests.").Default(metricsPublishingPort).String()
	configFile   = kingpin.Flag("config.file", "Exporter configuration file.").Short('c').Default("conf/check_by_nrpe_exporter.yml").String()
	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, metricsPublishingPort)
)

type route struct {
	// method  string
	regex   *regexp.Regexp
	handler http.HandlerFunc
}

//	func newRoute(method, pattern string, handler http.HandlerFunc) route {
//		return route{method, regexp.MustCompile("^" + pattern + "$"), handler}
//	}
func newRoute(APIPath string, pattern string, handler http.HandlerFunc) route {
	if !strings.HasSuffix(APIPath, "/") {
		APIPath += "/"
	}
	pattern = APIPath + pattern
	return route{regexp.MustCompile("^" + pattern + "$"), handler}
}

type ctxKey struct {
}
type ctxValue struct {
	config *Config
	path   string
}

func BuildHandler(config *Config, logger log.Logger) http.Handler {
	var routes = []route{
		newRoute(config.Globals.Httpd.APIPath, "poller(?:/(.*))?", PollersHandler),
		newRoute(config.Globals.Httpd.APIPath, "check(?:/(.*))?", ChecksHandler),
		newRoute(config.Globals.Httpd.APIPath, "trycheck(?:/(.*))?", TriesHandler),
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		level.Debug(logger).Log("msg", fmt.Sprintf("%s %s from %s", req.Method, req.URL.Path, req.RemoteAddr))
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
				level.Debug(logger).Log("msg", fmt.Sprintf("%s", w.Header().Get("Status")))
				return
			}
		}
		if strings.HasPrefix(req.URL.Path, config.Globals.Httpd.PagesUri) {
			path := config.Globals.Httpd.PagesUri
			if strings.HasSuffix(req.URL.Path, "/") {
				path += "/"
			}
			f := http.StripPrefix(path, http.FileServer(http.Dir(config.Globals.Httpd.PagesPath)))
			f.ServeHTTP(w, req)
			return
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
	kingpin.Version(version.Print(exeName)).VersionFlag.Short('V')
	kingpin.HelpFlag.Short('h')

	kingpin.Parse()

	logger := promlog.New(&logConfig)

	config, err := Load(*configFile, logger)
	if err != nil {
		level.Error(logger).Log("msg", fmt.Sprintf("Error reading config: %s", err))
		os.Exit(1)
	}

	level.Info(logger).Log("msg", fmt.Sprintf("starting %s ...", exeName))

	server := &http.Server{
		Handler: BuildHandler(config, logger),
	}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
}
