package relay

import (
	"camera-appliance/camera-manager/internal/streamrouting"
)

type ManagedRelay struct {
	streamrouting.RelayDefinition
	Endpoints []RelayEndpoint `json:"endpoints"`
}

type RelayEndpoint struct {
	DeviceID   string `json:"device_id"`
	SlotID     string `json:"slot_id,omitempty"`
	Label      string `json:"label,omitempty"`
	LocalHost  string `json:"local_host,omitempty"`
	LocalPort  string `json:"local_port"`
	BindHost   string `json:"bind_host"`
	HealthHost string `json:"health_host"`
	TargetHost string `json:"target_host"`
	TargetPort string `json:"target_port"`
}

type RelayEndpointStatus struct {
	DeviceID   string `json:"device_id"`
	SlotID     string `json:"slot_id,omitempty"`
	Label      string `json:"label,omitempty"`
	LocalHost  string `json:"local_host,omitempty"`
	LocalPort  string `json:"local_port"`
	BindHost   string `json:"bind_host"`
	HealthHost string `json:"health_host"`
	TargetHost string `json:"target_host"`
	TargetPort string `json:"target_port"`
	State      string `json:"state"`
	Message    string `json:"message"`
}

type Status struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Type         string                `json:"type"`
	Host         string                `json:"host"`
	BindHost     string                `json:"bind_host"`
	SSHTarget    string                `json:"ssh_target,omitempty"`
	AutoStart    bool                  `json:"auto_start"`
	Enabled      bool                  `json:"enabled"`
	PID          int                   `json:"pid,omitempty"`
	ProcessState string                `json:"process_state"`
	Message      string                `json:"message"`
	Started      bool                  `json:"started,omitempty"`
	LastError    string                `json:"last_error,omitempty"`
	BackoffUntil string                `json:"backoff_until,omitempty"`
	LogPath      string                `json:"log_path,omitempty"`
	Endpoints    []RelayEndpointStatus `json:"endpoints"`
}
