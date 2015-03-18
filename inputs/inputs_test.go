package inputs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInputNames(t *testing.T) {
	assert.Equal(t, "udpjson", UdpjsonInput.String())
	assert.Equal(t, "gobeacon", GoBeaconInput.String())
	assert.Equal(t, "sniffer", SnifferInput.String())
	assert.Equal(t, "impossible", Input(100).String())
}

func TestIsInList(t *testing.T) {
	assert.True(t, UdpjsonInput.IsInList([]string{"sniffer", "udpjson"}))
	assert.False(t, UdpjsonInput.IsInList([]string{"sniffer"}))
}
