package listener

import (
	"context"
	"fmt"
	"log"

	"goschedule-listener/internal/config"
	"goschedule-listener/internal/db"
	"goschedule-listener/internal/mqtt"
	supabase_realtime "goschedule-listener/internal/supabase-realtime"

	"go.uber.org/zap"
)

type Listener struct {
	rtClient *supabase_realtime.Client
	dbClient *db.Client
	mqClient *mqtt.Client
	cfg      *config.Config
	logger   *zap.Logger
}

func New(
	cfg *config.Config,
	dbClient *db.Client,
	mqClient *mqtt.Client,
	logger *zap.Logger,
) *Listener {
	// Pass empty projectRef — we override the URL directly below
	rtClient := supabase_realtime.CreateRealtimeClient("", cfg.DBSupabaseKey, logger)

	// Override the WebSocket URL built by the library with our own
	rtClient.Url = cfg.RealtimeWSURL()

	return &Listener{
		rtClient: rtClient,
		dbClient: dbClient,
		mqClient: mqClient,
		cfg:      cfg,
		logger:   logger,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	if err := l.rtClient.Connect(); err != nil {
		return fmt.Errorf("realtime connect: %w", err)
	}

	err := l.rtClient.ListenToPostgresChanges(supabase_realtime.PostgresChangesOptions{
		Schema: l.cfg.DBSchema,
		Table:  l.cfg.DBRealtimeTable,
		Filter: "*",
	}, l.handleChange)
	if err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}

	log.Printf("Listening on %s.%s ...", l.cfg.DBSchema, l.cfg.DBRealtimeTable)
	<-ctx.Done()
	log.Println("Shutting down listener...")
	l.rtClient.Disconnect()
	return nil
}

func (l *Listener) handleChange(payload map[string]any) {
	/*
	   payload
	     └── payload
	           └── data
	                 ├── type        → "INSERT" / "UPDATE" / "DELETE"
	                 ├── record      → new row (INSERT/UPDATE)
	                 └── old_record  → previous row (DELETE)
	*/

	// Structure: payload -> payload -> data -> record
	innerPayload, _ := payload["payload"].(map[string]any)
	data, _ := innerPayload["data"].(map[string]any)

	eventType, _ := data["type"].(string)

	// No more DELETE handling — UI sets duration_min = 0 instead
	if eventType == "DELETE" {
		log.Println("DELETE event ignored — use duration_min=0 to cancel schedules")
		return
	}

	record, _ := data["record"].(map[string]any)
	if record == nil {
		log.Printf("No record found for event [%s], skipping", eventType)
		return
	}

	deviceName, _ := record["device_name"].(string)
	if deviceName == "" {
		log.Printf("No device_name in record for event [%s], skipping", eventType)
		return
	}

	log.Printf("[%s] Change detected for device: %s", eventType, deviceName)

	schedules, err := l.dbClient.FetchFutureSchedules(context.Background(), deviceName)
	if err != nil {
		log.Println("DB fetch error:", err)
		return
	}

	if err := l.mqClient.Publish(deviceName, schedules); err != nil {
		log.Println("MQTT publish error:", err)
		return
	}

	log.Printf("[%s] Published %d schedule(s) for device [%s]", eventType, len(schedules), deviceName)
}
