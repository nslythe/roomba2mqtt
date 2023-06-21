package main

import (
	"errors"
	"fmt"
	"io/ioutil"
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

type Region struct {
	Id   string
	Name string
	Type string
}

type Map struct {
	Name    string
	Id      string
	Regions map[string]*Region
}

type Client struct {
	Version  int
	RoombaId string
	MqttConfig
	HomeAssistant
	ConnectionChannel chan MqttClient `json:"-"`
	SubscribeChannel  chan bool       `json:"-"`
	Maps              []*Map
}

var vacuum_client_list []*Client

var master_mqtt_config MqttConfig
var master_mqtt_client MqttClient

var master_mqtt_topic string = "roomba2mqtt"

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
var DATA_FOLDER = "/data"

func (self *Client) UpdateRoombaMessage(msg RoombaMessage) {
	// Config Vacuum
	if msg.State.Reported.Name != nil {
		self.HomeAssistant.Vacuum.Config.Name = *msg.State.Reported.Name
		self.HomeAssistant.Vacuum.Config.Device.Name = *msg.State.Reported.Name
		self.Vacuum.NeedSendConfig = true
	}

	if msg.State.Reported.SKU != nil {
		self.HomeAssistant.Vacuum.Config.Device.Model = *msg.State.Reported.SKU
		self.Vacuum.NeedSendConfig = true
	}

	if msg.State.Reported.SoftwareVer != nil {
		self.HomeAssistant.Vacuum.Config.Device.SwVersion = *msg.State.Reported.SoftwareVer
		self.Vacuum.NeedSendConfig = true
	}

	// Map & region
	if msg.State.Reported.Maps != nil {
		for i := range *msg.State.Reported.Maps {
			for map_id := range (*msg.State.Reported.Maps)[i] {
				map_name := (*msg.State.Reported.Maps)[i][map_id]

				var current_map *Map
				for m := range self.Maps {
					if self.Maps[m].Name == map_name {
						current_map = self.Maps[m]
						break
					}
					if self.Maps[m].Id == map_id {
						current_map = self.Maps[m]
						break
					}

				}
				if current_map == nil {
					current_map = &Map{
						Id:      map_id,
						Name:    map_name,
						Regions: make(map[string]*Region),
					}
					self.Maps = append(self.Maps, current_map)
				}
			}
		}
	}

	// Region
	if msg.State.Reported.LastCommand != nil {
		map_id := msg.State.Reported.LastCommand.PmapId
		if map_id != "" {
			var curren_map *Map
			for m := range self.Maps {
				if self.Maps[m].Id == map_id {
					curren_map = self.Maps[m]
					break
				}
			}
			if curren_map != nil {
				for r := range msg.State.Reported.LastCommand.Regions {
					region := msg.State.Reported.LastCommand.Regions[r]
					curren_map.Regions[region.RegionId] = &Region{
						Id:   region.RegionId,
						Type: region.Type,
					}
				}
			}
		}
	}

	//Create switch for each zone
	for m := range self.Maps {
		for r := range self.Maps[m].Regions {
			found := false
			for i := range self.HomeAssistant.RegionSwitches {
				if self.HomeAssistant.RegionSwitches[i].Region == self.Maps[m].Regions[r] {
					found = true
					break
				}
			}
			if !found {
				icon := "mdi:texture-box"
				if self.Maps[m].Regions[r].Type == "zid" {
					icon = "mdi:texture"
				}
				s := self.HomeAssistant.ConfigureRoombaRegionSwitch(self.RoombaId,
					self.Maps[m].Regions[r].Type+self.Maps[m].Regions[r].Id,
					self.Vacuum.Config.Device,
					icon)
				s.Region = self.Maps[m].Regions[r]
				s.Map = self.Maps[m]
			}
		}
	}

	// Attrivbute
	delete(self.Vacuum.Attributes, "error")
	self.Vacuum.Attributes["id"] = self.RoombaId
	self.Vacuum.Attributes["address"] = self.MqttConfig.Broker
	self.Vacuum.Attributes["length_maps"] = len(self.Maps)
	if DEBUG {
		i := 0
		for m := range self.Maps {
			self.Vacuum.Attributes["map "+strconv.Itoa(i)] = self.Maps[m].Id
			for r := range self.Maps[m].Regions {
				self.Vacuum.Attributes["map_"+strconv.Itoa(i)+"_room_"+r] = self.Maps[m].Regions[r].Id
			}
			i++
		}
		self.Vacuum.NeedSendAttributes = true
	}

	if msg.State.Reported.Bin != nil {
		self.Vacuum.Attributes["bin_present"] = msg.State.Reported.Bin.Present
		if !msg.State.Reported.Bin.Present {
			self.Vacuum.Attributes["error"] = "Bin is absent"
		}
		self.Vacuum.Attributes["bin_full"] = msg.State.Reported.Bin.Full
		self.Vacuum.NeedSendAttributes = true
	}
	if msg.State.Reported.TankLvl != nil {
		self.Vacuum.Attributes["tank_level"] = msg.State.Reported.TankLvl
		if *msg.State.Reported.TankLvl == 0 {
			self.Vacuum.Attributes["error"] = "Tank is empty"
		}
		self.Vacuum.NeedSendAttributes = true
	}
	if msg.State.Reported.LidOpen != nil {
		self.Vacuum.Attributes["lid_open"] = msg.State.Reported.LidOpen
		if *msg.State.Reported.LidOpen {
			self.Vacuum.Attributes["error"] = "Lid is open"
		}
		self.Vacuum.NeedSendAttributes = true
	}
	if msg.State.Reported.TankPresent != nil {
		self.Vacuum.Attributes["tank_present"] = msg.State.Reported.TankPresent
		if !*msg.State.Reported.TankPresent {
			self.Vacuum.Attributes["error"] = "Tank is absent"
		}
		self.Vacuum.NeedSendAttributes = true
	}
	if msg.State.Reported.DetectedPad != nil {
		self.Vacuum.Attributes["pad"] = msg.State.Reported.DetectedPad
		if *msg.State.Reported.DetectedPad == "invalid" {
			self.Vacuum.Attributes["error"] = "Pad invalid"
		}
		self.Vacuum.NeedSendAttributes = true
	}

	// State
	if msg.State.Reported.CleanMissionStatus != nil {
		self.Vacuum.State.State = StateMap[msg.State.Reported.CleanMissionStatus.Phase]
		self.Vacuum.NeedSendState = true
		if msg.State.Reported.CleanMissionStatus.Phase == "stuck" {
			self.Vacuum.Attributes["error"] = "Stuck"
		}
	}
	if msg.State.Reported.BatteryPercent != nil {
		self.Vacuum.State.BatteryLevel = *msg.State.Reported.BatteryPercent
		self.Vacuum.NeedSendState = true
	}
	if val, ok := self.Vacuum.Attributes["error"]; ok && val != "" {
		self.Vacuum.State.State = "error"
		self.Vacuum.NeedSendState = true
	}
}

func (self *Client) VacuumHandleMessage(topic string, payload []byte) {
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
		if self.RoombaId == "" {
			self.RoombaId = roombaId
			self.Load(DATA_FOLDER)
			if DEBUG {
				self.HomeAssistant.ConfigureCleanPassSelect(self.RoombaId,
					"clean_pass",
					self.Vacuum.Config.Device,
					"mdi:spray-bottle",
					[]string{
						"One",
						"Two",
					})

			}
		}
		msg := RoombaMessage{}
		err := json.Unmarshal(payload, &msg)
		if err != nil {
			log.Error().Err(err).Msg("Message received from roomba")
		} else {
			self.UpdateRoombaMessage(msg)
			self.Save(DATA_FOLDER)
			self.HomeAssistant.SendUpdate()
		}

		dst_topic := topic
		dst_topic = path.Join(master_mqtt_topic, "raw", dst_topic)
		master_mqtt_client.Publish(dst_topic, payload, 2, false)
		log.Info().Str("broker", self.Broker).Str("dst_topic", dst_topic).Msg("mapping message")
	}
}

func (self *Client) Save(data_dir string) string {
	file_name := path.Join(data_dir, self.RoombaId+".json")

	log.Info().Str("file_name", file_name).Msg("Saving vacuum")

	os.MkdirAll(data_dir, 0755)

	data, err := json.Marshal(self.Maps)
	if err != nil {
		log.Error().Err(err).Msg("Saving vacuum")
	} else {
		err = ioutil.WriteFile(file_name, data, 0644)
		if err != nil {
			log.Error().Err(err).Msg("Saving vacuum")
		}
		return file_name
	}
	return ""
}

func (self *Client) Load(data_dir string) {
	file_name := path.Join(data_dir, self.RoombaId+".json")

	log.Info().Str("file_name", file_name).Msg("Loading vacuum")

	data, err := ioutil.ReadFile(file_name)
	if err != nil {
		log.Error().Err(err).Msg("Saving vacuum")
	} else {
		json.Unmarshal(data, &self.Maps)
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

func main() {
	//GetCredential()
	debug_str := os.Getenv("DEBUG")
	DEBUG, _ = strconv.ParseBool(debug_str)
	p, found := os.LookupEnv("DEBUG_FOLDER")
	if found {
		DEBUG_FOLDER = p
	}
	p, found = os.LookupEnv("DATA_FOLDER")
	if found {
		DATA_FOLDER = p
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

	// configure roomba
	for i := 0; i < 10; i++ {
		log.Debug().Int("index", i).Msg("env variable loading")

		client := &Client{
			ConnectionChannel: make(chan MqttClient),
			SubscribeChannel:  make(chan bool),
			Maps:              []*Map{},
		}
		client.MqttConfig, err = NewMqttConfig(i)
		if err != nil {
			break
		}
		client.HomeAssistant = ConfigureHomeAssistant(master_mqtt_topic, master_mqtt_client)
		client.HomeAssistant.ConfigureVacuum(client.MqttConfig.Username)

		vacuum_client_list = append(vacuum_client_list, client)
		log.Info().Int("index", i).Msg("env variable loaded")
	}

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

func SubscribeToRoomba(client *Client, subscribe_channel chan bool) {
	mqtt_client := <-client.ConnectionChannel

	client.Vacuum.HomeAssistant.MqttClient = mqtt_client

	client.Vacuum.HomeAssistant.MqttClient.Subscribe("#", client.VacuumHandleMessage)

	master_mqtt_client.Subscribe(client.HomeAssistant.Vacuum.Config.CommandTopic, client.Vacuum.CommandHandler)

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
