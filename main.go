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
	last_roomba_message  RoombaMessage
	attributes           *map[string]interface{}
	state                *VacuumState
	NeedUpdateConfig     bool
	NeedUpdateAttributes bool
	NeedUpdateState      bool
	available            bool
	ConnectionChannel    chan MqttClient
	SubscribeChannel     chan bool
}

var vacuum_client_list []*VacuumClient

var master_mqtt_config MqttConfig
var master_mqtt_client MqttClient

var master_mqtt_topic string = "vacuum"

const (
	cleaning_state  string = "cleaning"
	docked_state           = "docked"
	paused_state           = "paused"
	idle_state             = "idle"
	returning_state        = "returning"
	error_state            = "error"
)

var StateMap map[string]string = map[string]string{
	"run":       cleaning_state,
	"pause":     paused_state,
	"stop":      idle_state,
	"hmUsrDock": returning_state,
	"charge":    docked_state,
	"evac":      docked_state,
	"hmPostMsn": docked_state,
	"stuck":     error_state,
}

var DEBUG bool = false
var DEBUG_FOLDER = "/debug"

func (self *VacuumClient) UpdateRoombaMessage(msg RoombaMessage) {
	// Config
	if msg.State.Reported.Name != nil {
		self.HAConfig.Vacuum.Name = *msg.State.Reported.Name
		self.HAConfig.Vacuum.Device.Name = *msg.State.Reported.Name
		self.NeedUpdateConfig = true
	}

	if msg.State.Reported.SKU != nil {
		self.HAConfig.Vacuum.Device.Model = *msg.State.Reported.SKU
		self.NeedUpdateConfig = true
	}

	if msg.State.Reported.SoftwareVer != nil {
		self.HAConfig.Vacuum.Device.SwVersion = *msg.State.Reported.SoftwareVer
		self.NeedUpdateConfig = true
	}

	// Attrivbute
	(*self.attributes)["id"] = self.RoombaId
	(*self.attributes)["address"] = self.MqttConfig.Broker
	if msg.State.Reported.Bin != nil {
		(*self.attributes)["bin_present"] = msg.State.Reported.Bin.Present
		if !msg.State.Reported.Bin.Present {
			(*self.attributes)["error"] = "Bin is absent"
		}
		(*self.attributes)["bin_full"] = msg.State.Reported.Bin.Full
		self.NeedUpdateAttributes = true
	}
	if msg.State.Reported.TankLvl != nil {
		(*self.attributes)["tank_level"] = msg.State.Reported.TankLvl
		if *msg.State.Reported.TankLvl == 0 {
			(*self.attributes)["error"] = "Tank is empty"
		}
		self.NeedUpdateAttributes = true
	}
	if msg.State.Reported.LidOpen != nil {
		(*self.attributes)["lid_open"] = msg.State.Reported.LidOpen
		if *msg.State.Reported.LidOpen {
			(*self.attributes)["error"] = "Lid is open"
		}
		self.NeedUpdateAttributes = true
	}
	if msg.State.Reported.TankPresent != nil {
		(*self.attributes)["tank_present"] = msg.State.Reported.TankPresent
		if !*msg.State.Reported.TankPresent {
			(*self.attributes)["error"] = "Tank is absent"
		}
		self.NeedUpdateAttributes = true
	}
	if msg.State.Reported.DetectedPad != nil {
		(*self.attributes)["pad"] = msg.State.Reported.DetectedPad
		if *msg.State.Reported.DetectedPad == "invalid" {
			(*self.attributes)["error"] = "Pad invalid"
		}
		self.NeedUpdateAttributes = true
	}

	// State
	if msg.State.Reported.CleanMissionStatus != nil {
		self.state.State = StateMap[msg.State.Reported.CleanMissionStatus.Phase]
		self.NeedUpdateState = true
		if msg.State.Reported.CleanMissionStatus.Phase == "stuck" {
			(*self.attributes)["error"] = "Stuck"
		}
	}
	if msg.State.Reported.BatteryPercent != nil {
		self.state.BatteryLevel = *msg.State.Reported.BatteryPercent
		self.NeedUpdateState = true
	}
	if val, ok := (*self.attributes)["error"]; ok && val != "" {
		self.state.State = "error"
		self.NeedUpdateState = true
	}

	self.last_roomba_message = msg
}

func (self VacuumClient) HandleRoombaMessage(msg RoombaMessage) {
	self.UpdateRoombaMessage(msg)
	self.available = true

	// Config
	if self.NeedUpdateConfig {
		self.SendConfig()
	}

	// Attributes
	if self.NeedUpdateAttributes {
		self.SendAttributes()
	}

	// State
	if self.NeedUpdateState {
		self.SendState()
	}

	if self.available {
		master_mqtt_client.Publish(self.HAConfig.Vacuum.AvailabilityTopic, []byte("online"), false)
	} else {
		master_mqtt_client.Publish(self.HAConfig.Vacuum.AvailabilityTopic, []byte("offline"), false)
	}
}

func (self *VacuumClient) SendState() {
	data, _ := json.Marshal(self.state)
	master_mqtt_client.Publish(self.HAConfig.Vacuum.StateTopic, data, false)
}

func (self *VacuumClient) SendAttributes() {
	data, _ := json.Marshal(self.attributes)
	master_mqtt_client.Publish(self.HAConfig.Vacuum.JsonAttributesTopic, data, false)
	self.NeedUpdateAttributes = false
}

func (self *VacuumClient) SendConfig() {
	data, err := json.Marshal(self.HAConfig.Vacuum)
	if err != nil {
		panic(err)
	}

	master_mqtt_client.Publish(self.ConfigTopic, data, true)
	self.NeedUpdateConfig = false
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
				f.WriteString(topic)
				f.WriteString("      ")
				f.Write(payload)
				f.WriteString("\n")
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
			self.HandleRoombaMessage(msg)
		}

		dst_topic := topic
		if roombaId == "" {
			dst_topic = fmt.Sprintf("$aws/things/%s/%s", self.RoombaId, dst_topic)
		}
		dst_topic = master_mqtt_topic + dst_topic
		master_mqtt_client.Publish(dst_topic, payload, false)
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
		if self.state.State == paused_state {
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
		if self.state.State == cleaning_state {
			cmd.Command = "stop"
			data, _ := json.Marshal(cmd)
			self.Publish("cmd", data, false)
			time.Sleep(5 * time.Second)
		}
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
	self.Publish("cmd", data, false)
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

		client := &VacuumClient{
			ConnectionChannel: make(chan MqttClient),
			SubscribeChannel:  make(chan bool),
			state:             &VacuumState{},
			attributes:        &map[string]interface{}{},
		}
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
		go ConnectToRoomba(vacuum_client_list[i].MqttConfig, vacuum_client_list[i].ConnectionChannel)
	}

	// Subscribe to roomba
	for i := range vacuum_client_list {
		go SubscribeToRoomba(vacuum_client_list[i], vacuum_client_list[i].SubscribeChannel)
	}

	// wait for ready
	for i := range vacuum_client_list {
		<-vacuum_client_list[i].SubscribeChannel
	}

	<-stop_channel
}

func SubscribeToRoomba(client *VacuumClient, subscribe_channel chan bool) {
	mqtt_client := <-client.ConnectionChannel
	client.MqttClient = mqtt_client

	client.Subscribe("#", client.VacuumHandleMessage)
	master_mqtt_client.Subscribe(client.HAConfig.Vacuum.CommandTopic, client.CommandHandler)

	subscribe_channel <- true
}

func ConnectToRoomba(config MqttConfig, connection_channel chan MqttClient) {
	timing := []time.Duration{
		10 * time.Second,
		30 * time.Second,
		1 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
	}
	timing_idx := 0
	for {
		log.Info().Str("address", config.Broker).Msg("Roomba MQTT connect")

		wait_time := timing[timing_idx]
		timing_idx++
		if timing_idx >= len(timing) {
			timing_idx = timing_idx - 1
		}

		client, err := NewMqttClient(config)
		if err != nil {
			log.Error().Err(err).Str("wait_time", wait_time.String()).Msg("Roomba MQTT connection")
			time.Sleep(wait_time)
			continue
		}
		err = client.Connect()
		if err != nil {
			log.Error().Err(err).Str("wait_time", wait_time.String()).Msg("Roomba MQTT connection")
			time.Sleep(wait_time)
			continue
		}

		log.Info().Msg("Roomba MQTT connected")

		connection_channel <- client
		return
	}
}
