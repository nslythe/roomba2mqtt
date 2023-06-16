package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"encoding/json"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type VacuumClient struct {
	RoombaId string
	MqttConfig
	MqttClient
	HAConfig
}

var vacuum_client_list []*VacuumClient

var master_mqtt_config MqttConfig
var master_mqtt_client MqttClient

var master_mqtt_topic string = "vacuum"

var StateMap map[string]string = map[string]string{
	"run":    "cleaning",
	"charge": "docked",
	"pause":  "paused",
	"stop":   "idle",
	"home":   "returning",
	"error":  "error",
}

var DEBUG bool = false
var DEBUG_FOLDER = "/debug"

func (self VacuumClient) Update(msg RoombaMessage) {
	// Avaibality
	available := true

	// Config
	if msg.State.Reported.Name != "" {
		updated_config := false

		if self.HAConfig.Vacuum.Name != msg.State.Reported.Name {
			self.HAConfig.Vacuum.Name = msg.State.Reported.Name
			self.HAConfig.Vacuum.Device.Name = msg.State.Reported.Name
			updated_config = true
		}
		if self.HAConfig.Vacuum.Device.Model != msg.State.Reported.SKU {
			self.HAConfig.Vacuum.Device.Model = msg.State.Reported.SKU
			updated_config = true
		}
		if self.HAConfig.Vacuum.Device.SwVersion != msg.State.Reported.SoftwareVer {
			self.HAConfig.Vacuum.Device.SwVersion = msg.State.Reported.SoftwareVer
			updated_config = true
		}
		if updated_config {
			self.SendConfig()
		}
	}

	// Attributes
	var attributes map[string]interface{} = make(map[string]interface{})
	if msg.State.Reported.Bin != nil {
		attributes["bin_present"] = msg.State.Reported.Bin.Present
		if !msg.State.Reported.Bin.Present {
			attributes["error"] = "Bin is absent"
		}
		attributes["bin_full"] = msg.State.Reported.Bin.Full
		if msg.State.Reported.Bin.Full {
			attributes["error"] = "Bin is full"
		}
	}
	if msg.State.Reported.TankLvl != nil {
		attributes["tank_level"] = msg.State.Reported.TankLvl
		if *msg.State.Reported.TankLvl == 0 {
			attributes["error"] = "Tank is empty"
		}
	}
	if msg.State.Reported.LidOpen != nil {
		attributes["lid_open"] = msg.State.Reported.LidOpen
		if *msg.State.Reported.LidOpen {
			attributes["error"] = "Lid is open"
		}
	}
	if msg.State.Reported.TankPresent != nil {
		attributes["tank_present"] = msg.State.Reported.TankPresent
		if !*msg.State.Reported.TankPresent {
			attributes["error"] = "Tank is absent"
		}
	}
	if msg.State.Reported.DetectedPad != nil {
		attributes["pad"] = msg.State.Reported.DetectedPad
		if *msg.State.Reported.DetectedPad == "invalid" {
			attributes["error"] = "Pad invalid"
		}
	}

	data, _ := json.Marshal(attributes)
	master_mqtt_client.Publish(self.HAConfig.Vacuum.JsonAttributesTopic, data)

	// State
	if msg.State.Reported.CleanMissionStatus.Phase != "" {
		state := VacuumState{
			State:        StateMap[msg.State.Reported.CleanMissionStatus.Phase],
			BatteryLevel: msg.State.Reported.BatteryPercent,
		}

		if val, ok := attributes["error"]; ok && val != "" {
			state.State = "error"
		}

		data, _ := json.Marshal(state)
		master_mqtt_client.Publish(self.HAConfig.Vacuum.StateTopic, data)
	}

	if available {
		master_mqtt_client.Publish(self.HAConfig.Vacuum.AvailabilityTopic, []byte("online"))
	} else {
		master_mqtt_client.Publish(self.HAConfig.Vacuum.AvailabilityTopic, []byte("offline"))
	}

}

func (self *VacuumClient) SendConfig() {
	data, err := json.Marshal(self.HAConfig.Vacuum)
	if err != nil {
		panic(err)
	}

	master_mqtt_client.Publish(self.ConfigTopic, data)
}

func (self VacuumClient) VacuumHandleMessage(topic string, payload []byte) {
	roombaId := ""
	if strings.HasPrefix(topic, "$aws") {
		roombaId = strings.Split(topic, "/")[2]
	}

	if _, err := os.Stat(DEBUG_FOLDER); !os.IsNotExist(err) {
		if roombaId != "" {
			f, err := os.OpenFile(path.Join(DEBUG_FOLDER, roombaId), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				defer f.Close()
				f.WriteString("\n============================\n")
				f.Write(payload)
			} else {
				log.Error().Err(err).Msg("save payload to file")
			}
		}
	}

	if roombaId != "" {
		self.RoombaId = roombaId
		msg := RoombaMessage{}
		err := json.Unmarshal(payload, &msg)
		if err != nil {
			log.Error().Err(err).Msg("Message received from roomba")
		} else {
			self.Update(msg)
		}

		dst_topic := topic
		if roombaId == "" {
			dst_topic = fmt.Sprintf("$aws/things/%s/%s", self.RoombaId, dst_topic)
		}
		dst_topic = master_mqtt_topic + dst_topic
		master_mqtt_client.Publish(dst_topic, payload)
		log.Info().Str("broker", self.Broker).Str("dst_topic", dst_topic).Msg("mapping message")
	}
}

func NewMqttConfig(id int) (MqttConfig, error) {

	ROOMBA_ADDRESS := fmt.Sprintf("%d_ROOMBA_ADDRESS", id)
	ROOMBA_USER := fmt.Sprintf("%d_ROOMBA_USER", id)
	ROOMBA_PASSWORD := fmt.Sprintf("%d_ROOMBA_PASSWORD", id)

	if _, ok := os.LookupEnv(ROOMBA_ADDRESS); !ok {
		return MqttConfig{}, errors.New("No env var")
	}
	if _, ok := os.LookupEnv(ROOMBA_USER); !ok {
		return MqttConfig{}, errors.New("No env var")
	}
	if _, ok := os.LookupEnv(ROOMBA_PASSWORD); !ok {
		return MqttConfig{}, errors.New("No env var")
	}

	return MqttConfig{
		Broker:   os.Getenv(ROOMBA_ADDRESS),
		Port:     8883,
		Username: os.Getenv(ROOMBA_USER),
		Password: os.Getenv(ROOMBA_PASSWORD),
		Version:  0,
	}, nil
}

type Command struct {
	Command   string `json:"command"`
	Time      int    `json:"time"`
	Initiator string `json:"initiator"`
}

func (self VacuumClient) CommandHandler(topic string, payload []byte) {
	command_requested := string(payload)
	cmd := Command{
		Time:      0,
		Initiator: "localApp",
	}
	if command_requested == "start" {
		cmd.Command = "start"
	}
	if command_requested == "stop" {
		cmd.Command = "stop"
	}
	if command_requested == "pause" {
		cmd.Command = "pause"
	}
	if command_requested == "return_to_base" {
		cmd.Command = "dock"
	}
	if command_requested == "locate" {
		log.Warn().Msg("locate")
		return
	}
	if command_requested == "clean_spot" {
		log.Warn().Msg("clean_spot")
		return
	}

	data, _ := json.Marshal(cmd)

	self.Publish("cmd", data)
}

func main() {
	debug_str := os.Getenv("DEBUG")
	DEBUG, _ = strconv.ParseBool(debug_str)
	p, found := os.LookupEnv("DEBUG_FOLDER")
	if found {
		DEBUG_FOLDER = p
	}

	if DEBUG {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().In(time.Local)
	}

	log.Info().Msg("Started")

	signal_channel := make(chan os.Signal, 2)
	stop_channel := make(chan bool, 2)
	signal.Notify(signal_channel, os.Interrupt, syscall.SIGTERM)
	go func(signal_channel chan os.Signal) {
		<-signal_channel
		stop_channel <- true
	}(signal_channel)

	var err error

	// configure roomba
	for i := 0; i < 10; i++ {
		log.Debug().Int("index", i).Msg("env variable loading")

		client := &VacuumClient{}
		client.MqttConfig, err = NewMqttConfig(i)
		if err != nil {
			break
		}
		client.HAConfig = ConfigureHomeAssistant(client.MqttConfig.Username)
		vacuum_client_list = append(vacuum_client_list, client)
		log.Info().Int("index", i).Msg("env variable loaded")
	}

	// master mqtt client
	port, _ := strconv.Atoi(os.Getenv("MQTT_PORT"))
	if port == 0 {
		port = 1883
	}
	master_mqtt_config = MqttConfig{
		Broker:   os.Getenv("MQTT_ADDRESS"),
		Port:     uint(port),
		Username: os.Getenv("MQTT_USER"),
		Password: os.Getenv("MQTT_PASSWORD"),
		Version:  5,
	}
	master_mqtt_client, err = NewMqttClient(master_mqtt_config)
	if err != nil {
		log.Error().Err(err).Msg("master MQTT connection")
		panic(err)
	}
	err = master_mqtt_client.Connect()
	if err != nil {
		log.Error().Err(err).Msg("master MQTT connection")
		panic(err)
	}
	log.Info().Msg("master MQTT connected")

	// connect to roomba
	for i := range vacuum_client_list {
		vacuum_client_list[i].MqttClient, err = NewMqttClient(vacuum_client_list[i].MqttConfig)
		if err != nil {
			log.Error().Err(err).Msg("Roomba MQTT connection")
		}
		err = vacuum_client_list[i].Connect()
		if err != nil {
			log.Error().Err(err).Msg("Roomba connection")
		}
		log.Info().Msg("Roomba MQTT connection")

		vacuum_client_list[i].Subscribe("#", vacuum_client_list[i].VacuumHandleMessage)
		master_mqtt_client.Subscribe(vacuum_client_list[i].HAConfig.Vacuum.CommandTopic, vacuum_client_list[i].CommandHandler)
	}

	<-stop_channel
}
