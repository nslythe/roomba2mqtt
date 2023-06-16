package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var cipher_suite []uint16 = []uint16{
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
	tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_RSA_WITH_RC4_128_SHA,
}

type MqttConfig struct {
	Broker   string
	Version  int
	Port     uint
	Username string
	Password string
}

type Message struct {
}

type SubscribeHandleFunction func(topic string, payload []byte)

type MqttClient interface {
	Connect() error
	Publish(topic string, payload []byte) error
	Subscribe(topic string, fnc SubscribeHandleFunction)
}

type MqttClientv5 struct {
	cm      *autopaho.ConnectionManager
	cfg     autopaho.ClientConfig
	fnc_map []SubscribeHandleFunction
}

type MqttClientv4 struct {
	client mqtt.Client
	opts   *mqtt.ClientOptions
}

func (self *MqttClientv5) message_handler(m *paho.Publish) {
	self.fnc_map[*m.Properties.SubscriptionIdentifier](m.Topic, m.Payload)
}

func (self *MqttClientv5) Connect() error {
	var err error
	self.cm, err = autopaho.NewConnection(context.Background(), self.cfg)
	if err != nil {
		return err
	}

	err = self.cm.AwaitConnection(context.Background())
	if err != nil {
		return err
	}

	return nil
}

func (self *MqttClientv5) Publish(topic string, payload []byte) error {
	_, err := self.cm.Publish(context.Background(), &paho.Publish{
		Topic:   topic,
		Payload: payload,
	})
	return err
}

func (self *MqttClientv5) Subscribe(topic string, fnc SubscribeHandleFunction) {
	id := len(self.fnc_map)
	self.cm.Subscribe(context.Background(),
		&paho.Subscribe{
			Properties: &paho.SubscribeProperties{
				SubscriptionIdentifier: &id,
			},
			Subscriptions: map[string]paho.SubscribeOptions{
				topic: {
					NoLocal: true,
				},
			},
		},
	)
	self.fnc_map = append(self.fnc_map, fnc)
}

func Connect5(config MqttConfig) (*MqttClientv5, error) {
	return_value := &MqttClientv5{}

	schema := "mqtt"
	if config.Port == 8883 {
		schema = "ssl"
	}
	return_value.cfg = autopaho.ClientConfig{
		BrokerUrls: []*url.URL{
			{
				Scheme: schema,
				Host:   fmt.Sprintf("%s:%d", config.Broker, config.Port),
			},
		},

		ClientConfig: paho.ClientConfig{
			ClientID: config.Username,
			Router:   paho.NewSingleHandlerRouter(return_value.message_handler),
		},
		TlsCfg: &tls.Config{
			CipherSuites:       cipher_suite,
			InsecureSkipVerify: true,
		},
	}
	return return_value, nil
}

func (self *MqttClientv4) Connect() error {
	if token := self.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	if !self.client.IsConnectionOpen() {
		return errors.New("Not connected")
	}
	return nil
}

func (self *MqttClientv4) Publish(topic string, payload []byte) error {
	token := self.client.Publish(topic, 0, false, payload)
	return token.Error()
}

func (self *MqttClientv4) Subscribe(topic string, fnc SubscribeHandleFunction) {
	self.client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
		fnc(msg.Topic(), msg.Payload())
	})
}

func Connect34(config MqttConfig) (*MqttClientv4, error) {
	return_value := &MqttClientv4{}

	return_value.opts = mqtt.NewClientOptions()
	schema := "tcp"
	if config.Port == 8883 {
		schema = "ssl"
	}
	return_value.opts.AddBroker(fmt.Sprintf("%s://%s:%d", schema, config.Broker, config.Port))
	return_value.opts.SetClientID(config.Username)
	return_value.opts.SetUsername(config.Username)
	return_value.opts.SetPassword(config.Password)
	return_value.opts.SetProtocolVersion(uint(config.Version))

	if config.Port == 8883 {
		tlsConfig := tls.Config{}
		//		tlsConfig.CipherSuites = append(tlsConfig.CipherSuites, uint16(client.Cipher))
		tlsConfig.CipherSuites = cipher_suite
		tlsConfig.InsecureSkipVerify = true
		return_value.opts.SetTLSConfig(&tlsConfig)
	}

	return_value.opts.AutoReconnect = true

	return_value.client = mqtt.NewClient(return_value.opts)

	return return_value, nil
}

func NewMqttClient(config MqttConfig) (MqttClient, error) {
	if config.Version >= 5 {
		return Connect5(config)
	} else {
		return Connect34(config)
	}
}