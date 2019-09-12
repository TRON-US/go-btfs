package main

import (
	"testing"
)

// function to send error message to status server
func TestSendError(t *testing.T) {
	err := SendError("Send Error", "http://localhost:8081", "my peer id", "my HValue")
	if err != nil {
		t.Errorf("Send error message to status server failed, reason: %v", err)
	} else {
		t.Log("Send error message to status server successfully!")
	}
}
