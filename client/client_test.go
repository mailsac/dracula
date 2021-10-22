package client

import (
	"github.com/mailsac/dracula/server"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestClient_Auth(t *testing.T) {
	// there are already tests with empty secret as well

	secret := "asdf-jkl-HOHOHO!"
	badSecret := "Brute-Force9"
	s := server.NewServer(60, secret)
	err := s.Listen(9000)
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = true
	defer s.Close()

	goodClient := NewClient("127.0.0.1", 9000, 5, secret)
	err = goodClient.Listen(9001)
	if err != nil {
		t.Fatal(err)
	}
	goodClient.Debug = true
	defer goodClient.Close()

	badClient := NewClient("127.0.0.1", 9000, 5, badSecret)
	err = badClient.Listen(9002)
	if err != nil {
		t.Fatal(err)
	}
	badClient.Debug = true
	defer badClient.Close()

	// good client checks
	err = goodClient.Put("asdf", "99.33.22.44")
	if err != nil {
		t.Fatal(err)
	}
	// check it worked with auth
	c, err := goodClient.Count("asdf", "99.33.22.44")
	if err != nil {
		t.Fatal(err)
	}
	if c != 1 {
		t.Error("expected pre check count=1, got=", c)
	}

	// bad client checks, put same and count same
	err = badClient.Put("asdf", "99.33.22.44")
	assert.Error(t, err)
	assert.Equal(t, "auth failed: packet hash invalid", err.Error())
}
