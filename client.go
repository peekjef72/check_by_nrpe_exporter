package main

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"crypto/tls"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/go-resty/resty/v2"
)

var (
	prom_met_pat   = regexp.MustCompile(`^(?P<metric_name>[a-zA-Z][a-zA-Z_0-9]+)(?:{(?P<labels>[^{]+)})? (?P<value>(?:\+|\-)?\d+(?:\.\d*)?)`)
	prom_label_pat = regexp.MustCompile(`^([a-zA-Z][a-zA-Z_0-9]+)="([^"]*)"`)
)

// Query wraps a sql.Stmt and all the metrics populated from it. It helps extract keys and values from result rows.
type Client struct {
	APIEndPoint string
	user        string
	password    string
	client      *resty.Client

	logContext []interface{}
	logger     log.Logger
	url        string
	auth_token string
	//mutex      sync.Mutex
}

// func onErrorHook(req *resty.Request, err error) {
// 	if v, ok := err.(*resty.ResponseError); ok {
// 		// Do something with v.Response
// 		if v.Response.StatusCode() == 403 {

// 		}
// 	}
// 	// Log the error, increment a metric, etc...
// }

func newClient(poller *PollerConfig, logger log.Logger) *Client {
	apiendpoint := fmt.Sprintf("%s://%s:%s", poller.Scheme, poller.Host, poller.Port)
	baseurl := strings.TrimPrefix(poller.BaseUrl, "/")
	if baseurl != "" {
		apiendpoint += "/" + baseurl
	}
	cl := &Client{
		client:      resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
		APIEndPoint: apiendpoint,
		logger:      logger,
	}
	if poller.Scheme == "https" {
		cl.client = resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: !bool(poller.VerifySSL)})
	} else if poller.Scheme == "http" {
		cl.client = resty.New()
	} else {
		level.Error(cl.logger).Log("msg", fmt.Sprintf("invalid scheme for url '%s'", poller.Scheme))
		return nil
	}
	timeout := time.Duration(poller.ScrapeTimeout)
	// cl.client.SetTransport(
	// 	&http.Transport{
	// 		DialContext: (&net.Dialer{
	// 			Timeout: timeout,
	// 		}).DialContext,
	// 	},
	// )
	cl.client.SetTimeout(timeout)

	if poller.AuthConfig.Mode == "basic" {
		passwd := string(poller.AuthConfig.Password)
		if poller.AuthConfig.Username != "" && passwd != "" &&
			!strings.Contains(passwd, "/encrypted/") {
			cl.client.SetBasicAuth(poller.AuthConfig.Username, passwd)
		}
	} else if poller.AuthConfig.Mode == "token" && poller.AuthConfig.Token != "" {
		cl.client.SetAuthToken(string(poller.AuthConfig.Token))
	}
	if poller.ProxyUrl != "" {
		cl.client.SetProxy(poller.ProxyUrl)
	}

	cl.client.SetHeader("Content-Type", "application/json").SetHeader("Accept", "*/*").SetHeader("X-Prometheus-Scrape-Timeout-Seconds", fmt.Sprintf("%f", math.Floor(timeout.Seconds())))
	// cl.client.OnError(onErrorHook)

	return cl
}

func (c *Client) Clone() *Client {
	//sync.Mutex{}
	cl := &Client{
		client:      resty.New().SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}),
		APIEndPoint: c.APIEndPoint,
		logContext:  []interface{}{},
		user:        c.user,
		password:    c.password,
		logger:      c.logger,
		url:         c.url,
		auth_token:  c.auth_token,
	}
	cl.client.SetHeader("Accept", "*/*")
	return cl
}

// set the url for client
func (c *Client) SetUrl(uri string) string {
	c.url = fmt.Sprintf("%s/%s", c.APIEndPoint, strings.TrimPrefix(uri, "/"))
	level.Debug(c.logger).Log("url", c.url)
	return c.url
}

// HTTP GET encapsulation
func (c *Client) Get(
	uri string,
	params map[string]string,
	with_retry bool) (
	*resty.Response,
	map[string]interface{},
	error) {

	return c.Execute("GET", uri, params, nil, with_retry)
}

// Post PowerMax HTTP POST encapsulation
func (c *Client) Post(
	uri string,
	body interface{},
	with_retry bool) (
	*resty.Response,
	map[string]interface{},
	error) {

	return c.Execute("POST", uri, nil, body, with_retry)
}

// parse a response to a json map[string]interface{}
// func (c *Client) getJSONResponse(resp *resty.Response) map[string]interface{} {
// 	var err error
// 	var data map[string]interface{}

// 	body := resp.Body()
// 	if len(body) > 0 {
// 		content_type := resp.Header().Get("content-type")
// 		if content_type == "application/json" {
// 			// tmp := make([]byte, len(body))
// 			// copy(tmp, body)
// 			err = json.Unmarshal(body, &data)
// 			if err != nil {
// 				level.Error(c.logger).Log("errmsg", "Fail to decode json results")
// 			}
// 		}
// 	}
// 	return data
// }

// parse a response to a json map[string]interface{}
func (c *Client) getNRPEResponse(resp *resty.Response) map[string]interface{} {
	var (
		err                                  error
		metric_name, label_name, label_value string
		value                                any
	)
	data := make(map[string]any)
	cmd := make(map[string]any)
	perfs := make(map[string]any)

	body := resp.Body()
	if len(body) > 0 {
		content_type := resp.Header().Get("content-type")
		if strings.Contains(content_type, "text/plain") {
			// tmp := make([]byte, len(body))
			// copy(tmp, body)
			for _, line := range bytes.SplitAfter(body, []byte("\n")) {
				if bytes.Index(line, []byte("#")) == 0 {
					continue
				}
				line = bytes.TrimSuffix(line, []byte{10})
				line = bytes.TrimSuffix(line, []byte("\n"))
				match := prom_met_pat.FindSubmatch(line)
				if len(match) > 3 {
					// metric_name 1
					// labels 2
					// value 3
					if len(match[1]) > 0 {
						metric_name = string(match[1])
					}
					if len(match[3]) > 0 {
						if metric_name == "nrpe_command_status" {
							switch string(match[3]) {
							case "0":
								value = "OK"
							case "1":
								value = "WARNING"
							case "2":
								value = "CRITICAL"
							default:
								value = "UNKNOWN"
							}
						} else {
							if value, err = strconv.ParseFloat(string(match[3]), 64); err != nil {
								value = 0
							}
						}
					}
					label_name = ""
					label_value = ""
					if len(match[2]) > 0 {
						for _, key_value := range bytes.SplitAfter(match[2], []byte(",")) {
							label_match := prom_label_pat.FindSubmatch(key_value)
							if len(label_match) > 2 {
								if len(label_match[1]) > 0 {
									label_name = string(label_match[1])
								}
								if len(label_match[2]) > 0 {
									label_value = string(label_match[2])
								}
								if label_name == "command" {
									cmd[metric_name] = value
									// build object "check_service" : {"nrpe_command_ok": x, "nrpe_command_result_msg: "", ...}
									if _, ok := data[label_value]; !ok {
										data[label_value] = cmd
									}
								} else {
									if !strings.Contains(metric_name, "nrpe_") {
										perfs[label_value] = value
									} else {
										cmd[label_name] = label_value
									}
								}
							}
						}
					}
					if label_name == "" {
						data[metric_name] = value
					}
				}
			}
		}
		if len(perfs) > 0 {
			cmd["perfdata"] = perfs
		}
	}
	return data
}

// sent HTTP Method to uri with params or body and get the reponse and the json obj
func (c *Client) Execute(
	method, uri string,
	params map[string]string,
	body interface{},
	with_retry bool) (
	*resty.Response,
	map[string]interface{},
	error) {

	var err error
	var data map[string]interface{}

	// lock client until current request is performed
	// c.mutex.Lock()
	// defer c.mutex.Unlock()

	c.SetUrl(uri)
	level.Debug(c.logger).Log("action", method, "url", c.url)
	if body != nil {
		level.Debug(c.logger).Log("action", method, "url", c.url, "body", fmt.Sprintf("%+v", body))
	}
	var resp *resty.Response

	req := c.client.NewRequest()
	if body != nil {
		req.SetBody(body)
	}
	if len(params) > 0 {
		req.SetQueryParams(params)
	}
	resp, err = req.Execute(method, c.url)
	if len(params) > 0 && req.RawRequest != nil && req.RawRequest.URL != nil {
		level.Debug(c.logger).Log("action", method, "params", req.RawRequest.URL.RawQuery)
	}
	if err == nil {
		if resp.StatusCode() == 200 {
			data = c.getNRPEResponse(resp)
		}
	}

	return resp, data, err
}
