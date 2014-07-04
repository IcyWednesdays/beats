package main

import (
    "encoding/json"
    "errors"
    "labix.org/v2/mgo/bson"
    "os"
    "strings"
    "time"
)

type PublisherType struct {
    name     string
    disabled bool
    Index    string
    Output   []OutputInterface

    RefreshTopologyTimer <-chan time.Time
}

type OutputInterface interface {
    PublishIPs(name string, localAddrs []string) error
    GetNameByIP(ip string) string
    PublishEvent(event *Event) error
}

var Publisher PublisherType

// Config
type tomlAgent struct {
    Name                  string
    Refresh_topology_freq int
    Ignore_outgoing       bool
    Topology_expire       int
}
type tomlMothership struct {
    Host     string
    Port     int
    Protocol string
    Username string
    Password string
    Index    string
    Path     string
}

const (
    ElasticsearchOutputName = "elasticsearch"
    RedisOutputName         = "redis"
)

var outputTypes = []string{ElasticsearchOutputName, RedisOutputName}

type Event struct {
    Timestamp    time.Time `json:"@timestamp"`
    Type         string    `json:"type"`
    Agent        string    `json:"agent"`
    Src_ip       string    `json:"src_ip"`
    Src_port     uint16    `json:"src_port"`
    Src_proc     string    `json:"src_proc"`
    Src_country  string    `json:"src_country"`
    Src_server   string    `json:"src_server"`
    Dst_ip       string    `json:"dst_ip"`
    Dst_port     uint16    `json:"dst_port"`
    Dst_proc     string    `json:"dst_proc"`
    Dst_server   string    `json:"dst_server"`
    ResponseTime int32     `json:"responsetime"`
    Status       string    `json:"status"`
    RequestRaw   string    `json:"request_raw"`
    ResponseRaw  string    `json:"response_raw"`

    Mysql bson.M `json:"mysql"`
    Http  bson.M `json:"http"`
    Redis bson.M `json:"redis"`
    Pgsql bson.M `json:"pgsql"`
}

type Topology struct {
    Name string `json:"name"`
    Ip   string `json:"ip"`
}

func PrintPublishEvent(event *Event) {
    json, err := json.MarshalIndent(event, "", "  ")
    if err != nil {
        ERR("json.Marshal: %s", err)
    } else {
        DEBUG("publish", "Publish: %s", string(json))
    }
}

const (
    OK_STATUS    = "OK"
    ERROR_STATUS = "Error"
)

func (publisher *PublisherType) GetServerName(ip string) string {
    // in case the IP is localhost, return current agent name
    islocal, err := IsLoopback(ip)
    if err != nil {
        ERR("Parsing IP %s fails with: %s", ip, err)
        return ""
    } else {
        if islocal {
            return publisher.name
        }
    }
    // find the agent with the desired IP
    return publisher.Output[0].GetNameByIP(ip)
}

func (publisher *PublisherType) PublishHttpTransaction(t *HttpTransaction) error {

    event := Event{}

    event.Type = "http"
    response := t.Http["response"].(bson.M)
    code := response["code"].(uint16)
    if code < 400 {
        event.Status = OK_STATUS
    } else {
        event.Status = ERROR_STATUS
    }
    event.ResponseTime = t.ResponseTime
    event.RequestRaw = t.Request_raw
    event.ResponseRaw = t.Response_raw
    event.Http = t.Http

    return publisher.PublishEvent(t.ts, &t.Src, &t.Dst, &event)

}

func (publisher *PublisherType) PublishMysqlTransaction(t *MysqlTransaction) error {

    event := Event{}
    event.Type = "mysql"

    if t.Mysql["iserror"].(bool) {
        event.Status = ERROR_STATUS
    } else {
        event.Status = OK_STATUS
    }

    event.ResponseTime = t.ResponseTime
    event.RequestRaw = t.Request_raw
    event.ResponseRaw = t.Response_raw
    event.Mysql = t.Mysql

    return publisher.PublishEvent(t.ts, &t.Src, &t.Dst, &event)
}

func (publisher *PublisherType) PublishRedisTransaction(t *RedisTransaction) error {

    event := Event{}
    event.Type = "redis"
    event.Status = OK_STATUS
    event.ResponseTime = t.ResponseTime
    event.RequestRaw = t.Request_raw
    event.ResponseRaw = t.Response_raw
    event.Redis = t.Redis

    return publisher.PublishEvent(t.ts, &t.Src, &t.Dst, &event)
}

func (publisher *PublisherType) PublishEvent(ts time.Time, src *Endpoint, dst *Endpoint, event *Event) error {

    event.Src_server = publisher.GetServerName(src.Ip)
    event.Dst_server = publisher.GetServerName(dst.Ip)

    if _Config.Agent.Ignore_outgoing && event.Dst_server != "" &&
        event.Dst_server != publisher.name {
        // duplicated transaction -> ignore it
        DEBUG("publish", "Ignore duplicated REDIS transaction on %s: %s -> %s", publisher.name, event.Src_server, event.Dst_server)
        return nil
    }

    event.Timestamp = ts
    event.Agent = publisher.name
    event.Src_ip = src.Ip
    event.Src_port = src.Port
    event.Src_proc = src.Proc
    event.Dst_ip = dst.Ip
    event.Dst_port = dst.Port
    event.Dst_proc = dst.Proc

    // set src_country if no src_server is set
    event.Src_country = ""
    if _GeoLite != nil {
        if len(event.Src_server) == 0 { // only for external IP addresses
            loc := _GeoLite.GetLocationByIP(src.Ip)
            if loc != nil {
                event.Src_country = loc.CountryCode
            }
        }
    }

    if IS_DEBUG("publish") {
        PrintPublishEvent(event)
    }

    // add transaction
    var err error
    if !publisher.disabled {
        for i := 0; i < len(publisher.Output); i++ {
            publisher.Output[i].PublishEvent(event)
        }
    }

    return err
}
func (publisher *PublisherType) PublishPgsqlTransaction(t *PgsqlTransaction) error {

    event := Event{}

    event.Type = "pgsql"
    if t.Pgsql["iserror"].(bool) {
        event.Status = ERROR_STATUS
    } else {
        event.Status = OK_STATUS
    }
    event.ResponseTime = t.ResponseTime
    event.RequestRaw = t.Request_raw
    event.ResponseRaw = t.Response_raw
    event.Pgsql = t.Pgsql

    return publisher.PublishEvent(t.ts, &t.Src, &t.Dst, &event)
}

func (publisher *PublisherType) UpdateTopologyPeriodically() {
    for _ = range publisher.RefreshTopologyTimer {
        publisher.PublishTopology()
    }
}

func (publisher *PublisherType) PublishTopology(params ...string) error {

    var localAddrs []string = params

    if len(params) == 0 {
        addrs, err := LocalIpAddrsAsStrings(false)
        if err != nil {
            ERR("Getting local IP addresses fails with: %s", err)
            return err
        }
        localAddrs = addrs
    }

    DEBUG("publish", "Add topology entry for %s: %s", publisher.name, strings.Join(localAddrs, " "))

    for i := 0; i < len(publisher.Output); i++ {
        publisher.Output[i].PublishIPs(publisher.name, localAddrs)
    }

    return nil
}

func (publisher *PublisherType) Init(publishDisabled bool) error {
    var err error

    for i := 0; i < len(outputTypes); i++ {
        output, exists := _Config.Output[outputTypes[i]]
        if exists {
            INFO("Using output %s, type=%s", output, outputTypes[i])
            switch outputTypes[i] {
            case ElasticsearchOutputName:
                ElasticsearchOutput.Init(output)
                publisher.Output = append(publisher.Output, OutputInterface(&ElasticsearchOutput))
                break

            case RedisOutputName:
                RedisOutput.Init(output)
                publisher.Output = append(publisher.Output, OutputInterface(&RedisOutput))
                INFO("connection %s", publisher.Output[0])
                break
            }
        }
    }

    if len(publisher.Output) == 0 {
        INFO("No outputs are defined. Please define one under [output]")
        return errors.New("No outputs are define")
    }

    publisher.name = _Config.Agent.Name
    if len(publisher.name) == 0 {
        // use the hostname
        publisher.name, err = os.Hostname()
        if err != nil {
            return err
        }

        INFO("No agent name configured, using hostname '%s'", publisher.name)
    }

    publisher.disabled = publishDisabled
    if publisher.disabled {
        INFO("Dry run mode. Elasticsearch won't be updated or queried.")
    }

    RefreshTopologyFreq := 10 * time.Second
    if _Config.Agent.Refresh_topology_freq != 0 {
        RefreshTopologyFreq = time.Duration(_Config.Agent.Refresh_topology_freq) * time.Second
    }
    publisher.RefreshTopologyTimer = time.Tick(RefreshTopologyFreq)
    INFO("Topology map refreshed every %s", RefreshTopologyFreq)

    if !publisher.disabled {
        // register agent and its public IP addresses
        err = publisher.PublishTopology()
        if err != nil {
            ERR("Failed to publish topology: %s", err)
            return err
        }

        // update topology periodically
        go publisher.UpdateTopologyPeriodically()
    }

    return nil
}
