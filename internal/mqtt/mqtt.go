package mqtt

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"time"

	"goschedule-listener/internal/config"
	"goschedule-listener/internal/db"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	client paho.Client
}

func New(cfg *config.Config) (*Client, error) {
	opts := paho.NewClientOptions().
		AddBroker(cfg.MQTTBroker).
		SetClientID(cfg.MQTTClientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(_ paho.Client) {
			log.Println("MQTT connected")
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Println("MQTT connection lost:", err)
		})

	if cfg.MQTTTLSEnabled() {
		tlsCfg, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("mqtt TLS config: %w", err)
		}
		opts.SetTLSConfig(tlsCfg)
		log.Println("MQTT TLS enabled")
	} else {
		log.Println("MQTT TLS disabled — connecting without certs")
	}

	c := paho.NewClient(opts)
	token := c.Connect()
	if token.WaitTimeout(10*time.Second) && token.Error() != nil {
		return nil, fmt.Errorf("mqtt connect: %w", token.Error())
	}

	return &Client{client: c}, nil
}

// buildTLSConfig parses raw PEM strings directly from env vars
func buildTLSConfig(cfg *config.Config) (*tls.Config, error) {
	// --- CA Certificate ---
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(cfg.MQTTCACert)) {
		return nil, fmt.Errorf("failed to parse CA certificate PEM — check ECS_MQTT_CA_CERTIFICATE")
	}

	// --- Client Certificate + Private Key ---
	// tls.X509KeyPair accepts raw PEM bytes directly
	clientCert, err := tls.X509KeyPair(
		[]byte(cfg.MQTTClientCert),
		[]byte(cfg.MQTTPrivateKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse client cert/key pair: %w", err)
	}

	return &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// validatePEM is a helper to catch malformed PEM early with a clear error
func validatePEM(label, pemStr string) error {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return fmt.Errorf("%s: not valid PEM — missing -----BEGIN ... ----- header", label)
	}
	return nil
}

func (c *Client) Publish(deviceName string, schedules []db.ScheduleEntry) error {
	topic := fmt.Sprintf("machine/schedule/%s", deviceName)

	// Always publish array, never null — machine needs empty list to clear schedule
	if schedules == nil {
		schedules = []db.ScheduleEntry{}
	}

	payload, err := json.Marshal(map[string]any{
		"schedules": schedules,
	})
	if err != nil {
		return err
	}

	token := c.client.Publish(topic, 1, false, payload)
	token.Wait()
	return token.Error()
}

func (c *Client) Disconnect() {
	c.client.Disconnect(500)
}
