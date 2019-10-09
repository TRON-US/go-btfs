package storage

import (
	"testing"
)

func TestRetry(t *testing.T) {
	retryQueue := NewRetryQueue(2)
	host1 := &HostNode{
		Identity: "test1",
	}
	host2 := &HostNode{
		Identity: "test2",
	}
	// offer 2
	if err := retryQueue.Offer(host1); err != nil {
		t.Fatal(err)
	}
	if err := retryQueue.Offer(host2); err != nil {
		t.Fatal(err)
	}
	size := retryQueue.Size()
	if size != 2 {
		t.Errorf("size is incorrect, got %d, want %d", size, 2)
	}
	peek, err := retryQueue.Peek()
	if err != nil {
		t.Fatal(err)
	}
	if peek != host1 {
		t.Errorf("peek is incorrect, got %v, want %v", peek, host1)
	}

	// poll 1
	poll, err := retryQueue.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if peek != host1 {
		t.Errorf("poll is incorrect, got %v, want %v", poll, host1)
	}
	size = retryQueue.Size()
	if size != 1 {
		t.Errorf("size is incorrect, got %d, want %d", size, 1)
	}
	peek, err = retryQueue.Peek()
	if err != nil {
		t.Fatal(err)
	}
	if peek != host2 {
		t.Errorf("peek is incorrect, got %v, want %v", peek, host2)
	}

	// isEmpty
	if retryQueue.Empty() {
		t.Errorf("judging empty is incorrect, got %v, want %v", retryQueue.Empty(), false)
	}
	_, err = retryQueue.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if !retryQueue.Empty() {
		t.Errorf("judging empty is incorrect, got %v, want %v", retryQueue.Empty(), true)
	}
}
