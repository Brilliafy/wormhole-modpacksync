package signaling

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// A WebRTC handshake needs one rendezvous point both peers can reach. The
// sender's embedded relay only works on a LAN/Tailnet, so for the default
// "WebRTC code" mode we use public MQTT brokers as the rendezvous: both peers
// dial out, exchange SDP on a topic derived from the session code, and then
// all file data flows peer-to-peer over the encrypted DataChannel. The broker
// only ever sees signalling messages (base64 SDP), never file contents.
//
// Set WORMHOLE_MQTT to a comma-separated list of broker URLs to use your own
// (for example "ssl://mqtt.example.com:8883").
var DefaultMQTTBrokers = []string{
	"ssl://broker.hivemq.com:8883",
	"ssl://broker.emqx.io:8883",
	"tcp://broker.hivemq.com:1883",
}

func mqttBrokers() []string {
	if v := strings.TrimSpace(os.Getenv("WORMHOLE_MQTT")); v != "" {
		brokers := make([]string, 0, 2)
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				brokers = append(brokers, p)
			}
		}
		if len(brokers) > 0 {
			return brokers
		}
	}
	return DefaultMQTTBrokers
}

// RoomSignaler is the rendezvous contract shared by the embedded/self-hosted
// HTTP relay and the public MQTT rendezvous.
type RoomSignaler interface {
	PublishOffer(ctx context.Context, code string, offerSDP string, directAddr string) error
	WaitForAnswer(ctx context.Context, code string) (string, error)
	FetchOffer(ctx context.Context, code string) (string, string, error)
	PublishAnswer(ctx context.Context, code string, answerSDP string) error
	Close()
}

// NewRoomSignaler selects the rendezvous backend for a session.
// Precedence: an explicit "<code>@relay" override, then WORMHOLE_RELAY, then
// WORMHOLE_SIGNAL=local (embedded relay only, LAN/Tailnet), otherwise the
// public MQTT rendezvous so plain codes work across different networks.
func NewRoomSignaler(relayOverride, code string) RoomSignaler {
	if strings.TrimSpace(relayOverride) != "" {
		return NewSignalingClient(relayOverride)
	}
	if strings.TrimSpace(os.Getenv("WORMHOLE_RELAY")) != "" {
		return NewSignalingClient("")
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("WORMHOLE_SIGNAL")), "local") {
		return NewSignalingClient("")
	}
	return NewMQTTClient(code)
}

const mqttTopicRoot = "wormhole-sync/v1"

// MQTTClient exchanges WebRTC signalling through a public MQTT broker.
type MQTTClient struct {
	brokers []string
	code    string

	mu       sync.Mutex
	client   mqtt.Client
	offer    string
	direct   string
	offerSet bool
	answers  chan string
	notify   chan struct{}
	closed   bool
}

type mqttOfferPayload struct {
	Offer      string `json:"offer"`
	DirectAddr string `json:"directAddr,omitempty"`
}

type mqttAnswerPayload struct {
	Answer string `json:"answer"`
}

func NewMQTTClient(code string) *MQTTClient {
	return &MQTTClient{
		brokers: mqttBrokers(),
		code:    code,
		answers: make(chan string, 1),
		notify:  make(chan struct{}, 1),
	}
}

func (c *MQTTClient) offerTopic() string  { return mqttTopicRoot + "/" + c.code + "/offer" }
func (c *MQTTClient) answerTopic() string { return mqttTopicRoot + "/" + c.code + "/answer" }

func randomSuffix() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func robustMQTTDialer(uri *url.URL, options mqtt.ClientOptions) (net.Conn, error) {
	d := &net.Dialer{Timeout: 5 * time.Second}
	tlsCfg := options.TLSConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{}
	} else {
		tlsCfg = tlsCfg.Clone()
	}
	if tlsCfg.ServerName == "" {
		tlsCfg.ServerName = uri.Hostname()
	}

	isTLS := uri.Scheme == "ssl" || uri.Scheme == "tls" || uri.Scheme == "tcps"

	// 1. Try dual-stack dial
	conn, err := d.Dial("tcp", uri.Host)
	if err == nil {
		if !isTLS {
			return conn, nil
		}
		tlsConn := tls.Client(conn, tlsCfg)
		if err = tlsConn.Handshake(); err == nil {
			return tlsConn, nil
		}
		conn.Close()
	}

	// 2. If dual-stack failed or TLS handshake reset on IPv6, fallback to pure IPv4
	conn4, err4 := d.Dial("tcp4", uri.Host)
	if err4 != nil {
		if err != nil {
			return nil, err
		}
		return nil, err4
	}
	if !isTLS {
		return conn4, nil
	}
	tlsConn4 := tls.Client(conn4, tlsCfg)
	if err4 = tlsConn4.Handshake(); err4 != nil {
		conn4.Close()
		return nil, err4
	}
	return tlsConn4, nil
}

// connect dials the first reachable broker. Subscriptions are (re)created by
// the OnConnect handler, which also covers automatic reconnects.
func (c *MQTTClient) connect(ctx context.Context) error {
	c.mu.Lock()
	if c.client != nil && c.client.IsConnectionOpen() {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	opts := mqtt.NewClientOptions()
	for _, b := range c.brokers {
		opts.AddBroker(b)
	}
	opts.SetClientID("wormhole-sync-" + randomSuffix())
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetCustomOpenConnectionFn(robustMQTTDialer)
	opts.SetOnConnectHandler(func(cl mqtt.Client) {
		cl.Subscribe(c.offerTopic(), 1, func(_ mqtt.Client, msg mqtt.Message) {
			var p mqttOfferPayload
			if err := json.Unmarshal(msg.Payload(), &p); err != nil {
				return
			}
			c.mu.Lock()
			c.offer = p.Offer
			c.direct = p.DirectAddr
			c.offerSet = true
			c.mu.Unlock()
			select {
			case c.notify <- struct{}{}:
			default:
			}
		})
		cl.Subscribe(c.answerTopic(), 1, func(_ mqtt.Client, msg mqtt.Message) {
			var p mqttAnswerPayload
			if err := json.Unmarshal(msg.Payload(), &p); err != nil || p.Answer == "" {
				return
			}
			select {
			case c.answers <- p.Answer:
			default:
				select {
				case <-c.answers:
				default:
				}
				c.answers <- p.Answer
			}
		})
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(20 * time.Second) {
		return fmt.Errorf("timed out connecting to rendezvous brokers %v", c.brokers)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("could not reach rendezvous brokers %v: %w", c.brokers, err)
	}

	c.mu.Lock()
	old := c.client
	c.client = client
	c.mu.Unlock()
	if old != nil {
		old.Disconnect(100)
	}
	return nil
}

// RoomSignalerLabel describes where the handshake will happen, for UI text.
func RoomSignalerLabel(relayOverride string) string {
	if strings.TrimSpace(relayOverride) != "" {
		return normalizeRelayURL(relayOverride)
	}
	if strings.TrimSpace(os.Getenv("WORMHOLE_RELAY")) != "" {
		return RelayURLFromEnv()
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("WORMHOLE_SIGNAL")), "local") {
		return RelayURLFromEnv()
	}
	return "public rendezvous (MQTT)"
}

func (c *MQTTClient) PublishOffer(ctx context.Context, code string, offerSDP string, directAddr string) error {
	if err := c.connect(ctx); err != nil {
		return err
	}

	payload, err := json.Marshal(mqttOfferPayload{Offer: offerSDP, DirectAddr: directAddr})
	if err != nil {
		return err
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	// Retained so a receiver that starts a few seconds later still finds it.
	token := client.Publish(c.offerTopic(), 1, true, payload)
	if !token.WaitTimeout(20 * time.Second) {
		return fmt.Errorf("timed out publishing offer to rendezvous broker")
	}
	return token.Error()
}

func (c *MQTTClient) WaitForAnswer(ctx context.Context, code string) (string, error) {
	if err := c.connect(ctx); err != nil {
		return "", err
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case answer := <-c.answers:
		return answer, nil
	}
}

func (c *MQTTClient) FetchOffer(ctx context.Context, code string) (string, string, error) {
	if err := c.connect(ctx); err != nil {
		return "", "", err
	}

	for {
		c.mu.Lock()
		if c.offerSet && (c.offer != "" || c.direct != "") {
			offer, direct := c.offer, c.direct
			c.mu.Unlock()
			return offer, direct, nil
		}
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-c.notify:
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *MQTTClient) PublishAnswer(ctx context.Context, code string, answerSDP string) error {
	if err := c.connect(ctx); err != nil {
		return err
	}

	payload, err := json.Marshal(mqttAnswerPayload{Answer: answerSDP})
	if err != nil {
		return err
	}

	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	token := client.Publish(c.answerTopic(), 1, false, payload)
	if !token.WaitTimeout(20 * time.Second) {
		return fmt.Errorf("timed out publishing answer to rendezvous broker")
	}
	return token.Error()
}

// Close disconnects and clears the retained offer so stale sessions do not
// linger on the public broker.
func (c *MQTTClient) Close() {
	c.mu.Lock()
	client := c.client
	c.client = nil
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()

	if client == nil {
		return
	}
	if client.IsConnectionOpen() {
		token := client.Publish(c.offerTopic(), 0, true, []byte{})
		token.WaitTimeout(2 * time.Second)
	}
	client.Disconnect(250)
}
