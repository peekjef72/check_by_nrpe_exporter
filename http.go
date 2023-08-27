package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-kit/log/level"
)

const (
	contentTypeHeader     = "Content-Type"
	contentLengthHeader   = "Content-Length"
	contentEncodingHeader = "Content-Encoding"
	acceptEncodingHeader  = "Accept-Encoding"
	applicationJSON       = "application/json"
)

type CheckJSON struct {
	Poller string            `json:"poller"`
	Target string            `json:"target"`
	Type   string            `json:"type"`
	Params map[string]string `json:"params"`
}

// ExporterHandlerFor returns an http.Handler for the provided Exporter.
func ChecksHandlerFor(config *Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// ctx, cancel := contextFor(req, config)
		// defer cancel()

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
			err := fmt.Errorf("poller parameter is missing")
			HandleError(http.StatusBadRequest, err, config, w, req)
			return
		}

		poller, found := config.Pollers[check_conf.Poller]
		if !found {
			err := fmt.Errorf("poller name '%s' not found", check_conf.Poller)
			HandleError(http.StatusNotFound, err, config, w, req)
			return
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
		// header.Set(contentLengthHeader, fmt.Sprint(buf.Len()))
		// if encoding != "" {
		// 	header.Set(contentEncodingHeader, encoding)
		// }
		w.Write(res)
	})
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
	w.WriteHeader(status)
	w.Header().Set(contentTypeHeader, string(applicationJSON))

	errMsg := make(map[string]any)
	errMsg["status"] = 0
	errMsg["message"] = fmt.Sprintf("%s", err)
	err_msg, err := json.Marshal(errMsg)
	if err != nil {
		level.Error(config.logger).Log("msg", fmt.Sprintf("Failed to generate error msg: %s", err))
		return
	}
	w.Write(err_msg)
}
