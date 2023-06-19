package main

import (
	"encoding/json"
	"fmt"
	"path"
)

type Device struct {
	Name             string   `json:"name"`
	Identifiers      []string `json:"identifiers"`
	Manufacturer     string   `json:"manufacturer"`
	Model            string   `json:"model"`
	SwVersion        string   `json:"sw_version"`
	ConfigurationUrl string   `json:"configuration_url"`
}

type VacuumConfig struct {
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

type VacuumState struct {
	State        string `json:"state"`
	BatteryLevel int    `json:"battery_level"`
}

type Vacuum struct {
	ConfigTopic string
	Config      VacuumConfig
	Attributes  map[string]interface{}
	State       VacuumState
}

type SwitchConfig struct {
	Name              string `json:"name"`
	CommandTopic      string `json:"command_topic"`
	AvailabilityTopic string `json:"availability_topic"`
	StateTopic        string `json:"state_topic"`
	UniqueId          string `json:"unique_id"`
	Device            Device `json:"device"`
	PayloadOff        string `json:"payload_off"`
	PpayloadOn        string `json:"payload_on"`
}

type SwitchState string

type Switch struct {
	ConfigTopic string
	Config      SwitchConfig
	State       SwitchState
}

type HomeAssistant struct {
	ConfigBaseTopic  string
	CommandBaseTopic string
	Vacuum           *Vacuum
	Switches         []*Switch
}

func (self *HomeAssistant) ConfigureVacuum(roomba_id string) *Vacuum {
	base_vacuum_topic := path.Join(self.CommandBaseTopic, fmt.Sprintf("vacuum/homeassistant/%s/", roomba_id))

	self.Vacuum = &Vacuum{
		ConfigTopic: path.Join(self.ConfigBaseTopic, fmt.Sprintf("/vacuum/%s/vacuum/config", roomba_id)),
		Attributes:  make(map[string]interface{}),
		Config: VacuumConfig{
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
			AvailabilityTopic:   path.Join(base_vacuum_topic, "available"),
			StateTopic:          path.Join(base_vacuum_topic, "state"),
			CommandTopic:        path.Join(base_vacuum_topic, "command"),
			JsonAttributesTopic: path.Join(base_vacuum_topic, "attributes"),
			ErrorTopic:          path.Join(base_vacuum_topic, "state"),
			ErrorTemplate:       "{{ value_json.error }}",

			Device: Device{
				Name: "temp_name",
				Identifiers: []string{
					roomba_id,
				},
				ConfigurationUrl: "https://github.com/nslythe/roomba2mqtt",
				Manufacturer:     "IRobot",
				Model:            "",
				SwVersion:        "",
			},
		},
	}

	return self.Vacuum
}

func (self *HomeAssistant) ConfigureSwitch(roomba_id string, switch_id string, dev Device) *Switch {
	base_switch_topic := path.Join(self.CommandBaseTopic, fmt.Sprintf("switch/homeassistant/%s_%s/", roomba_id, switch_id))
	unique_id := "roomba_switch_" + roomba_id + "_" + switch_id

	for s := range self.Switches {
		if self.Switches[s].Config.UniqueId == unique_id {
			return self.Switches[s]
		}
	}

	return_value := &Switch{
		ConfigTopic: path.Join(self.ConfigBaseTopic, fmt.Sprintf("/switch/%s_%s/switch/config", roomba_id, switch_id)),
		Config: SwitchConfig{
			Name:              "zone_" + switch_id,
			UniqueId:          unique_id,
			Device:            dev,
			CommandTopic:      path.Join(base_switch_topic, "command"),
			AvailabilityTopic: path.Join(base_switch_topic, "available"),
			StateTopic:        path.Join(base_switch_topic, "state"),
			PayloadOff:        "OFF",
			PpayloadOn:        "ON",
		},
	}

	self.Switches = append(self.Switches, return_value)

	return return_value
}

func ConfigureHomeAssistant(master_mqtt_topic string) HomeAssistant {
	return HomeAssistant{
		ConfigBaseTopic:  "homeassistant",
		CommandBaseTopic: master_mqtt_topic,
	}
}

func (self *HomeAssistant) SendConfig() {
	data, err := json.Marshal(self.Vacuum.Config)
	if err != nil {
		panic(err)
	}
	master_mqtt_client.Publish(self.Vacuum.ConfigTopic, data, 2, true)

	for i := range self.Switches {
		data, err := json.Marshal(self.Switches[i].Config)
		if err != nil {
			panic(err)
		}
		master_mqtt_client.Publish(self.Switches[i].ConfigTopic, data, 2, true)
	}
}

func (self *HomeAssistant) SendState() {
	data, err := json.Marshal(self.Vacuum.State)
	if err != nil {
		panic(err)
	}
	master_mqtt_client.Publish(self.Vacuum.Config.StateTopic, data, 2, true)

	for i := range self.Switches {
		data = []byte(self.Switches[i].State)
		if err != nil {
			panic(err)
		}
		master_mqtt_client.Publish(self.Switches[i].Config.StateTopic, data, 2, true)
	}
}

func (self *HomeAssistant) SendAvaibality() {
	master_mqtt_client.Publish(self.Vacuum.Config.AvailabilityTopic, []byte("online"), 1, false)

	for i := range self.Switches {
		master_mqtt_client.Publish(self.Switches[i].Config.AvailabilityTopic, []byte("online"), 1, false)
	}
}

func (self *HomeAssistant) SendAttributes() {
	data, _ := json.Marshal(self.Vacuum.Attributes)
	master_mqtt_client.Publish(self.Vacuum.Config.JsonAttributesTopic, data, 1, false)
}
