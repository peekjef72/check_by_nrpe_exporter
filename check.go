package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"
)

type CommandParam struct {
	Name      string             `yaml:"name"`
	Mandatory ConvertibleBoolean `yaml:"mandatory,omitempty"`
	Default   string             `yaml:"default,omitempty"`
	Encode    string             `yaml:"encode,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for CommandParam.
func (p *CommandParam) UnmarshalYAML(unmarshal func(interface{}) error) error {

	type plain CommandParam
	p.Name = "<not set>"
	p.Mandatory = false
	p.Default = "<not set>"

	if err := unmarshal((*plain)(p)); err != nil {
		return err
	}
	if p.Name == "" || p.Name == "<not set>" {
		return fmt.Errorf("invalid paramater name")
	}
	return nil
}

// checkConfig contains parameters for a nrpe command to check
type CheckConfig struct {
	Command       string         `yaml:"command"`
	CommandLine   string         `yaml:"command_line"`
	CommandParams []CommandParam `yaml:"params"`

	cmd_tmpl *template.Template
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for CheckConfig.
func (check *CheckConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {

	type plain CheckConfig
	var err error

	check.Command = "<not set>"
	if err := unmarshal((*plain)(check)); err != nil {
		return err
	}
	if check.Command == "" || check.Command == "<not set>" {
		return fmt.Errorf("invalid command")
	}
	if check.CommandLine == "" && len(check.CommandParams) > 0 {
		return fmt.Errorf("invalid command line")
	}
	check.cmd_tmpl = template.New("command")
	check.cmd_tmpl, err = check.cmd_tmpl.Parse(check.CommandLine)
	if err != nil {
		return fmt.Errorf("command_line template %s is invalid: %s", check.CommandLine, err)
	}
	return nil
}

// build the command paramater to send to nrpe_exporter
// check each CheckConfig.CommandParams: mandatory, default, encoding
// use CheckConfig.CommandLine template has final res
func (check *CheckConfig) Build(check_conf *CheckJSON) (string, error) {
	// var err error
	var (
		value   any
		r_value string
		ok      bool
	)
	// build a symbols table for the template
	formatter := make(map[string]any)
	for _, param := range check.CommandParams {
		value = 0
		if value, ok = check_conf.Params[param.Name]; ok {
			if param.Encode == "base64" {
				if r_value, ok = value.(string); ok {
					msg := []byte(r_value)
					value = base64.StdEncoding.EncodeToString(msg)
				}
			}
			formatter[param.Name] = value
		} else if param.Default != "<not set>" {
			formatter[param.Name] = param.Default
		} else if param.Mandatory {
			return "", fmt.Errorf("mandatory parameter '%s' not set", param.Name)
		}
	}
	tmp_res := new(strings.Builder)
	err := ((*template.Template)(check.cmd_tmpl)).Execute(tmp_res, &formatter)
	if err != nil {
		return "", err
	}

	// obtain final string from builder
	return tmp_res.String(), nil
}

func (check *CheckConfig) Play(poller *PollerConfig, check_conf *CheckJSON) (map[string]any, error) {

	// var res []byte

	target_host := check_conf.Target
	if check_conf.Target == "" {
		return nil, fmt.Errorf("target host is empty")
	}

	// to set into resty response
	//	timeout := 10
	// command := check_conf.Type
	// service := "sshd.service"
	// service := "nrpe_exporter.service"
	// cmd_params := fmt.Sprintf("-s %s", service)
	cmd_params, err := check.Build(check_conf)
	if err != nil {
		return nil, err
	}

	params := make(map[string]string)
	params["ssl"] = "true"
	params["command"] = check.Command
	params["result_message"] = "true"
	params["target"] = target_host
	params["params"] = cmd_params

	resp, data, err := poller.client.Get("export", params, false)
	// ok
	if err != nil {
		return nil, err
	} else if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid result http code: %s", resp.Status())
	} else if data != nil {
		// if cmd, ok := data[command].(map[string]any); ok {
		// 	cmd["service"] = service
		// }
		// res, _ = json.MarshalIndent(data, "", "   ")
	}
	return data, err
}
