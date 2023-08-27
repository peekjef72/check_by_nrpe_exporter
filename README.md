# check_by_nrpe_exporter

- webserver that wait for clients request sent in JSON format
- check request content for a command that is understood
- translate it to expected nrpe_exporter format and send it to corresponding nrpe_exporter (poller)
- translate openmetrics response from poller into a JSON format.

# pre-requirements
* nrpe_exporter v 0.3.0:
    * build with result_message enabled
    * transport ssl (openssl 1.1 or 3.1 enabled)
    
* nrpe agent compiled with allow arguments

# configuration
config is defined in conf/check_by_nrpe_exporter.yml
it defines:
 * known pollers list
 * allowed commands

 ## pollers list

 format:
 ```ymal
 pollers:
    name:
        scheme: http(default)|https         # may be omitted
        host: fully_qualified_domain_name   # mandatory
        port: 9275                          # may be omitted
        baseUrl: ""             # may be omitted
        proxy:                  # may be omitted
        VeriySSL: true          # may be omitted
        connection_timeout: 10  # may be omitted

    example:
        host: my_nrpe_exporter.domain.name

 ```

 ## commands
 list of knowns command, that user can address and check every targets using poller.

format:

```yaml
checks:
    command_name:
        command: real_nrpe_command_to_play
        command_line: command_template_argument_line
        params:
            - name: param_name
              mandatory: true|false
              default: "value"
              encode: "" or "base64"

  filesystem:
    command: check_disk
    command_line: "
      -X binfmt_misc -X devpts -X devtmpfs -X none -X proc -X procfs -X rpc_pipefs 
      -X sysfs -X tmpfs -X overlay -X debugfs -X tracefs -X autofs -X cgroup 
      --errors-only {{ .options }} -t {{ .timeout }} -w {{ .perc_threshold_warn }} -c {{ .perc_threshold_crit }} --all --local -i /.snapshot/"
    params:
      - name: options
        default: ""
      - name: perc_threshold_warn
        default: 95
      - name: perc_threshold_crit
        default: 97
      - name: timeout
        default: 5

```

# checks

command check are sent to "/check"
format:

```json
{
    "poller": "poller_name",
    "type": "command_name",
    "target": "host:port",
    "params": {
        "name1": "value1", 
        "name_2": "value2"
        [,...]
    }
}
```

e.g.: check for process

```json
{"poller": "poller_name", "target": "hostname:5666", "type": "process", "params": {"process": ".*sshd.*"}}
```