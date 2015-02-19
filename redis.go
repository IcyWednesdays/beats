package main

import (
	"bytes"
	"packetbeat/common"
	"packetbeat/logp"
	"packetbeat/outputs"
	"strconv"
	"strings"
	"time"
)

type RedisMessage struct {
	Ts            time.Time
	NumberOfBulks int64
	Bulks         []string

	TcpTuple     TcpTuple
	CmdlineTuple *CmdlineTuple
	Direction    uint8

	IsRequest bool
	IsError   bool
	Message   string
	Method    string
	Path      string
	Size      int
}

type RedisStream struct {
	tcpStream *TcpStream

	data []byte

	parseOffset   int
	bytesReceived int

	message *RedisMessage
}

type RedisTransaction struct {
	Type         string
	tuple        TcpTuple
	Src          Endpoint
	Dst          Endpoint
	ResponseTime int32
	Ts           int64
	JsTs         time.Time
	ts           time.Time
	cmdline      *CmdlineTuple
	Method       string
	Path         string
	Query        string
	IsError      bool
	BytesOut     int
	BytesIn      int

	Redis common.MapStr

	Request_raw  string
	Response_raw string

	timer *time.Timer
}

// Keep sorted for future command addition
var RedisCommands = map[string]struct{}{
	"APPEND":           struct{}{},
	"AUTH":             struct{}{},
	"BGREWRITEAOF":     struct{}{},
	"BGSAVE":           struct{}{},
	"BITCOUNT":         struct{}{},
	"BITOP":            struct{}{},
	"BITPOS":           struct{}{},
	"BLPOP":            struct{}{},
	"BRPOP":            struct{}{},
	"BRPOPLPUSH":       struct{}{},
	"CLIENT GETNAME":   struct{}{},
	"CLIENT KILL":      struct{}{},
	"CLIENT LIST":      struct{}{},
	"CLIENT PAUSE":     struct{}{},
	"CLIENT SETNAME":   struct{}{},
	"CONFIG GET":       struct{}{},
	"CONFIG RESETSTAT": struct{}{},
	"CONFIG REWRITE":   struct{}{},
	"CONFIG SET":       struct{}{},
	"DBSIZE":           struct{}{},
	"DEBUG OBJECT":     struct{}{},
	"DEBUG SEGFAULT":   struct{}{},
	"DECR":             struct{}{},
	"DECRBY":           struct{}{},
	"DEL":              struct{}{},
	"DISCARD":          struct{}{},
	"DUMP":             struct{}{},
	"ECHO":             struct{}{},
	"EVAL":             struct{}{},
	"EVALSHA":          struct{}{},
	"EXEC":             struct{}{},
	"EXISTS":           struct{}{},
	"EXPIRE":           struct{}{},
	"EXPIREAT":         struct{}{},
	"FLUSHALL":         struct{}{},
	"FLUSHDB":          struct{}{},
	"GET":              struct{}{},
	"GETBIT":           struct{}{},
	"GETRANGE":         struct{}{},
	"GETSET":           struct{}{},
	"HDEL":             struct{}{},
	"HEXISTS":          struct{}{},
	"HGET":             struct{}{},
	"HGETALL":          struct{}{},
	"HINCRBY":          struct{}{},
	"HINCRBYFLOAT":     struct{}{},
	"HKEYS":            struct{}{},
	"HLEN":             struct{}{},
	"HMGET":            struct{}{},
	"HMSET":            struct{}{},
	"HSCAN":            struct{}{},
	"HSET":             struct{}{},
	"HSETINX":          struct{}{},
	"HVALS":            struct{}{},
	"INCR":             struct{}{},
	"INCRBY":           struct{}{},
	"INCRBYFLOAT":      struct{}{},
	"INFO":             struct{}{},
	"KEYS":             struct{}{},
	"LASTSAVE":         struct{}{},
	"LINDEX":           struct{}{},
	"LINSERT":          struct{}{},
	"LLEN":             struct{}{},
	"LPOP":             struct{}{},
	"LPUSH":            struct{}{},
	"LPUSHX":           struct{}{},
	"LRANGE":           struct{}{},
	"LREM":             struct{}{},
	"LSET":             struct{}{},
	"LTRIM":            struct{}{},
	"MGET":             struct{}{},
	"MIGRATE":          struct{}{},
	"MONITOR":          struct{}{},
	"MOVE":             struct{}{},
	"MSET":             struct{}{},
	"MSETNX":           struct{}{},
	"MULTI":            struct{}{},
	"OBJECT":           struct{}{},
	"PERSIST":          struct{}{},
	"PEXPIRE":          struct{}{},
	"PEXPIREAT":        struct{}{},
	"PFADD":            struct{}{},
	"PFCOUNT":          struct{}{},
	"PFMERGE":          struct{}{},
	"PING":             struct{}{},
	"PSETEX":           struct{}{},
	"PSUBSCRIBE":       struct{}{},
	"PTTL":             struct{}{},
	"PUBLISH":          struct{}{},
	"PUBSUB":           struct{}{},
	"PUNSUBSCRIBE":     struct{}{},
	"QUIT":             struct{}{},
	"RANDOMKEY":        struct{}{},
	"RENAME":           struct{}{},
	"RENAMENX":         struct{}{},
	"RESTORE":          struct{}{},
	"RPOP":             struct{}{},
	"RPOPLPUSH":        struct{}{},
	"RPUSH":            struct{}{},
	"RPUSHX":           struct{}{},
	"SADD":             struct{}{},
	"SAVE":             struct{}{},
	"SCAN":             struct{}{},
	"SCARD":            struct{}{},
	"SCRIPT EXISTS":    struct{}{},
	"SCRIPT FLUSH":     struct{}{},
	"SCRIPT KILL":      struct{}{},
	"SCRIPT LOAD":      struct{}{},
	"SDIFF":            struct{}{},
	"SDIFFSTORE":       struct{}{},
	"SELECT":           struct{}{},
	"SET":              struct{}{},
	"SETBIT":           struct{}{},
	"SETEX":            struct{}{},
	"SETNX":            struct{}{},
	"SETRANGE":         struct{}{},
	"SHUTDOWN":         struct{}{},
	"SINTER":           struct{}{},
	"SINTERSTORE":      struct{}{},
	"SISMEMBER":        struct{}{},
	"SLAVEOF":          struct{}{},
	"SLOWLOG":          struct{}{},
	"SMEMBERS":         struct{}{},
	"SMOVE":            struct{}{},
	"SORT":             struct{}{},
	"SPOP":             struct{}{},
	"SRANDMEMBER":      struct{}{},
	"SREM":             struct{}{},
	"SSCAN":            struct{}{},
	"STRLEN":           struct{}{},
	"SUBSCRIBE":        struct{}{},
	"SUNION":           struct{}{},
	"SUNIONSTORE":      struct{}{},
	"SYNC":             struct{}{},
	"TIME":             struct{}{},
	"TTL":              struct{}{},
	"TYPE":             struct{}{},
	"UNSUBSCRIBE":      struct{}{},
	"UNWATCH":          struct{}{},
	"WATCH":            struct{}{},
	"ZADD":             struct{}{},
	"ZCARD":            struct{}{},
	"ZCOUNT":           struct{}{},
	"ZINCRBY":          struct{}{},
	"ZINTERSTORE":      struct{}{},
	"ZRANGE":           struct{}{},
	"ZRANGEBYSCORE":    struct{}{},
	"ZRANK":            struct{}{},
	"ZREM":             struct{}{},
	"ZREMRANGEBYLEX":   struct{}{},
	"ZREMRANGEBYRANK":  struct{}{},
	"ZREMRANGEBYSCORE": struct{}{},
	"ZREVRANGE":        struct{}{},
	"ZREVRANGEBYSCORE": struct{}{},
	"ZREVRANK":         struct{}{},
	"ZSCAN":            struct{}{},
	"ZSCORE":           struct{}{},
	"ZUNIONSTORE":      struct{}{},
}

var redisTransactionsMap = make(map[HashableTcpTuple]*RedisTransaction, TransactionsHashSize)

func (stream *RedisStream) PrepareForNewMessage() {
	stream.data = stream.data[stream.parseOffset:]
	stream.parseOffset = 0
	stream.message = &RedisMessage{Ts: stream.message.Ts}
	stream.message.Bulks = []string{}
}

func redisMessageParser(s *RedisStream) (bool, bool) {

	var err error
	var value string
	m := s.message

	iserror := false

	for s.parseOffset < len(s.data) {

		if s.data[s.parseOffset] == '*' {
			//Multi Bulk Message

			found, line, off := readLine(s.data, s.parseOffset)
			if !found {
				logp.Debug("redis", "End of line not found, waiting for more data")
				return true, false
			}

			if len(line) == 3 && line[1] == '-' && line[2] == '1' {
				//NULL Multi Bulk
				s.parseOffset = off
				value = "nil"
			} else {

				m.NumberOfBulks, err = strconv.ParseInt(line[1:], 10, 64)

				if err != nil {
					logp.Err("Failed to read number of bulk messages: %s", err)
					return false, false
				}
				s.parseOffset = off
				m.Bulks = []string{}

				continue
			}

		} else if s.data[s.parseOffset] == '$' {
			old_offset := s.parseOffset
			// Bulk Reply

			found, line, off := readLine(s.data, s.parseOffset)
			if !found {
				logp.Debug("redis", "End of line not found, waiting for more data")
				s.parseOffset = old_offset
				return true, false
			}

			if len(line) == 3 && line[1] == '-' && line[2] == '1' {
				// NULL Bulk Reply
				value = "nil"
				s.parseOffset = off
			} else {
				length, err := strconv.ParseInt(line[1:], 10, 64)
				if err != nil {
					logp.Err("Failed to read bulk message: %s", err)
					return false, false
				}

				s.parseOffset = off

				found, line, off = readLine(s.data, s.parseOffset)
				if !found {
					logp.Debug("redis", "End of line not found, waiting for more data")
					s.parseOffset = old_offset
					return true, false
				}

				if int64(len(line)) != length {
					logp.Err("Wrong length of data: %d instead of %d", len(line), length)
					return false, false
				}
				value = line
				s.parseOffset = off
			}

		} else if s.data[s.parseOffset] == ':' {
			// Integer reply
			found, line, off := readLine(s.data, s.parseOffset)
			if !found {
				return true, false
			}
			n, err := strconv.ParseInt(line[1:], 10, 64)

			if err != nil {
				logp.Err("Failed to read integer reply: %s", err)
				return false, false
			}
			value = strconv.Itoa(int(n))
			s.parseOffset = off

		} else if s.data[s.parseOffset] == '+' {
			// Status Reply
			found, line, off := readLine(s.data, s.parseOffset)
			if !found {
				return true, false
			}

			value = line[1:]
			s.parseOffset = off
		} else if s.data[s.parseOffset] == '-' {
			// Error Reply
			found, line, off := readLine(s.data, s.parseOffset)
			if !found {
				return true, false
			}
			iserror = true

			value = line[1:]
			s.parseOffset = off
		} else {
			logp.Debug("redis", "Unexpected message starting with %s", s.data[s.parseOffset:])
			return false, false
		}

		// add value
		if m.NumberOfBulks > 0 {
			m.NumberOfBulks = m.NumberOfBulks - 1
			m.Bulks = append(m.Bulks, value)

			if len(m.Bulks) == 1 {
				logp.Debug("redis", "Value: %s", value)
				// first word.
				// check if it's a command
				if isRedisCommand(value) {
					logp.Debug("redis", "is request")
					m.IsRequest = true
					m.Method = value
				}
			}

			if len(m.Bulks) == 2 {
				// second word. This is usually the path
				if m.IsRequest {
					m.Path = value
				}
			}

			if m.NumberOfBulks == 0 {
				// the last bulk received
				m.Message = strings.Join(m.Bulks, " ")
				m.Size = s.parseOffset
				return true, true
			}
		} else {
			m.Message = value
			m.Size = s.parseOffset
			if iserror {
				m.IsError = true
			}
			return true, true
		}

	} //end for

	return true, false
}

func readLine(data []byte, offset int) (bool, string, int) {
	q := bytes.Index(data[offset:], []byte("\r\n"))
	if q == -1 {
		return false, "", 0
	}
	return true, string(data[offset : offset+q]), offset + q + 2
}

func ParseRedis(pkt *Packet, tcp *TcpStream, dir uint8) {
	defer logp.Recover("ParseRedis exception")

	if tcp.redisData[dir] == nil {
		tcp.redisData[dir] = &RedisStream{
			tcpStream: tcp,
			data:      pkt.payload,
			message:   &RedisMessage{Ts: pkt.ts},
		}
	} else {
		// concatenate bytes
		tcp.redisData[dir].data = append(tcp.redisData[dir].data, pkt.payload...)
		if len(tcp.redisData[dir].data) > TCP_MAX_DATA_IN_STREAM {
			logp.Debug("redis", "Stream data too large, dropping TCP stream")
			tcp.redisData[dir] = nil
			return
		}
	}

	stream := tcp.redisData[dir]
	for len(stream.data) > 0 {
		if stream.message == nil {
			stream.message = &RedisMessage{Ts: pkt.ts}
		}

		ok, complete := redisMessageParser(tcp.redisData[dir])

		if !ok {
			// drop this tcp stream. Will retry parsing with the next
			// segment in it
			tcp.redisData[dir] = nil
			logp.Debug("redis", "Ignore Redis message. Drop tcp stream. Try parsing with the next segment")
			return
		}

		if complete {

			if stream.message.IsRequest {
				logp.Debug("redis", "REDIS request message: %s", stream.message.Message)
			} else {
				logp.Debug("redis", "REDIS response message: %s", stream.message.Message)
			}

			// all ok, go to next level
			handleRedis(stream.message, tcp, dir)

			// and reset message
			stream.PrepareForNewMessage()
		} else {
			// wait for more data
			break
		}
	}

}

func isRedisCommand(key string) bool {
	_, exists := RedisCommands[strings.ToUpper(key)]
	return exists
}

func handleRedis(m *RedisMessage, tcp *TcpStream,
	dir uint8) {

	m.TcpTuple = TcpTupleFromIpPort(tcp.tuple, tcp.id)
	m.Direction = dir
	m.CmdlineTuple = procWatcher.FindProcessesTuple(tcp.tuple)

	if m.IsRequest {
		receivedRedisRequest(m)
	} else {
		receivedRedisResponse(m)
	}
}

func receivedRedisRequest(msg *RedisMessage) {
	// Add it to the HT
	tuple := msg.TcpTuple

	trans := redisTransactionsMap[tuple.raw]
	if trans != nil {
		if trans.Redis != nil {
			logp.Warn("Two requests without a Response. Dropping old request")
		}
	} else {
		trans = &RedisTransaction{Type: "redis", tuple: tuple}
		redisTransactionsMap[tuple.raw] = trans
	}

	trans.Redis = common.MapStr{}
	trans.Method = msg.Method
	trans.Path = msg.Path
	trans.Query = msg.Message
	trans.Request_raw = msg.Message
	trans.BytesIn = msg.Size

	trans.cmdline = msg.CmdlineTuple
	trans.ts = msg.Ts
	trans.Ts = int64(trans.ts.UnixNano() / 1000) // transactions have microseconds resolution
	trans.JsTs = msg.Ts
	trans.Src = Endpoint{
		Ip:   msg.TcpTuple.Src_ip.String(),
		Port: msg.TcpTuple.Src_port,
		Proc: string(msg.CmdlineTuple.Src),
	}
	trans.Dst = Endpoint{
		Ip:   msg.TcpTuple.Dst_ip.String(),
		Port: msg.TcpTuple.Dst_port,
		Proc: string(msg.CmdlineTuple.Dst),
	}
	if msg.Direction == TcpDirectionReverse {
		trans.Src, trans.Dst = trans.Dst, trans.Src
	}

	if trans.timer != nil {
		trans.timer.Stop()
	}
	trans.timer = time.AfterFunc(TransactionTimeout, func() { trans.Expire() })

}

func (trans *RedisTransaction) Expire() {

	// remove from map
	delete(redisTransactionsMap, trans.tuple.raw)
}

func receivedRedisResponse(msg *RedisMessage) {

	tuple := msg.TcpTuple
	trans := redisTransactionsMap[tuple.raw]
	if trans == nil {
		logp.Warn("Response from unknown transaction. Ignoring.")
		return
	}
	// check if the request was received
	if trans.Redis == nil {
		logp.Warn("Response from unknown transaction. Ignoring.")
		return

	}

	trans.IsError = msg.IsError
	if msg.IsError {
		trans.Redis["error"] = msg.Message
	} else {
		trans.Redis["return_value"] = msg.Message
	}

	trans.BytesOut = msg.Size
	trans.Response_raw = msg.Message

	trans.ResponseTime = int32(msg.Ts.Sub(trans.ts).Nanoseconds() / 1e6) // resp_time in milliseconds

	err := Publisher.PublishRedisTransaction(trans)
	if err != nil {
		logp.Warn("Publish failure: %s", err)
	}

	logp.Debug("redis", "Redis transaction completed: %s", trans.Redis)

	// remove from map
	delete(redisTransactionsMap, trans.tuple.raw)
	if trans.timer != nil {
		trans.timer.Stop()
	}

}

func (publisher *PublisherType) PublishRedisTransaction(t *RedisTransaction) error {

	event := outputs.Event{}
	event.Type = "redis"
	if !t.IsError {
		event.Status = OK_STATUS
	} else {
		event.Status = ERROR_STATUS
	}
	event.ResponseTime = t.ResponseTime
	event.RequestRaw = t.Request_raw
	event.ResponseRaw = t.Response_raw
	event.Redis = common.MapStr(t.Redis)
	event.Method = strings.ToUpper(t.Method)
	event.Path = t.Path
	event.Query = t.Query
	event.BytesIn = uint64(t.BytesIn)
	event.BytesOut = uint64(t.BytesOut)

	return publisher.PublishEvent(t.ts, &t.Src, &t.Dst, &event)
}
