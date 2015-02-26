package udpjson

import (
	"net"
	"packetbeat/common"
	"packetbeat/logp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUdpJson(t *testing.T) {
	//t.Skip("Skipped because it seems to hang on Travis CI")

	if testing.Verbose() {
		logp.LogInit(logp.LOG_DEBUG, "", false, []string{"udpjson"})
	}

	events := make(chan common.MapStr)
	server, err := NewServer(Config{
		Port:   32000,
		BindIp: "127.0.0.1",
	}, 10*time.Millisecond, events)

	assert.Nil(t, err)
	assert.NotNil(t, server)

	ready := make(chan bool)

	go func() {
		ready <- true
		err := server.ReceiveForever()
		assert.Nil(t, err, "Error: %v", err)
	}()

	// make sure the goroutine runs first
	<-ready

	logp.Debug("udpjson", server.conn.LocalAddr().String())

	// send a message
	clientConn, err := net.Dial("udp", server.conn.LocalAddr().String())
	assert.Nil(t, err, "Error: %v", err)

	_, err = clientConn.Write([]byte(`{"hello": "udpserver"}`))
	assert.Nil(t, err)

	obj := <-events
	assert.Equal(t, obj, common.MapStr{"hello": "udpserver"})

	_, err = clientConn.Write([]byte(`{"obj2": 4}`))
	assert.Nil(t, err)

	obj = <-events
	assert.Equal(t, obj, common.MapStr{"obj2": 4})

	server.Stop()
}
