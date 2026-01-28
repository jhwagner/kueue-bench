package kueue

import (
	"testing"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

func TestProvisionKueueObjects_NilConfig(t *testing.T) {
	// Verify that nil config doesn't cause errors
	err := ProvisionKueueObjects(nil, nil, nil)
	if err != nil {
		t.Errorf("expected no error with nil config, got: %v", err)
	}
}

func TestProvisionKueueObjects_EmptyConfig(t *testing.T) {
	// Verify that empty config doesn't cause errors
	emptyConfig := &config.KueueConfig{}
	err := ProvisionKueueObjects(nil, nil, emptyConfig)
	if err != nil {
		t.Errorf("expected no error with empty config, got: %v", err)
	}
}
