package main

import (
	"encoding/hex"
	"testing"
	//"fmt"
	//"log/syslog"
)

func TestRedisParser_simpleRequest(t *testing.T) {

	data := []byte(
		"2a330d0a24330d0a5345540d0a24340d0a6b6579310d0a24350d0a48656c6c6f0d0a")

	message, err := hex.DecodeString(string(data))
	if err != nil {
		t.Errorf("Failed to decode hex string")
	}

	stream := &RedisStream{data: message, message: new(RedisMessage)}

	ok, complete := redisMessageParser(stream)

	if !ok {
		t.Errorf("Parsing returned error")
	}
	if !complete {
		t.Errorf("Expecting a complete message")
	}
	if !stream.message.IsRequest {
		t.Errorf("Failed to parse Redis request")
	}
	if stream.message.Message != "SET key1 Hello" {
		t.Errorf("Failed to parse Redis request: %s", stream.message.Message)
	}
}

func TestRedisParser_PosResult(t *testing.T) {

	data := []byte(
		"2b4f4b0d0a")

	message, err := hex.DecodeString(string(data))
	if err != nil {
		t.Errorf("Failed to decode hex string")
	}

	stream := &RedisStream{data: message, message: new(RedisMessage)}

	ok, complete := redisMessageParser(stream)

	if !ok {
		t.Errorf("Parsing returned error")
	}
	if !complete {
		t.Errorf("Expecting a complete message")
	}
	if stream.message.IsRequest {
		t.Errorf("Failed to parse Redis response")
	}
	if stream.message.Message != "OK" {
		t.Errorf("Failed to parse Redis response: %s", stream.message.Message)
	}
}

func TestRedisParser_NilResult(t *testing.T) {

	data := []byte(
		"2a310d0a242d310d0a")

	message, err := hex.DecodeString(string(data))
	if err != nil {
		t.Errorf("Failed to decode hex string")
	}

	stream := &RedisStream{data: message, message: new(RedisMessage)}

	ok, complete := redisMessageParser(stream)

	if !ok {
		t.Errorf("Parsing returned error")
	}
	if !complete {
		t.Errorf("Expecting a complete message")
	}
	if stream.message.IsRequest {
		t.Errorf("Failed to parse Redis response")
	}
	if stream.message.Message != "nil" {
		t.Errorf("Failed to parse Redis response: %s", stream.message.Message)
	}
}
