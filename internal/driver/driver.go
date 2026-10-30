// Package driver defines attributed facts from read-only device observations.
package driver

type Params struct {
	Destination, Source, VRF string
	Interfaces               []string
	Sudo                     bool
}
type Command struct {
	Name   string `json:"name"`
	Group  string `json:"group"`
	Text   string `json:"command"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type Driver interface {
	Platform() string
	Plan([]string, Params) ([]Command, error)
	Validate(string, Params) error
	Parse(string, []byte) (Facts, error)
}
type Address struct {
	Interface    string `json:"interface"`
	Address      string `json:"address"`
	PrefixLength *int   `json:"prefix_length"`
	Master       string `json:"master"`
	Evidence     string `json:"evidence"`
}
type NextHop struct {
	Address   string `json:"address"`
	Interface string `json:"interface"`
	Active    *bool  `json:"active"`
	Installed *bool  `json:"installed"`
	Evidence  string `json:"evidence"`
}
type Route struct {
	Prefix    string    `json:"prefix"`
	Protocol  string    `json:"protocol"`
	Context   string    `json:"context"`
	Selected  *bool     `json:"selected"`
	Installed *bool     `json:"installed"`
	NextHops  []NextHop `json:"next_hops"`
	Evidence  string    `json:"evidence"`
}
type Counter struct {
	Interface string  `json:"interface"`
	State     string  `json:"state"`
	MTUBytes  *uint64 `json:"mtu_bytes"`
	RXErrors  *uint64 `json:"rx_errors"`
	TXErrors  *uint64 `json:"tx_errors"`
	RXDrops   *uint64 `json:"rx_drops"`
	TXDrops   *uint64 `json:"tx_drops"`
	SpeedBPS  *uint64 `json:"speed_bps"`
	Evidence  string  `json:"evidence"`
}
type Neighbor struct {
	Interface           string   `json:"interface"`
	Name                string   `json:"name"`
	ChassisID           string   `json:"chassis_id"`
	PortID              string   `json:"port_id"`
	ManagementAddresses []string `json:"management_addresses"`
	Evidence            string   `json:"evidence"`
}
type Version struct {
	OS       string `json:"os"`
	Version  string `json:"version"`
	Evidence string `json:"evidence"`
}
type CPU struct {
	Load1    *float64 `json:"load_1"`
	Load5    *float64 `json:"load_5"`
	Load15   *float64 `json:"load_15"`
	Evidence string   `json:"evidence"`
}
type Facts struct {
	Addresses      []Address  `json:"addresses"`
	Routes         []Route    `json:"routes"`
	Counters       []Counter  `json:"counters"`
	Neighbors      []Neighbor `json:"neighbors"`
	Version        *Version   `json:"version"`
	CPU            *CPU       `json:"cpu"`
	UptimeSeconds  *float64   `json:"uptime_seconds"`
	UptimeEvidence string     `json:"uptime_evidence"`
	Notes          []string   `json:"notes"`
}

func Empty() Facts {
	return Facts{Addresses: []Address{}, Routes: []Route{}, Counters: []Counter{}, Neighbors: []Neighbor{}, Notes: []string{}}
}
