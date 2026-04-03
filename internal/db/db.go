package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net"
	"time"

	"goschedule-listener/internal/config"

	"github.com/lib/pq"
)

type ScheduleEntry struct {
	Start    int64 `json:"start"`
	Duration int   `json:"duration"`
}

type Client struct {
	db *sql.DB
}

// ipv4Dialer forces TCP4 to avoid IPv6 on environments that don't support it
type ipv4Dialer struct{}

func (d ipv4Dialer) Dial(network, addr string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(
		context.Background(), "tcp4", addr,
	)
}

func (d ipv4Dialer) DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	return (&net.Dialer{Timeout: timeout}).DialContext(
		context.Background(), "tcp4", addr,
	)
}

func New(cfg *config.Config) (*Client, error) {
	// Register a custom driver that forces IPv4
	sql.Register("postgres-ipv4", &pq.Driver{})

	connector, err := pq.NewConnector(cfg.DatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("db connector: %w", err)
	}

	// Wrap connector with IPv4-only dialer
	db := sql.OpenDB(&ipv4Connector{connector: connector})
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}

	return &Client{db: db}, nil
}

// ipv4Connector wraps pq.Connector to inject IPv4-only dialing
type ipv4Connector struct {
	connector driver.Connector
}

func (c *ipv4Connector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.connector.Connect(ctx)
}

func (c *ipv4Connector) Driver() driver.Driver {
	return c.connector.Driver()
}

func (c *Client) Close() { c.db.Close() }

func (c *Client) FetchFutureSchedules(ctx context.Context, deviceName string) ([]ScheduleEntry, error) {
	query := `
		SELECT start_time, duration_min
		FROM schedules
		WHERE (start_time + (duration_min * interval '1 minute')) > NOW()
		  AND duration_min > 0
		  AND device_name = $1
		ORDER BY start_time ASC
	`

	rows, err := c.db.QueryContext(ctx, query, deviceName)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var entries []ScheduleEntry
	for rows.Next() {
		var startTime time.Time
		var durationMin int
		if err := rows.Scan(&startTime, &durationMin); err != nil {
			continue
		}
		entries = append(entries, ScheduleEntry{
			Start:    startTime.Unix(),
			Duration: durationMin,
		})
	}

	return entries, nil
}
