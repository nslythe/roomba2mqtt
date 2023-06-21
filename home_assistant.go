package main

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type Entity struct {
	ConfigTopic        string
	Attributes         map[string]interface{}
	HomeAssistant      *HomeAssistant
	NeedSendState      bool
	NeedSendAttributes bool
	NeedSendConfig     bool
}

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
	Device              *Device  `json:"device"`
}

type VacuumState struct {
	State        string `json:"state"`
	BatteryLevel int    `json:"battery_level"`
}

type Vacuum struct {
	Entity
	Config VacuumConfig
	State  VacuumState
}

type SwitchConfig struct {
	Name                string  `json:"name"`
	CommandTopic        string  `json:"command_topic"`
	AvailabilityTopic   string  `json:"availability_topic"`
	JsonAttributesTopic string  `json:"json_attributes_topic"`
	StateTopic          string  `json:"state_topic"`
	UniqueId            string  `json:"unique_id"`
	Device              *Device `json:"device"`
	PayloadOff          string  `json:"payload_off"`
	PayloadOn           string  `json:"payload_on"`
	Icon                string  `json:"icon"`
}

type SwitchState bool

type Switch struct {
	Entity
	Config SwitchConfig
	State  SwitchState
}

type RoombaRegionSwitch struct {
	Switch
	Region *Region
	Map    *Map
}

type SelectConfig struct {
	Name                string   `json:"name"`
	CommandTopic        string   `json:"command_topic"`
	AvailabilityTopic   string   `json:"availability_topic"`
	JsonAttributesTopic string   `json:"json_attributes_topic"`
	StateTopic          string   `json:"state_topic"`
	UniqueId            string   `json:"unique_id"`
	Device              *Device  `json:"device"`
	Icon                string   `json:"icon"`
	Options             []string `json:"options"`
	EntityCategory      string   `json:"entity_category"`
}

type SelectState string

type Select struct {
	Entity
	Config SelectConfig
	State  SelectState
}
type CleanPassSelect struct {
	Select
}

type HomeAssistant struct {
	MqttClient       MqttClient
	MasterMqttClient MqttClient
	ConfigBaseTopic  string
	CommandBaseTopic string
	Entities         []interface{}
	Vacuum           *Vacuum
	RegionSwitches   []*RoombaRegionSwitch
	CleanPassSelect  *CleanPassSelect
}

var global_retain_value bool = true
var global_qos_value uint8 = 0

func (self *HomeAssistant) ConfigureVacuum(roomba_id string) *Vacuum {
	base_vacuum_topic := path.Join(self.CommandBaseTopic, fmt.Sprintf("vacuum/homeassistant/%s/", roomba_id))

	vacuum := &Vacuum{
		Entity: Entity{
			HomeAssistant: self,
			ConfigTopic:   path.Join(self.ConfigBaseTopic, fmt.Sprintf("/vacuum/%s/vacuum/config", roomba_id)),
			Attributes:    make(map[string]interface{}),
		},
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

			Device: &Device{
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

	self.Entities = append(self.Entities, vacuum)
	self.Vacuum = vacuum

	return vacuum
}

func (self *HomeAssistant) ConfigureRoombaRegionSwitch(roomba_id string, switch_id string, dev *Device, icon string) *RoombaRegionSwitch {
	return_value := &RoombaRegionSwitch{
		Switch: *self.ConfigureSwitch(roomba_id, switch_id, dev, icon),
	}

	self.RegionSwitches = append(self.RegionSwitches, return_value)

	self.MasterMqttClient.Subscribe(return_value.Config.CommandTopic, return_value.CommandHandler)

	return return_value
}

func (self *HomeAssistant) ConfigureSwitch(device_id string, switch_id string, dev *Device, icon string) *Switch {
	base_switch_topic := path.Join(self.CommandBaseTopic, fmt.Sprintf("switch/homeassistant/%s_%s/", device_id, switch_id))
	unique_id := "roomba_switch_" + device_id + "_" + switch_id

	return_value := &Switch{
		Entity: Entity{
			HomeAssistant: self,
			ConfigTopic:   path.Join(self.ConfigBaseTopic, fmt.Sprintf("/switch/%s_%s/switch/config", device_id, switch_id)),
			Attributes:    make(map[string]interface{}),
		},
		Config: SwitchConfig{
			Name:                "zone_" + switch_id,
			UniqueId:            unique_id,
			Device:              dev,
			CommandTopic:        path.Join(base_switch_topic, "command"),
			AvailabilityTopic:   path.Join(base_switch_topic, "available"),
			JsonAttributesTopic: path.Join(base_switch_topic, "attributes"),
			StateTopic:          path.Join(base_switch_topic, "state"),
			PayloadOff:          "OFF",
			PayloadOn:           "ON",
			Icon:                icon,
		},
	}

	self.Entities = append(self.Entities, return_value)
	return_value.NeedSendConfig = true
	return_value.NeedSendState = true

	return return_value
}

func (self *HomeAssistant) ConfigureSelect(device_id string, select_id string, dev *Device, icon string, options []string) *Select {
	base_switch_topic := path.Join(self.CommandBaseTopic, fmt.Sprintf("select/homeassistant/%s_%s/", device_id, select_id))
	unique_id := "roomba_switch_" + device_id + "_" + select_id

	return_value := &Select{
		Entity: Entity{
			HomeAssistant: self,
			ConfigTopic:   path.Join(self.ConfigBaseTopic, fmt.Sprintf("/select/%s_%s/select/config", device_id, select_id)),
			Attributes:    make(map[string]interface{}),
		},
		Config: SelectConfig{
			Name:                select_id,
			UniqueId:            unique_id,
			Device:              dev,
			CommandTopic:        path.Join(base_switch_topic, "command"),
			AvailabilityTopic:   path.Join(base_switch_topic, "available"),
			JsonAttributesTopic: path.Join(base_switch_topic, "attributes"),
			StateTopic:          path.Join(base_switch_topic, "state"),
			Options:             options,
			Icon:                icon,
			EntityCategory:      "config",
		},
	}

	self.Entities = append(self.Entities, return_value)
	return_value.NeedSendConfig = true
	return_value.NeedSendState = true

	return return_value
}

func (self *HomeAssistant) ConfigureCleanPassSelect(device_id string, select_id string, dev *Device, icon string, options []string) *CleanPassSelect {
	return_value := &CleanPassSelect{
		Select: *self.ConfigureSelect(device_id, select_id, dev, icon, options),
	}

	self.CleanPassSelect = return_value

	return_value.State = SelectState(options[0])
	return_value.NeedSendConfig = true
	return_value.NeedSendState = true

	self.MasterMqttClient.Subscribe(return_value.Config.CommandTopic, return_value.CommandHandler)

	return return_value
}

func (self *CleanPassSelect) CommandHandler(topic string, payload []byte) {
	self.State = SelectState(payload)
	self.NeedSendState = true
}

func (self *HomeAssistant) SendUpdate() {
	for i := range self.Entities {
		vacuum, ok := self.Entities[i].(*Vacuum)
		if ok {
			vacuum.SendConfig()
			vacuum.SendAttributes()
			vacuum.SendState()
			vacuum.SendAvaibality()
		}

		region_switch, ok := self.Entities[i].(*Switch)
		if ok {
			region_switch.SendConfig()
			region_switch.SendAttributes()
			region_switch.SendState()
			region_switch.SendAvaibality()
		}

		select_entity, ok := self.Entities[i].(*Select)
		if ok {
			select_entity.SendConfig()
			select_entity.SendAttributes()
			select_entity.SendState()
			select_entity.SendAvaibality()
		}
	}
}

func ConfigureHomeAssistant(master_mqtt_topic string, master_mqtt_client MqttClient) HomeAssistant {
	return HomeAssistant{
		ConfigBaseTopic:  "homeassistant",
		CommandBaseTopic: master_mqtt_topic,
		MasterMqttClient: master_mqtt_client,
	}
}

func (self *Vacuum) SendConfig() {
	if self.NeedSendConfig {
		data, err := json.Marshal(self.Config)
		if err != nil {
			panic(err)
		}
		master_mqtt_client.Publish(self.ConfigTopic, data, 0, true)
		self.NeedSendConfig = false

		for i := range self.HomeAssistant.Entities {
			region, ok := self.HomeAssistant.Entities[i].(*RoombaRegionSwitch)
			if ok {
				region.NeedSendConfig = true
			}

			select_entity, ok := self.HomeAssistant.Entities[i].(*Select)
			if ok {
				select_entity.NeedSendConfig = true
			}
		}
	}
}
func (self *Switch) SendConfig() {
	if self.NeedSendConfig {
		data, err := json.Marshal(self.Config)
		if err != nil {
			panic(err)
		}
		master_mqtt_client.Publish(self.ConfigTopic, data, global_qos_value, global_retain_value)
		self.NeedSendConfig = false
	}
}
func (self *Select) SendConfig() {
	if self.NeedSendConfig {
		data, err := json.Marshal(self.Config)
		if err != nil {
			panic(err)
		}
		master_mqtt_client.Publish(self.ConfigTopic, data, global_qos_value, global_retain_value)
		self.NeedSendConfig = false
	}
}
func (self *Vacuum) SendState() {
	var err error
	var data []byte
	if self.NeedSendState {
		data, err = json.Marshal(self.State)
		if err != nil {
			panic(err)
		}
		master_mqtt_client.Publish(self.Config.StateTopic, data, global_qos_value, global_retain_value)
		self.NeedSendState = false
	}
}

func (self *Switch) SendState() {
	if self.NeedSendState {
		var data []byte
		if self.State {
			data = []byte(self.Config.PayloadOn)
		} else {
			data = []byte(self.Config.PayloadOff)
		}
		master_mqtt_client.Publish(self.Config.StateTopic, data, global_qos_value, global_retain_value)
		self.NeedSendState = false
	}
}

func (self *Select) SendState() {
	if self.NeedSendState {
		data := []byte(self.State)
		master_mqtt_client.Publish(self.Config.StateTopic, data, global_qos_value, global_retain_value)
		self.NeedSendState = false
	}
}

func (self *Vacuum) SendAvaibality() {
	master_mqtt_client.Publish(self.Config.AvailabilityTopic, []byte("online"), global_qos_value, global_retain_value)
}
func (self *Switch) SendAvaibality() {
	master_mqtt_client.Publish(self.Config.AvailabilityTopic, []byte("online"), global_qos_value, global_retain_value)
}
func (self *Select) SendAvaibality() {
	master_mqtt_client.Publish(self.Config.AvailabilityTopic, []byte("online"), global_qos_value, global_retain_value)
}

func (self *Vacuum) SendAttributes() {
	if self.NeedSendAttributes {
		data, _ := json.Marshal(self.Attributes)
		self.HomeAssistant.MasterMqttClient.Publish(self.Config.JsonAttributesTopic, data, global_qos_value, global_retain_value)
		self.NeedSendAttributes = false
	}
}
func (self *Switch) SendAttributes() {
	if self.NeedSendAttributes {
		data, _ := json.Marshal(self.Attributes)
		self.HomeAssistant.MasterMqttClient.Publish(self.Config.JsonAttributesTopic, data, global_qos_value, global_retain_value)
		self.NeedSendAttributes = false
	}
}
func (self *Select) SendAttributes() {
	if self.NeedSendAttributes {
		data, _ := json.Marshal(self.Attributes)
		self.HomeAssistant.MasterMqttClient.Publish(self.Config.JsonAttributesTopic, data, global_qos_value, global_retain_value)
		self.NeedSendAttributes = false
	}
}

func (self *RoombaRegionSwitch) CommandHandler(topic string, payload []byte) {
	self.State = string(payload) == self.Config.PayloadOn
	self.NeedSendState = true
	self.Switch.SendState()
}

func (self *Vacuum) CommandHandler(topic string, payload []byte) {
	command_requested := string(payload)
	cmd := Command{
		Time:      0,
		Initiator: "localApp",
	}
	if command_requested == "start" {
		if self.State.State == paused_state {
			cmd.Command = "resume"
		} else {
			cmd.Command = "start"
		}
	}
	if command_requested == "stop" {
		cmd.Command = "stop"
	}
	if command_requested == "pause" {
		cmd.Command = "pause"
	}
	if command_requested == "return_to_base" {
		if self.State.State == cleaning_state {
			cmd.Command = "stop"
			data, _ := json.Marshal(cmd)
			self.HomeAssistant.MqttClient.Publish("cmd", data, 0, false)
			time.Sleep(15 * time.Second)
		}
		cmd.Command = "dock"
	}
	if command_requested == "locate" {
		log.Warn().Msg("locate")
		return
	}
	if command_requested == "clean_spot" {
		cmd.Command = "start"

		for i := range self.HomeAssistant.RegionSwitches {
			region_switch := self.HomeAssistant.RegionSwitches[i]
			twoPasses := false
			if strings.ToLower(string(self.HomeAssistant.CleanPassSelect.State)) == "two" {
				twoPasses = true
			}
			if region_switch.State {
				cmd.PmapId = region_switch.Map.Id
				cmd.Regions = append(cmd.Regions, RoombaRegion{
					RegionId: region_switch.Region.Id,
					Type:     region_switch.Region.Type,
					Params: RoombaRegionParams{
						NoAutoPasses: true,
						TwoPass:      twoPasses,
					},
				})
			}
		}
	}

	data, _ := json.Marshal(cmd)
	self.HomeAssistant.MqttClient.Publish("cmd", data, 0, false)
}
