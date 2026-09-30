package main

import (
	"networkroute/internal/config"
	"testing"
)

func TestStandaloneSmartCreatesNoRelayClient(t *testing.T) {
	c, err := config.Load("../../config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != "smart" || c.Relay.Address != "" {
		t.Fatal("example requires infrastructure")
	}
	relay, err := relayClient(c)
	if err != nil || relay != nil {
		t.Fatalf("standalone attempted to load relay identity: %v", err)
	}
}
