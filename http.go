package main

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/go-kit/log/level"
	"github.com/prometheus/common/version"
	"golang.org/x/exp/maps"
)

const (
	contentTypeHeader     = "Content-Type"
	contentLengthHeader   = "Content-Length"
	contentEncodingHeader = "Content-Encoding"
	acceptEncodingHeader  = "Accept-Encoding"
	allowOriginHeader     = "Access-Control-Allow-Origin"
	applicationJSON       = "application/json"
	templates             = `
      <html>
      <head>
        <title>{{ .ExeName }}</title>
        <style type="text/css">
          body { margin: 0; font-family: "Helvetica Neue", Helvetica, Arial, sans-serif; font-size: 14px; line-height: 1.42857143; color: #333; background-color: #fff; }
          .navbar { display: flex; background-color: #222; margin: 0; border-width: 0 0 1px; border-style: solid; border-color: #080808; }
          .navbar > * { margin: 0; padding: 15px; }
          .navbar * { line-height: 20px; color: #9d9d9d; }
          .navbar a { text-decoration: none; }
          .navbar a:hover, .navbar a:focus { color: #fff; }
          .navbar-header { font-size: 18px; }
          body > * { margin: 15px; padding: 0; }
          pre { padding: 10px; font-size: 13px; background-color: #f5f5f5; border: 1px solid #ccc; }
          h1, h2 { font-weight: 500; }
          a { color: #337ab7; }
          a:hover, a:focus { color: #23527c; }
		  table { border: 1px solid #edd2e6; border-collapse: collapse; margin-bottom: 1rem; width: 80%; }
		  tr { border: 1px solid #edd2e6; padding: 0.3rem; text-align: left; width: 35%; }
		  th { border: 1px solid #edd2e6; padding: 0.3rem; }
		  td { border: 1px solid #edd2e6; padding: 0.3rem; }
		  .odd { background-color: rgba(0,0,0,.05); }
        </style>
      </head>
      <body>
	  <h2>Build Information</h2>
	  <table>
			<tbody>
			  <tr class="odd" >
				  <th>Version</th>
				  <td>{{ .Version }}</td>
			  </tr>
			  <tr>
				  <th>Revision</th>
				  <td>{{ .Revision }}</td>
			  </tr>
			  <tr class="odd" >
				  <th>Branch</th>
				  <td>{{ .Branch }}</td>
			  </tr>
			  <tr>
				  <th>BuildUser</th>
				  <td>{{ .BuildUser }}</td>
			  </tr>
			  <tr class="odd" >
				  <th>BuildDate</th>
				  <td>{{ .BuildDate }}</td>
			  </tr>
			  <tr>
				  <th>GoVersion</th>
				  <td>{{ .GoVersion }}</td>
			  </tr>
			  <tr class="odd" >
			      <th>Server start</th>
                  <td>{{ .StartTime }}</td>
              </tr>
		</tbody>
	  </table>
	  </body>
      </html>
    `
)

type CheckJSON struct {
	Poller string            `json:"poller"`
	Target string            `json:"target"`
	Type   string            `json:"type"`
	Params map[string]string `json:"params"`
}

// CheckHandlerFor returns an http.Handler for the provided POST to /check.
//
// awaiting data in JSON format:
// { "poller": "poller host", "type": "type of check", "params": {"param1": "value1", ...}}
func TriesHandler(w http.ResponseWriter, req *http.Request) {
	var (
		poller_name, check_name string
	)
	ctxval, ok := req.Context().Value(ctxKey{}).(*ctxValue)
	if !ok {
		err := fmt.Errorf("invalid context received")
		HandleError(http.StatusInternalServerError, err, nil, w, req)
		return

	}
	config := ctxval.config

	if req.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTION")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set(allowOriginHeader, "*")
		return
	} else if req.Method != http.MethodPost {
		err := fmt.Errorf("invalid method: only POST allowed")
		HandleError(http.StatusMethodNotAllowed, err, config, w, req)
		return
	}
	if ctxval.path != "" {
		path_elmts := strings.Split(ctxval.path, "/")
		if len(path_elmts) > 0 {
			poller_name = path_elmts[0]
		}
		if len(path_elmts) > 1 {
			check_name = path_elmts[1]
		}
	}
	// params := req.URL.Query()
	contentLength := 0
	if header := req.Header.Get(contentLengthHeader); header != "" {
		length, err := strconv.Atoi(header)
		if err != nil {
			HandleError(http.StatusBadRequest, err, config, w, req)
			return
		}
		contentLength = length
	}
	if contentLength <= 0 {
		err := fmt.Errorf("invalid content-length detect")
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}
	if contentLength >= config.Globals.MaxContentLength {
		err := fmt.Errorf("invalid content-length detect > %d", config.Globals.MaxContentLength)
		HandleError(http.StatusBadRequest, err, config, w, req)
		return

	}

	body := make([]byte, contentLength)
	defer req.Body.Close()
	_, err := req.Body.Read(body)
	if err != nil && err != io.EOF {
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}
	var check_conf CheckJSON

	err = json.Unmarshal(body, &check_conf)
	if err != nil {
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}

	if check_conf.Poller == "" {
		if poller_name != "" {
			check_conf.Poller = poller_name
		} else {
			err := fmt.Errorf("poller parameter is missing")
			HandleError(http.StatusBadRequest, err, config, w, req)
			return
		}
	}

	poller, found := config.Pollers[check_conf.Poller]
	if !found {
		err := fmt.Errorf("poller name '%s' not found", check_conf.Poller)
		HandleError(http.StatusNotFound, err, config, w, req)
		return
	}
	if check_conf.Type == "" {
		if check_name != "" {
			check_conf.Type = check_name
		} else {
			err := fmt.Errorf("check type parameter is missing")
			HandleError(http.StatusBadRequest, err, config, w, req)
			return

		}
	}
	check := config.FindCheck(check_conf.Type)
	if check == nil {
		err := fmt.Errorf("check type '%s' not found", check_conf.Type)
		HandleError(http.StatusNotFound, err, config, w, req)
		return
	}
	data, err := check.Play(poller, &check_conf)
	if err != nil {
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}
	// // Go through prometheus.Gatherers to sanitize and sort metrics.
	// gatherer := prometheus.Gatherers{exporter.WithContext(ctx, t)}
	// mfs, err := gatherer.Gather()
	// if err != nil {
	// 	level.Error(config.logger).Log("msg", fmt.Sprintf("Error gathering metrics for '%s': %s", tname, err))
	// 	if len(mfs) == 0 {
	// 		http.Error(w, "No metrics gathered, "+err.Error(), http.StatusInternalServerError)
	// 		return
	// 	}
	// }

	/*
		contentType := expfmt.Negotiate(req.Header)
		buf := getBuf()
		defer giveBuf(buf)
		writer, encoding := decorateWriter(req, buf)
		enc := expfmt.NewEncoder(writer, contentType)
		var errs prometheus.MultiError
		for _, mf := range mfs {
			if err := enc.Encode(mf); err != nil {
				errs = append(errs, err)
				level.Info(config.logger).Log("msg", fmt.Sprintf("Error encoding metric family %q: %s", mf.GetName(), err))
			}
		}
		if closer, ok := writer.(io.Closer); ok {
			closer.Close()
		}
		if errs.MaybeUnwrap() != nil && buf.Len() == 0 {
			err = fmt.Errorf("no result encoded: %s, ", errs.Error())
			HandleError(http.StatusInternalServerError, err, config, w, req)
			return
		}
	*/
	if data == nil {
		data = make(map[string]any)
	}
	data["status"] = 1
	data["message"] = "ok"

	res, err := json.Marshal(data)
	if err != nil {
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}

	header := w.Header()
	header.Set(contentTypeHeader, string(applicationJSON))
	header.Set(contentLengthHeader, fmt.Sprint(len(res)))
	header.Set(allowOriginHeader, "*")
	// header.Set(contentLengthHeader, fmt.Sprint(buf.Len()))
	// if encoding != "" {
	// 	header.Set(contentEncodingHeader, encoding)
	// }
	w.Write(res)
}

func PollersHandler(w http.ResponseWriter, req *http.Request) {
	// ctx, cancel := contextFor(req, config)
	// defer cancel()
	ctxval, ok := req.Context().Value(ctxKey{}).(*ctxValue)
	if !ok {
		err := fmt.Errorf("invalid context received")
		HandleError(http.StatusInternalServerError, err, nil, w, req)
		return

	}
	config := ctxval.config
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		err := fmt.Errorf("invalid method: only GET allowed")
		HandleError(http.StatusMethodNotAllowed, err, config, w, req)
		return
	}

	data := make(map[string]any)
	data["status"] = 1
	data["message"] = "ok"

	if ctxval.path != "" {
		var (
			poller *PollerConfig
			ok     bool = false
		)
		path_elmts := strings.Split(ctxval.path, "/")
		if len(path_elmts) > 0 {
			poller_name := path_elmts[0]
			poller, ok = config.Pollers[poller_name]
			if poller != nil {
				subdata := make(map[string]any)
				subdata[poller_name] = poller
				data["poller"] = subdata
			}
		}
		if !ok {
			err := fmt.Errorf("poller not found")
			HandleError(http.StatusNotFound, err, config, w, req)
			return
		}
	} else {
		data["pollers"] = maps.Keys(config.Pollers)
	}

	res, err := json.Marshal(data)
	if err != nil {
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}

	header := w.Header()
	header.Set(contentTypeHeader, string(applicationJSON))
	header.Set(contentLengthHeader, fmt.Sprint(len(res)))
	header.Set(allowOriginHeader, "*")
	// header.Set(contentLengthHeader, fmt.Sprint(buf.Len()))
	// if encoding != "" {
	// 	header.Set(contentEncodingHeader, encoding)
	// }
	w.Write(res)
}

func ChecksHandler(w http.ResponseWriter, req *http.Request) {
	// ctx, cancel := contextFor(req, config)
	// defer cancel()
	ctxval, ok := req.Context().Value(ctxKey{}).(*ctxValue)
	if !ok {
		err := fmt.Errorf("invalid context received")
		HandleError(http.StatusInternalServerError, err, nil, w, req)
		return

	}
	config := ctxval.config
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		err := fmt.Errorf("invalid method: only GET allowed")
		HandleError(http.StatusMethodNotAllowed, err, config, w, req)
		return
	}

	data := make(map[string]any)
	data["status"] = 1
	data["message"] = "ok"

	if ctxval.path != "" {
		var (
			check *CheckConfig
			ok    bool = false
		)
		path_elmts := strings.Split(ctxval.path, "/")
		if len(path_elmts) > 0 {
			check, ok = config.Checks[ctxval.path]
			if check != nil {
				subdata := make(map[string]any)
				subdata[ctxval.path] = check
				data["check"] = subdata
			}
		}
		if !ok {
			err := fmt.Errorf("check not found")
			HandleError(http.StatusNotFound, err, config, w, req)
			return
		}

	} else {
		data["checks"] = maps.Keys(config.Checks)
	}

	res, err := json.Marshal(data)
	if err != nil {
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}

	header := w.Header()
	header.Set(contentTypeHeader, string(applicationJSON))
	header.Set(contentLengthHeader, fmt.Sprint(len(res)))
	header.Set(allowOriginHeader, "*")
	// header.Set(contentLengthHeader, fmt.Sprint(buf.Len()))
	// if encoding != "" {
	// 	header.Set(contentEncodingHeader, encoding)
	// }
	w.Write(res)
}

/*
func contextFor(req *http.Request, config *Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(0)
	configTimeout := time.Duration(config.Globals.ScrapeTimeout)
	// If a timeout is provided in the Prometheus header, use it.
	if v := req.Header.Get("X-Prometheus-Scrape-Timeout-Seconds"); v != "" {
		timeoutSeconds, err := strconv.ParseFloat(v, 64)
		if err != nil {
			level.Error(config.logger).Log("msg", fmt.Sprintf("Failed to parse timeout (`%s`) from Prometheus header: %s", v, err))
		} else {
			timeout = time.Duration(timeoutSeconds * float64(time.Second))
		}
	}

	// If the configured scrape timeout is more restrictive, use that instead.
	if configTimeout > 0 && (timeout <= 0 || configTimeout < timeout) {
		timeout = configTimeout
	}

	if timeout <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), timeout)
}

var bufPool sync.Pool

func getBuf() *bytes.Buffer {
	buf := bufPool.Get()
	if buf == nil {
		return &bytes.Buffer{}
	}
	return buf.(*bytes.Buffer)
}

func giveBuf(buf *bytes.Buffer) {
	buf.Reset()
	bufPool.Put(buf)
}

// decorateWriter wraps a writer to handle gzip compression if requested.  It
// returns the decorated writer and the appropriate "Content-Encoding" header
// (which is empty if no compression is enabled).
func decorateWriter(request *http.Request, writer io.Writer) (w io.Writer, encoding string) {
	header := request.Header.Get(acceptEncodingHeader)
	parts := strings.Split(header, ",")
	for _, part := range parts {
		part := strings.TrimSpace(part)
		if part == "gzip" || strings.HasPrefix(part, "gzip;") {
			return gzip.NewWriter(writer), "gzip"
		}
	}
	return writer, ""
}
*/
// HandleError is an error handler that other handlers defer to in case of error. It is important to not have written
// anything to w before calling HandleError(), or the 500 status code won't be set (and the content might be mixed up).
func HandleError(status int, err error, config *Config, w http.ResponseWriter, r *http.Request) {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set(contentTypeHeader, string(applicationJSON))
	w.Header().Set(allowOriginHeader, "*")
	w.WriteHeader(status)

	errMsg := make(map[string]any)
	errMsg["status"] = 0
	errMsg["message"] = html.EscapeString(fmt.Sprintf("%s", err))
	err_msg, err := json.Marshal(errMsg)
	if err != nil && config != nil {
		level.Error(config.logger).Log("msg", fmt.Sprintf("Failed to generate error msg: %s", err))
		return
	}
	w.Write(err_msg)
}

var (
	statusTemplate = template.Must(template.New("").Parse(templates))
)

type versionInfo struct {
	ExeName   string
	Version   string
	Revision  string
	Branch    string
	BuildUser string
	BuildDate string
	GoVersion string
	StartTime string
}

// ConfigHandlerFunc is the HTTP handler for the `/config` page. It outputs the configuration marshaled in YAML format.
func StatusHandler(w http.ResponseWriter, req *http.Request) {
	vinfos := versionInfo{
		ExeName:   exeName,
		Version:   version.Version,
		Revision:  version.Revision,
		Branch:    version.Branch,
		BuildUser: version.BuildUser,
		BuildDate: version.BuildDate,
		GoVersion: runtime.Version(),
		StartTime: start_time,
	}

	if err := statusTemplate.Execute(w, vinfos); err != nil {
		ctxval, ok := req.Context().Value(ctxKey{}).(*ctxValue)
		if !ok {
			err := fmt.Errorf("invalid context received")
			HandleError(http.StatusInternalServerError, err, nil, w, req)
			return

		}
		config := ctxval.config
		if req.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			err := fmt.Errorf("invalid method: only GET allowed")
			HandleError(http.StatusMethodNotAllowed, err, config, w, req)
			return
		}
		HandleError(http.StatusBadRequest, err, config, w, req)
		return
	}
}
