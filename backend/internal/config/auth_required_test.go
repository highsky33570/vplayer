package config

import "testing"

func TestLoadAuthRequired(t *testing.T) {
	t.Setenv("AUTH_REQUIRED", "")
	if !Load().AuthRequired {
		t.Fatal("AUTH_REQUIRED defaults to true")
	}
	t.Setenv("AUTH_REQUIRED", "false")
	if Load().AuthRequired {
		t.Fatal("AUTH_REQUIRED=false")
	}
	t.Setenv("AUTH_REQUIRED", "true")
	if !Load().AuthRequired {
		t.Fatal("AUTH_REQUIRED=true")
	}
}
