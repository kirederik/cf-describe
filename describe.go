package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"code.cloudfoundry.org/cli/cf/terminal"
	"code.cloudfoundry.org/cli/plugin"
	"code.cloudfoundry.org/cli/plugin/models"
)

type DescribePlugin struct {
	cliConnection plugin.CliConnection
	brokerName    string
	serviceName   string
	showGuids     bool
}

func (d *DescribePlugin) GetMetadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name: "describe",
		Version: plugin.VersionType{
			Major: 1,
			Minor: 0,
			Build: 0,
		},
		MinCliVersion: plugin.VersionType{
			Major: 6,
			Minor: 7,
			Build: 0,
		},
		Commands: []plugin.Command{
			{
				Name:     "describe",
				HelpText: "Show information about brokers or service instances",
				UsageDetails: plugin.Usage{
					Usage: "cf describe [-b broker-name] [-s service-instance-name]",
					Options: map[string]string{
						"-b":          "The name of the broker",
						"-s":          "The name of the service instance",
						"-show-guids": "If set, will display the service instances guid",
					},
				},
			},
		},
	}
}

func (d *DescribePlugin) ParseFlags(args []string) {
	flagSet := flag.NewFlagSet(args[0], flag.ContinueOnError)
	brokerName := flagSet.String("b", "", "-b <broker-name>")
	serviceInstanceName := flagSet.String("s", "", "-s <service-instance-name>")
	showGuids := flagSet.Bool("show-guids", false, "")

	err := flagSet.Parse(args[1:])
	if err != nil {
		Fail(err, "cannot parse flags")
	}

	d.brokerName = *brokerName
	d.serviceName = *serviceInstanceName
	d.showGuids = *showGuids
}

func (d *DescribePlugin) Run(cliConnection plugin.CliConnection, args []string) {
	d.cliConnection = cliConnection
	if args[0] == "describe" {

		d.ParseFlags(args)

		if d.brokerName != "" {
			d.DescribeBroker()
		}

		if d.serviceName != "" {
			d.DescribeService()
		}
	}
}

type CurlResponse struct {
	TotalResults int `json:"total_results"`
	Resources    []struct {
		Metadata map[string]interface{} `json:"metadata"`
		Entity   map[string]interface{} `json:"entity"`
	} `json:"resources"`
}

func (d *DescribePlugin) DescribeBroker() {
	curlResponse := d.curl(fmt.Sprintf("/v2/service_brokers?q=name:%s", url.QueryEscape(d.brokerName)))
	if curlResponse.TotalResults == 0 {
		Warn(d.brokerName + " not found")
	}

	brokerGUID := curlResponse.Resources[0].Metadata["guid"]

	var response bytes.Buffer
	username, _ := d.cliConnection.Username()
	response.WriteString(fmt.Sprintf("Describing broker %s as visible by %s\n\n", Entity(d.brokerName), Entity(username)))

	plansResponse := d.curl(fmt.Sprintf("/v2/service_plans?q=service_broker_guid:%s", brokerGUID)) //TODO: pagination
	if plansResponse.TotalResults == 0 {
		Warn(d.brokerName + " has no plans")
	}

	spaces, _ := d.cliConnection.GetSpaces()
	orgs := d.getOrgs(spaces)

	for _, plan := range plansResponse.Resources {
		instances := d.curl(plan.Entity["service_instances_url"].(string))
		if instances.TotalResults > 0 {
			response.WriteString(fmt.Sprintf("Plan %s:\n", Entity(plan.Entity["name"].(string))))
			for _, instance := range instances.Resources {
				response.WriteString("  ")
				space := findSpace(spaces, instance.Entity["space_guid"].(string))
				if d.showGuids {
					response.WriteString(fmt.Sprintf("Guid: %s - ", Entity(instance.Metadata["guid"].(string))))
				}
				response.WriteString(
					fmt.Sprintf(
						"Name: %s - Org: %s - Space: %s\n",
						Entity(instance.Entity["name"].(string)),
						Entity(orgs[space.Guid]),
						Entity(space.Name),
					),
				)
			}
		}
	}

	fmt.Println(response.String())
}

func (d *DescribePlugin) getOrgs(spaces []plugin_models.GetSpaces_Model) map[string]string {
	orgs := map[string]string{}
	for _, space := range spaces {
		orgResponse := d.curl(fmt.Sprintf("/v2/organizations?q=space_guid:%s", space.Guid))
		orgs[space.Guid] = orgResponse.Resources[0].Entity["name"].(string)
	}
	return orgs
}

func findSpace(spaces []plugin_models.GetSpaces_Model, spaceGUID string) plugin_models.GetSpaces_Model {
	for _, s := range spaces {
		if s.Guid == spaceGUID {
			return s
		}
	}
	return plugin_models.GetSpaces_Model{}
}

func findOrg(orgs []plugin_models.GetOrgs_Model, orgGUID string) plugin_models.GetOrgs_Model {
	for _, s := range orgs {
		if s.Guid == orgGUID {
			return s
		}
	}
	return plugin_models.GetOrgs_Model{}
}

func (d *DescribePlugin) DescribeService() {

}

func (d *DescribePlugin) curl(endpoint string) CurlResponse {
	brokersResponse, _ := d.cliConnection.CliCommandWithoutTerminalOutput("curl", endpoint)

	var curlResponse CurlResponse
	err := json.Unmarshal([]byte(strings.Join(brokersResponse, "")), &curlResponse)
	if err != nil {
		Fail(err, "could not unmarshal response")
	}
	return curlResponse
}

func Entity(s string) string {
	return terminal.EntityNameColor(s)
}

func Fail(err error, message string) {
	fmt.Printf("%s: %s. Error: %s", terminal.FailureColor("FAILED"), message, err.Error())
	os.Exit(1)
}

func Warn(message string) {
	fmt.Printf(terminal.WarningColor(message))
	os.Exit(0)
}

func main() {
	plugin.Start(new(DescribePlugin))
}
