package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
)

type Config struct {
	NodeID, OrganizationID, ListenAddress string
	DatabaseURL                           string
	Peers                                 []node.Peer
	RequestTimeout                        time.Duration
}

func FromEnvironment() (Config, error) {
	config := Config{NodeID: os.Getenv("NODE_ID"), OrganizationID: os.Getenv("ORGANIZATION_ID"), ListenAddress: os.Getenv("LISTEN_ADDRESS"), RequestTimeout: 5 * time.Second}
	config.DatabaseURL = os.Getenv("DATABASE_URL")
	if config.ListenAddress == "" {
		config.ListenAddress = ":8081"
	}
	if value := os.Getenv("REQUEST_TIMEOUT"); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("REQUEST_TIMEOUT: %w", err)
		}
		config.RequestTimeout = duration
	}
	for i, value := range strings.Split(os.Getenv("PEERS"), ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			config.Peers = append(config.Peers, node.Peer{ID: fmt.Sprintf("peer-%d", i+1), BaseURL: value})
		}
	}
	if config.NodeID == "" || config.OrganizationID == "" || config.DatabaseURL == "" {
		return Config{}, fmt.Errorf("NODE_ID, ORGANIZATION_ID, and DATABASE_URL are required")
	}
	return config, nil
}
