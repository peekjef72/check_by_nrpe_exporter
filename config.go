package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

//
// Pollers
//

// PollerConfig defines a url and a set of collectors to be executed on it.
type PollerConfig struct {
	Scheme        string             `yaml:"scheme" json:"scheme"`
	Host          string             `yaml:"host" json:"host"`
	Port          string             `yaml:"port,omitempty" json:"port,omitempty"`
	BaseUrl       string             `yaml:"baseUrl,omitempty" json:"baseUrl,omitempty"`
	AuthConfig    AuthConfig         `yaml:"auth_mode,omitempty" json:"auth_mode,omitempty"`
	ProxyUrl      string             `yaml:"proxy,omitempty" json:"proxy,omitempty"`
	VerifySSL     ConvertibleBoolean `yaml:"verifySSL,omitempty" json:"verifySSL,omitempty"`
	ScrapeTimeout model.Duration     `yaml:"scrape_timeout,omitempty" json:"scrape_timeout,omitempty"` // connection timeout, per-target

	client *Client
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for PollerConfig.
func (p *PollerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain PollerConfig
	p.VerifySSL = true
	p.ScrapeTimeout = model.Duration(10 * time.Second)

	if err := unmarshal((*plain)(p)); err != nil {
		return err
	}

	// Check required fields
	if p.Scheme == "" {
		p.Scheme = "http"
	}
	if p.Port == "" {
		p.Port = "9275"
	}
	if p.BaseUrl != "" {
		p.BaseUrl = strings.Trim(p.BaseUrl, "/")
	}

	if p.Host == "" {
		return fmt.Errorf("missing proxy host name %+v", p)
	}
	if p.AuthConfig.Mode == "" {
		p.AuthConfig.Mode = "basic"
	}
	return nil
}

// ConvertibleBoolean special type to retrive 1 yes true to boolean true
type ConvertibleBoolean bool

func (bit *ConvertibleBoolean) UnmarshalJSON(data []byte) error {
	asString := strings.ToLower(string(data))
	if asString == "1" || asString == "true" || asString == "yes" || asString == "on" {
		*bit = true
	} else if asString == "0" || asString == "false" || asString == "no" || asString == "off" {
		*bit = false
	} else {
		return fmt.Errorf("boolean unmarshal error: invalid input %s", asString)
	}
	return nil
}

func (bit *ConvertibleBoolean) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain string
	var data string
	if err := unmarshal((*plain)(&data)); err != nil {
		return err
	}
	asString := strings.ToLower(string(data))
	if asString == "1" || asString == "true" || asString == "yes" || asString == "on" {
		*bit = true
	} else if asString == "0" || asString == "false" || asString == "no" || asString == "off" {
		*bit = false
	} else {
		return fmt.Errorf("boolean unmarshal error: invalid input %s", asString)
	}
	return nil
}

// Secret special type for storing secrets.
type Secret string

// UnmarshalYAML implements the yaml.Unmarshaler interface for Secrets.
func (s *Secret) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Secret
	return unmarshal((*plain)(s))
}

// MarshalYAML implements the yaml.Marshaler interface for Secrets.
func (s Secret) MarshalYAML() (interface{}, error) {
	if s != "" {
		return "<secret>", nil
	}
	return nil, nil
}

// MarshalJSON implements the json.Marshaler interface for Secrets.
func (s Secret) MarshalJSON() ([]byte, error) {
	if s != "" {
		return []byte("<secret>"), nil
	}
	return nil, nil
}

type AuthConfig struct {
	Mode     string `yaml:"mode,omitempty" json:"mode,omitempty"` // basic, encrypted, bearer
	Username string `yaml:"user,omitempty" json:"user,omitempty"`
	Password Secret `yaml:"password,omitempty" json:"password,omitempty"`
	Token    Secret `yaml:"token,omitempty" json:"token,omitempty"`
	// authKey  string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for authConfig
func (auth *AuthConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain AuthConfig
	if err := unmarshal((*plain)(auth)); err != nil {
		return err
	}

	// Check required fields
	if auth.Mode == "" {
		auth.Mode = "basic"
	} else {
		auth.Mode = strings.ToLower(auth.Mode)
		mode := make(map[string]int)
		for _, val := range []string{"basic", "token", "script"} {
			mode[val] = 1
		}
		if _, err := mode[auth.Mode]; !err {
			return fmt.Errorf("invalid mode auth %s", auth.Mode)
		}
	}
	if auth.Mode == "token" && auth.Token == "" {
		return fmt.Errorf("token not set with auth mode 'token'")
	}

	return nil
}

// GlobalConfig contains globally applicable defaults.
type GlobalConfig struct {
	ScrapeTimeout    model.Duration `yaml:"scrape_timeout"` // per-scrape timeout, global
	MaxContentLength int            `yaml:"max_content_length,omitempty"`
	Httpd            *HttpdConfig   `yaml:"httpd,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for GlobalConfig.
func (g *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {

	g.ScrapeTimeout = model.Duration(10 * time.Second)
	g.MaxContentLength = 16384
	type plain GlobalConfig
	if err := unmarshal((*plain)(g)); err != nil {
		return err
	}
	if g.Httpd == nil {
		g.Httpd = &HttpdConfig{}
		g.Httpd.init()
	}
	return nil
}

type HttpdConfig struct {
	PagesUri  string `yaml:"pages_uri"`
	PagesPath string `yaml:"pages_uri"`
	APIPath   string `yaml:"api_uri"`
}

func (h *HttpdConfig) init() {
	h.PagesUri = "/html"
	h.PagesPath = "pages"
	h.APIPath = "/api"
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for GlobalConfig.
func (h *HttpdConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	h.init()

	type plain HttpdConfig
	if err := unmarshal((*plain)(h)); err != nil {
		return err
	}

	return nil
}

// Config is a collection of targets and collectors.
type Config struct {
	Globals *GlobalConfig            `yaml:"global"`
	Pollers map[string]*PollerConfig `yaml:"pollers"`
	Checks  map[string]*CheckConfig  `yaml:"checks"`

	configFile string
	logger     log.Logger
}

// Load attempts to parse the given config file and return a Config object.
func Load(configFile string, logger log.Logger) (*Config, error) {
	level.Info(logger).Log("msg", fmt.Sprintf("Loading configuration from %s", configFile))
	buf, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	c := Config{
		configFile: configFile,
		logger:     logger,
	}

	err = yaml.Unmarshal(buf, &c)
	if err != nil {
		return nil, err
	}
	for seg, poller := range c.Pollers {
		poller.client = newClient(poller, logger)
		if poller.client == nil {
			return nil, fmt.Errorf("invalid poller parameter for '%s'", seg)
		}
	}
	return &c, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Config.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if len(c.Pollers) == 0 {
		return fmt.Errorf("at least one poller in `pollers` must be defined")
	}
	return nil
}

func (c *Config) FindCheck(check_type string) *CheckConfig {
	if check, ok := c.Checks[check_type]; ok {
		return check
	}
	return nil
}
