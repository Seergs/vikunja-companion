package config

import "errors"

// Relay is the validated configuration for cmd/relay.
type Relay struct {
	ListenAddr  string
	APNS        APNS
	APNSSandbox bool // hit the APNs sandbox gateway instead of production
	DBPath      string
	LogLevel    string
}

// LoadRelay reads and validates the relay configuration from the environment.
func LoadRelay() (*Relay, error) {
	var errs []error
	push := func(_ string, err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	r := &Relay{
		ListenAddr:  get("RELAY_LISTEN_ADDR", ":8081"),
		APNSSandbox: boolVal("RELAY_APNS_SANDBOX"),
		DBPath:      get("RELAY_DB_PATH", "/data/relay.db"),
		LogLevel:    get("RELAY_LOG_LEVEL", "info"),
	}

	v, err := required("RELAY_APNS_KEY_PATH")
	push("", err)
	r.APNS.KeyPath = v

	v, err = required("RELAY_APNS_KEY_ID")
	push("", err)
	r.APNS.KeyID = v

	v, err = required("RELAY_APNS_TEAM_ID")
	push("", err)
	r.APNS.TeamID = v

	v, err = required("RELAY_APNS_TOPIC")
	push("", err)
	r.APNS.Topic = v

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return r, nil
}
