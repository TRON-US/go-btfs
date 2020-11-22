package path

import (
	"testing"
)

func TestList(t *testing.T) {
	_, err := list("/")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdd(t *testing.T) {
	err := add("/tmp", "abc")
	if err != nil {
		t.Fatal(err)
	}
}
