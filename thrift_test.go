package main

import (
	"encoding/hex"
	"testing"
)

func TestThrift_thriftReadString(t *testing.T) {

	if testing.Verbose() {
		LogInit(LOG_DEBUG, "", false, []string{"thrift", "thriftdetailed"})
	}

	var data []byte
	var ok, complete bool
	var off int
	var str string

	data, _ = hex.DecodeString("0000000470696e67")
	str, ok, complete, off = thriftReadString(data)
	if str != "ping" || !ok || !complete || off != 8 {
		t.Error("Bad result: %s %s %s %s", str, ok, complete, off)
	}

	data, _ = hex.DecodeString("0000000470696e670000")
	str, ok, complete, off = thriftReadString(data)
	if str != "ping" || !ok || !complete || off != 8 {
		t.Error("Bad result: %s %s %s %s", str, ok, complete, off)
	}

	data, _ = hex.DecodeString("0000000470696e")
	str, ok, complete, off = thriftReadString(data)
	if str != "" || !ok || complete || off != 0 {
		t.Error("Bad result: %s %s %s %s", str, ok, complete, off)
	}

	data, _ = hex.DecodeString("000000")
	str, ok, complete, off = thriftReadString(data)
	if str != "" || !ok || complete || off != 0 {
		t.Error("Bad result: %s %s %s %s", str, ok, complete, off)
	}
}

func TestThriftParser_simpleRequest(t *testing.T) {

	if testing.Verbose() {
		LogInit(LOG_DEBUG, "", false, []string{"thrift", "thriftdetailed"})
	}

	data := []byte(
		"800100010000000470696e670000000000",
	)

	message, err := hex.DecodeString(string(data))
	if err != nil {
		t.Error("Failed to decode hex string")
	}

	stream := &ThriftStream{tcpStream: nil, data: message, message: new(ThriftMessage)}

	ok, complete := thriftMessageParser(stream)

	if !ok {
		t.Error("Parsing returned error")
	}
	if !complete {
		t.Error("Expecting a complete message")
	}
	if !stream.message.IsRequest {
		t.Error("Failed to parse Thrift request")
	}
	if stream.message.Method != "ping" {
		t.Error("Failed to parse query")
	}

}
