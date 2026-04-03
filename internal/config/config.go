package config

import (
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// DB connection
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBCACert   string

	// Realtime
	DBSchema        string
	DBRealtimeTable string
	DBForeignKey    string
	DBSupabaseKey   string
	DBRealtimeURL   string // https://xxxxxxxxxxxx.supabase.co

	// MQTT
	MQTTBroker     string
	MQTTClientID   string
	MQTTCACert     string // ECS_MQTT_CA_CERTIFICATE
	MQTTClientCert string // ECS_MQTT_CLIENT_CERTIFICATE
	MQTTPrivateKey string // ECS_MQTT_PRIVATE_KEY
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		DBHost:          os.Getenv("SUPABASE_DB_HOST"),
		DBPort:          os.Getenv("SUPABASE_DB_PORT"),
		DBUser:          os.Getenv("SUPABASE_DB_USER"),
		DBPassword:      os.Getenv("SUPABASE_DB_PASSWORD"),
		DBName:          os.Getenv("SUPABASE_DB_NAME"),
		DBCACert:        os.Getenv("SUPABASE_CA_CERT"),
		DBSchema:        os.Getenv("SUPABASE_REALTIME_SCHEMA"),
		DBRealtimeTable: os.Getenv("SUPABASE_REALTIME_TABLE"),
		DBForeignKey:    os.Getenv("SUPABASE_REALTIME_FOREIGN_KEY"),
		DBSupabaseKey:   os.Getenv("SUPABASE_KEY"),
		DBRealtimeURL:   os.Getenv("SUPABASE_URL"),
		MQTTBroker:      os.Getenv("MQTT_BROKER"),
		MQTTClientID:    os.Getenv("MQTT_CLIENT_ID"),
		MQTTCACert:      os.Getenv("ECS_MQTT_CA_CERTIFICATE"),
		MQTTClientCert:  os.Getenv("ECS_MQTT_CLIENT_CERTIFICATE"),
		MQTTPrivateKey:  os.Getenv("ECS_MQTT_PRIVATE_KEY"),
	}

	// Defaults
	if cfg.DBPort == "" {
		cfg.DBPort = "5432"
	}
	if cfg.DBUser == "" {
		cfg.DBUser = "postgres"
	}
	if cfg.DBName == "" {
		cfg.DBName = "postgres"
	}
	if cfg.DBSchema == "" {
		cfg.DBSchema = "public"
	}
	if cfg.MQTTClientID == "" {
		cfg.MQTTClientID = "schedule-listener"
	}

	// Required fields
	missing := []string{}
	if cfg.DBHost == "" {
		missing = append(missing, "SUPABASE_DB_HOST")
	}
	if cfg.DBPassword == "" {
		missing = append(missing, "SUPABASE_DB_PASSWORD")
	}
	if cfg.DBRealtimeTable == "" {
		missing = append(missing, "SUPABASE_REALTIME_TABLE")
	}
	if cfg.DBSupabaseKey == "" {
		missing = append(missing, "SUPABASE_KEY")
	}
	if cfg.DBRealtimeURL == "" {
		missing = append(missing, "SUPABASE_URL")
	}
	if cfg.MQTTBroker == "" {
		missing = append(missing, "MQTT_BROKER")
	}

	// TLS — all three or none
	tlsCount := 0
	if cfg.MQTTCACert != "" {
		tlsCount++
	}
	if cfg.MQTTClientCert != "" {
		tlsCount++
	}
	if cfg.MQTTPrivateKey != "" {
		tlsCount++
	}
	if tlsCount > 0 && tlsCount < 3 {
		missing = append(missing, "all three TLS vars required together: ECS_MQTT_CA_CERTIFICATE, ECS_MQTT_CLIENT_CERTIFICATE, ECS_MQTT_PRIVATE_KEY")
	}
	if tlsCount == 3 {
		for label, val := range map[string]string{
			"ECS_MQTT_CA_CERTIFICATE":     cfg.MQTTCACert,
			"ECS_MQTT_CLIENT_CERTIFICATE": cfg.MQTTClientCert,
			"ECS_MQTT_PRIVATE_KEY":        cfg.MQTTPrivateKey,
		} {
			block, _ := pem.Decode([]byte(val))
			if block == nil {
				missing = append(missing, fmt.Sprintf("%s is not valid PEM", label))
			}
		}
	}

	if len(missing) > 0 {
		log.Fatalf("Missing required env vars: %v", missing)
	}

	return cfg
}

// RealtimeWSURL builds the WebSocket URL directly from SUPABASE_URL
// https://xxxxxxxxxxxx.supabase.co → wss://xxxxxxxxxxxx.supabase.co/realtime/v1/websocket?...
func (c *Config) RealtimeWSURL() string {
	base := strings.TrimPrefix(c.DBRealtimeURL, "https://")
	base = strings.TrimPrefix(base, "http://")
	base = strings.TrimSuffix(base, "/")
	return fmt.Sprintf(
		"wss://%s/realtime/v1/websocket?apikey=%s&log_level=info&vsn=1.0.0",
		base, c.DBSupabaseKey,
	)
}

func (c *Config) MQTTTLSEnabled() bool {
	return c.MQTTCACert != "" && c.MQTTClientCert != "" && c.MQTTPrivateKey != ""
}

func (c *Config) DatabaseURL() string {
	if c.DBCACert != "" {
		return fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=verify-full sslrootcert=%s target_session_attrs=any",
			c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBCACert,
		)
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName,
	)
}
