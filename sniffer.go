package main

import (
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"code.google.com/p/gopacket/pcap"
	"fmt"
	"time"
)

type SnifferSetup struct {
	pcapHandle     *pcap.Handle
	afpacketHandle *AfpacketHandle
	config         *tomlInterfaces

	DataSource gopacket.PacketDataSource
}

type tomlInterfaces struct {
	Device         string
	Devices        []string
	Type           string
	File           string
	With_vlans     bool
	Bpf_filter     string
	Snaplen        int
	Buffer_size_mb int
}

// Computes the block_size and the num_blocks in such a way that the
// allocated mmap buffer is close to but smaller than target_size_mb
func afpacketComputeSize(target_size_mb int, frame_size int) (
	block_size int, num_blocks int, err error) {

	// 128 is the default from the gopacket library for
	block_size = frame_size * 128
	num_blocks = (target_size_mb * 1024 * 1024) / block_size

	if num_blocks == 0 {
		return 0, 0, fmt.Errorf("Buffer size too small")
	}

	return block_size, num_blocks, nil
}

func CreateSniffer(config *tomlInterfaces, file *string) (*SnifferSetup, error) {
	var sniffer SnifferSetup
	var err error

	sniffer.config = config

	if file != nil {
		// we read file with the pcap provider
		sniffer.config.Type = "pcap"
		sniffer.config.File = *file
	}

	// set defaults
	if len(sniffer.config.Device) == 0 {
		sniffer.config.Device = "any"
	}

	if len(sniffer.config.Devices) == 0 {
		// 'devices' not set but 'device' is set. For backwards compatibility,
		// use the one configured device
		if len(sniffer.config.Device) > 0 {
			sniffer.config.Devices = []string{sniffer.config.Device}
		}
	}
	if sniffer.config.Snaplen == 0 {
		sniffer.config.Snaplen = 1514
	}

	if sniffer.config.Type == "pcap" || sniffer.config.Type == "" {
		if len(sniffer.config.File) > 0 {
			sniffer.pcapHandle, err = pcap.OpenOffline(sniffer.config.File)
			if err != nil {
				return nil, err
			}
		} else {
			if len(sniffer.config.Devices) > 1 {
				return nil, fmt.Errorf("Pcap sniffer only supports one device. You can use 'any' if you want")
			}
			sniffer.pcapHandle, err = pcap.OpenLive(
				sniffer.config.Devices[0],
				int32(sniffer.config.Snaplen),
				true,
				500*time.Millisecond)
			if err != nil {
				return nil, err
			}
		}

		sniffer.DataSource = gopacket.PacketDataSource(sniffer.pcapHandle)

	} else if sniffer.config.Type == "af_packet" {
		if sniffer.config.Buffer_size_mb == 0 {
			sniffer.config.Buffer_size_mb = 24
		}

			if len(sniffer.config.Devices) > 1 {
				return nil, fmt.Errorf("Afpacket sniffer only supports one device. You can use 'any' if you want")
			}

		block_size, num_blocks, err := afpacketComputeSize(
			sniffer.config.Buffer_size_mb,
			sniffer.config.Snaplen)
		if err != nil {
			return nil, err
		}

		sniffer.afpacketHandle, err = NewAfpacketHandle(
			sniffer.config.Devices[0],
			int32(sniffer.config.Snaplen),
			block_size,
			num_blocks,
			500*time.Millisecond) // timeout
		if err != nil {
			return nil, err
		}

		sniffer.DataSource = gopacket.PacketDataSource(sniffer.afpacketHandle)

	} else {
		return nil, fmt.Errorf("Unknown sniffer type: %s", sniffer.config.Type)
	}

	return &sniffer, nil
}

func (sniffer *SnifferSetup) Reopen() error {
	var err error

	if sniffer.config.Type != "pcap" || sniffer.config.File == "" {
		return fmt.Errorf("Reopen is only possible for files")
	}

	sniffer.pcapHandle.Close()
	sniffer.pcapHandle, err = pcap.OpenOffline(sniffer.config.File)
	if err != nil {
		return err
	}

	sniffer.DataSource = gopacket.PacketDataSource(sniffer.pcapHandle)

	return nil
}

func (sniffer *SnifferSetup) Close() {
	switch sniffer.config.Type {
	case "pcap":
		sniffer.pcapHandle.Close()
	}
}

func (sniffer *SnifferSetup) Datalink() layers.LinkType {
	if sniffer.config.Type == "pcap" {
		return sniffer.pcapHandle.LinkType()
	}
	return layers.LinkTypeEthernet
}
