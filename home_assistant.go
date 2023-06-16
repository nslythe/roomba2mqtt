package main

import (
	"fmt"
)

type Device struct {
	Name             string   `json:"name"`
	Identifiers      []string `json:"identifiers"`
	Manufacturer     string   `json:"manufacturer"`
	Model            string   `json:"model"`
	SwVersion        string   `json:"sw_version"`
	ConfigurationUrl string   `json:"configuration_url"`
}

type Vacuum struct {
	Name                string   `json:"name"`
	Schema              string   `json:"schema"`
	SupportedFeatures   []string `json:"supported_features"`
	AvailabilityTopic   string   `json:"availability_topic"`
	CommandTopic        string   `json:"command_topic"`
	StateTopic          string   `json:"state_topic"`
	JsonAttributesTopic string   `json:"json_attributes_topic"`
	ErrorTopic          string   `json:"error_topic"`
	ErrorTemplate       string   `json:"error_template"`
	UniqueId            string   `json:"unique_id"`
	Device              Device   `json:"device"`
}

type HAConfig struct {
	Vacuum      Vacuum
	ConfigTopic string
}

type VacuumState struct {
	State        string `json:"state"`
	BatteryLevel int    `json:"battery_level"`
}

func ConfigureHomeAssistant(roomba_id string) HAConfig {
	return_value := HAConfig{}

	return_value.ConfigTopic = fmt.Sprintf("homeassistant/vacuum/%s/vacuum/config", roomba_id)
	base_vacuum_topic := fmt.Sprintf("vacuum/homeassistant/%s/", roomba_id)

	return_value.Vacuum = Vacuum{
		Schema:   "state",
		Name:     "temp_name",
		UniqueId: "roomba_" + roomba_id,

		SupportedFeatures: []string{
			"start",
			"stop",
			"pause",
			"return_home",
			"battery",
			"status",
			//			"locate",
			"clean_spot",
			"send_command",
		},
		AvailabilityTopic:   base_vacuum_topic + "available",
		StateTopic:          base_vacuum_topic + "state",
		CommandTopic:        base_vacuum_topic + "command",
		JsonAttributesTopic: base_vacuum_topic + "attributes",
		ErrorTopic:          base_vacuum_topic + "state",
		ErrorTemplate:       "{{ value_json.error }}",

		Device: Device{
			Name: "temp_name",
			Identifiers: []string{
				roomba_id,
			},
			ConfigurationUrl: "https://www.irobot.ca/fr_CA/home",
			Manufacturer:     "IRobot",
			Model:            "",
			SwVersion:        "",
		},
	}
	return return_value
}
