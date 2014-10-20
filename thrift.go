package main

import (
	"encoding/binary"
	"math"
	"time"
	"strconv"

	"labix.org/v2/mgo/bson"
)

type ThriftMessage struct {
	Ts time.Time

	Stream_id    uint32
	Tuple        *IpPortTuple
	CmdlineTuple *CmdlineTuple
	Direction    uint8

	start int
	end   int

	IsRequest bool
	Version   uint32
	Type      uint32
	Method    string
	SeqId     uint32
	Fields    []*ThriftField
}

type ThriftField struct {
	Type byte
	Id uint16

	Value string
}

type ThriftStream struct {
	tcpStream *TcpStream

	data []byte

	parseOffset   int
	parseState    int
	bytesReceived int

	message *ThriftMessage
}

type ThriftTransaction struct {
	Type         string
	tuple        TcpTuple
	Src          Endpoint
	Dst          Endpoint
	ResponseTime int32
	Ts           int64
	JsTs         time.Time
	ts           time.Time
	cmdline      *CmdlineTuple

	Thrift bson.M

	Request_raw  string
	Response_raw string

	timer *time.Timer
}

const (
	ThriftStartState = iota
	ThriftFieldState
)

const (
	ThriftVersionMask = 0xffff0000
	ThriftVersion1    = 0x80010000
	ThriftTypeMask    = 0x000000ff
)

// Thrift types
const (
	ThriftTypeStop = iota
	ThriftTypeVoid
	ThriftTypeBool
	ThriftTypeByte
	ThriftTypeDouble
	ThriftTypeI16 = 6
	ThriftTypeI32 = 8
	ThriftTypeI64 = 10
	ThriftTypeString = 11
	ThriftTypeStruct = 12
	ThriftTypeMap = 13
	ThriftTypeSet = 14
	ThriftTypeList = 15
	ThriftTypeUtf8 = 16
	ThriftTypeUtf16 = 17
)

// Thrift message types -- TODO: rename to ThriftTypeMsg..
const (
	_ = iota
	ThriftTypeCall
	ThriftTypeReply
	ThriftTypeException
	ThriftTypeOneway
)

func (m *ThriftMessage) readMessageBegin(s *ThriftStream) (bool, bool) {
	var ok, complete bool
	var offset, off int

	if len(s.data[s.parseOffset:]) < 9 {
		return true, false // ok, not complete
	}

	sz := Bytes_Ntohl(s.data[s.parseOffset : s.parseOffset+4])
	if int32(sz) < 0 {
		m.Version = sz & ThriftVersionMask
		if m.Version != ThriftVersion1 {
			DEBUG("thrift", "Unexpected version: %d", m.Version)
		}

		DEBUG("thriftdetailed", "version = %d", m.Version)

		offset = s.parseOffset + 4

		DEBUG("thriftdetailed", "offset = %d", offset)

		m.Type = sz & ThriftTypeMask
		m.Method, ok, complete, off = thriftReadString(s.data[offset:])
		if !ok {
			return false, false // not ok, not complete
		}
		if !complete {
			DEBUG("thriftdetailed", "Method name not complete")
			return true, false // ok, not complete
		}
		offset += off

		DEBUG("thriftdetailed", "method = %s", m.Method)
		DEBUG("thriftdetailed", "offset = %d", offset)

		if len(s.data[offset:]) < 4 {
			return true, false // ok, not complete
		}
		m.SeqId = Bytes_Ntohl(s.data[offset : offset+4])
		s.parseOffset = offset + 4
	} else {
		// no version mode
		offset = s.parseOffset

		m.Method, ok, complete, off = thriftReadString(s.data[offset:])
		if !ok {
			return false, false // not ok, not complete
		}
		if !complete {
			DEBUG("thriftdetailed", "Method name not complete")
			return true, false // ok, not complete
		}
		offset += off

		DEBUG("thriftdetailed", "method = %s", m.Method)
		DEBUG("thriftdetailed", "offset = %d", offset)

		if len(s.data[offset:]) < 5 {
			return true, false // ok, not complete
		}

		m.Type = uint32(s.data[offset])
		offset += 1
		m.SeqId = Bytes_Ntohl(s.data[offset : offset+4])
		s.parseOffset = offset + 4
	}

	if m.Type == ThriftTypeCall || m.Type == ThriftTypeOneway {
		m.IsRequest = true
	}

	return true, true
}

func thriftReadString(data []byte) (str string, ok bool, complete bool, off int) {
	if len(data) < 4 {
		return "", true, false, 0 // ok, not complete
	}
	sz := int(Bytes_Ntohl(data[:4]))
	if int32(sz) < 0 {
		return "", false, false, 0 // not ok
	}
	if len(data[4:]) < sz {
		return "", true, false, 0 // ok, not complete
	}

	str = string(data[4 : 4+sz])
	off = 4 + sz

	return str, true, true, off // all good
}

func thriftReadField(s *ThriftStream) (ok bool, complete bool, field *ThriftField) {

	field = new(ThriftField)

	if len(s.data) == 0 {
		return true, false, nil // ok, not complete
	}
	field.Type = byte(s.data[s.parseOffset])
	offset := s.parseOffset + 1
	if field.Type == ThriftTypeStop {
		s.parseOffset = offset
		return true, true, nil	// done
	}

	if len(s.data[offset:]) < 2 {
		return true, false, nil // ok, not complete
	}
	field.Id = Bytes_Ntohs(s.data[offset:offset+2])
	offset += 2

	switch field.Type {
		case ThriftTypeBool:
			if len(s.data[offset:]) < 1 {
				return true, false, nil
			}
			if s.data[offset] == byte(0) {
				field.Value = "true"
			} else {
				field.Value = "false"
			}
			offset += 1

		case ThriftTypeByte:
			if len(s.data[offset:]) < 1 {
				return true, false, nil
			}
			field.Value = strconv.Itoa(int(s.data[offset]))
			offset += 1

		case ThriftTypeDouble:
			if len(s.data[offset:]) < 8 {
				return true, false, nil
			}
			bits := binary.BigEndian.Uint64(s.data[offset:offset+8])
			double := math.Float64frombits(bits)
			field.Value = strconv.FormatFloat(double, 'f', -1, 64)
			offset += 8
	}

	s.parseOffset = offset
	return true, false, field
}

func thriftMessageParser(s *ThriftStream) (bool, bool) {
	var ok, complete bool
	var m = s.message

	for s.parseOffset < len(s.data) {
		switch s.parseState {
		case ThriftStartState:
			m.start = s.parseOffset

			ok, complete = m.readMessageBegin(s)
			if !ok {
				return false, false
			}
			if !complete {
				return true, false
			}

			s.parseState = ThriftFieldState
		case ThriftFieldState:
			ok, complete, field := thriftReadField(s)
			if !ok {
				return false, false
			}
			if complete {
				// done
				return true, true
			}
			if field == nil {
				return true, false // ok, not complete
			}

			m.Fields = append(m.Fields, field)
		}
	}

	return true, false
}

func (stream *ThriftStream) PrepareForNewMessage() {
	stream.data = stream.data[stream.parseOffset:]
	stream.parseOffset = 0
	stream.message.IsRequest = false
}

func ParseThrift(pkt *Packet, tcp *TcpStream, dir uint8) {
	defer RECOVER("ParseThrift exception")

	if tcp.thriftData[dir] == nil {
		tcp.thriftData[dir] = &ThriftStream{
			tcpStream: tcp,
			data:      pkt.payload,
			message:   &ThriftMessage{Ts: pkt.ts},
		}
	} else {
		// concatenate bytes
		tcp.thriftData[dir].data = append(tcp.thriftData[dir].data, pkt.payload...)
		if len(tcp.thriftData[dir].data) > TCP_MAX_DATA_IN_STREAM {
			DEBUG("thrift", "Stream data too large, dropping TCP stream")
			tcp.thriftData[dir] = nil
			return
		}
	}

	stream := tcp.thriftData[dir]
	for len(stream.data) > 0 {
		if stream.message == nil {
			stream.message = &ThriftMessage{Ts: pkt.ts}
		}

		ok, complete := thriftMessageParser(tcp.thriftData[dir])

		if !ok {
			// drop this tcp stream. Will retry parsing with the next
			// segment in it
			tcp.thriftData[dir] = nil
			DEBUG("thrift", "Ignore Thrift message. Drop tcp stream. Try parsing with the next segment")
			return
		}

		if complete {

			if stream.message.IsRequest {
				DEBUG("thrift", "Thrift request message: %s", stream.message.Method)
			} else {
				DEBUG("thrift", "Thrift response message: %s", stream.message.Method)
			}

			// all ok, go to next level
			//handleThrift(stream.message, tcp, dir)

			// and reset message
			stream.PrepareForNewMessage()
		} else {
			// wait for more data
			break
		}
	}

}
