package main

import (
	"os"
	"testing"
)

func TestGenerateStartConfig(t *testing.T) {
	pwd, err := os.Getwd()
	t.Logf("pwd is: %v", pwd)
	err = GenerateStartConfig()
	if err != nil {
		t.Fatalf("GenerateStartConfig failed: %s", err)
	}
}
