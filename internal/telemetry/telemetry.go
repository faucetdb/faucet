package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	posthogEndpoint = "https://us.i.posthog.com/capture/"
	posthogKey      = "phx_158V8vJdOGiidyidTnIXsd4O4NzqvJZ2AbhzyRoIgBjbOKd"
	flushInterval   = 1 * time.Hour
	httpTimeout     = 3 * time.Second
)

// SettingsStore is the interface the telemetry package needs from the config store.
type SettingsStore interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Properties holds the telemetry payload sent to PostHog.
type Properties struct {
	Version    string   `json:"version"`
	GoVersion  string   `json:"go_version"`
	OS         string   `json:"os"`
	Arch       string   `json:"arch"`
	DBTypes    []string `json:"db_types"`
	Services   int      `json:"service_count"`
	Tables     int      `json:"table_count"`
	Admins     int      `json:"admin_count"`
	APIKeys    int      `json:"api_key_count"`
	Roles      int      `json:"role_count"`
	Features   []string `json:"features"`
	UptimeHrs  float64  `json:"uptime_hours"`
}

// PropertiesFunc is called each flush to gather current state.
type PropertiesFunc func() Properties

// Tracker manages anonymous telemetry reporting to PostHog.
type Tracker struct {
	instanceID string
	propsFn    PropertiesFunc
	client     *http.Client
	startedAt  time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a Tracker. It resolves (or generates) the instance ID from the
// settings store. Returns nil if telemetry is disabled via env var or settings.
func New(ctx context.Context, store SettingsStore, propsFn PropertiesFunc) *Tracker {
	// Check environment variable override first
	if envVal := os.Getenv("FAUCET_TELEMETRY"); envVal == "0" || envVal == "false" || envVal == "off" {
		return nil
	}

	// Check settings store
	if store != nil {
		val, err := store.GetSetting(ctx, "telemetry.enabled")
		if err == nil && (val == "false" || val == "0") {
			return nil
		}
	}

	// Resolve or generate instance ID
	instanceID := resolveInstanceID(ctx, store)

	return &Tracker{
		instanceID: instanceID,
		propsFn:    propsFn,
		client:     &http.Client{Timeout: httpTimeout},
		startedAt:  time.Now(),
	}
}

// Start begins the background telemetry loop. It sends an initial event
// immediately and then repeats every hour. Non-blocking.
func (t *Tracker) Start() {
	if t == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		// Initial capture
		t.flush()

		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				t.flush()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Shutdown stops the background loop and sends a final event.
func (t *Tracker) Shutdown() {
	if t == nil {
		return
	}
	if t.cancel != nil {
		t.cancel()
	}
	t.wg.Wait()
	// Final capture with latest state
	t.flush()
}

func (t *Tracker) flush() {
	props := t.propsFn()
	props.UptimeHrs = time.Since(t.startedAt).Hours()
	t.capture("server_heartbeat", props)
}

func (t *Tracker) capture(event string, props Properties) {
	payload := map[string]any{
		"api_key":              posthogKey,
		"event":                event,
		"distinct_id":          t.instanceID,
		"properties":           props,
		"timestamp":            time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return // fail silently
	}

	req, err := http.NewRequest("POST", posthogEndpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return // fail silently — network issues are expected
	}
	resp.Body.Close()
}

// resolveInstanceID loads or generates a persistent anonymous instance ID.
func resolveInstanceID(ctx context.Context, store SettingsStore) string {
	if store != nil {
		id, err := store.GetSetting(ctx, "instance_id")
		if err == nil && id != "" {
			return id
		}
	}

	id := uuid.New().String()

	if store != nil {
		_ = store.SetSetting(ctx, "instance_id", id)
	}
	return id
}

// PrintNotice prints the first-run telemetry notice to stderr.
func PrintNotice() {
	fmt.Fprintln(os.Stderr,
		"Anonymous usage stats are enabled to help improve Faucet.",
	)
	fmt.Fprintln(os.Stderr,
		"Disable with: faucet config set telemetry.enabled false  (or set FAUCET_TELEMETRY=0)",
	)
	fmt.Fprintln(os.Stderr,
		"See: https://github.com/faucetdb/faucet/blob/main/TELEMETRY.md",
	)
	fmt.Fprintln(os.Stderr)
}
