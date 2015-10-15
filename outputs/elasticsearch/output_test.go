package elasticsearch

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/elastic/libbeat/common"
	"github.com/elastic/libbeat/logp"
	"github.com/elastic/libbeat/outputs"
	"github.com/stretchr/testify/assert"
)

func createElasticsearchConnection(flushInterval int, bulkSize int) elasticsearchOutput {
	index := fmt.Sprintf("packetbeat-unittest-%d", os.Getpid())

	esPort, err := strconv.Atoi(GetEsPort())

	if err != nil {
		logp.Err("Invalid port. Cannot be converted to in: %s", GetEsPort())
	}

	var output elasticsearchOutput
	output.Init("packetbeat", outputs.MothershipConfig{
		Enabled:        true,
		Save_topology:  true,
		Host:           GetEsHost(),
		Port:           esPort,
		Username:       "",
		Password:       "",
		Path:           "",
		Index:          index,
		Protocol:       "",
		Flush_interval: &flushInterval,
		Bulk_size:      &bulkSize,
	}, 10)

	return output
}

func TestTopologyInES(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping topology tests in short mode, because they require Elasticsearch")
	}
	if testing.Verbose() {
		logp.LogInit(logp.LOG_DEBUG, "", false, true, []string{"topology", "output_elasticsearch"})
	}

	elasticsearchOutput1 := createElasticsearchConnection(0, 0)
	elasticsearchOutput2 := createElasticsearchConnection(0, 0)
	elasticsearchOutput3 := createElasticsearchConnection(0, 0)

	elasticsearchOutput1.PublishIPs("proxy1", []string{"10.1.0.4"})
	elasticsearchOutput2.PublishIPs("proxy2", []string{"10.1.0.9",
		"fe80::4e8d:79ff:fef2:de6a"})
	elasticsearchOutput3.PublishIPs("proxy3", []string{"10.1.0.10"})

	name2 := elasticsearchOutput3.GetNameByIP("10.1.0.9")
	if name2 != "proxy2" {
		t.Errorf("Failed to update proxy2 in topology: name=%s", name2)
	}

	elasticsearchOutput1.PublishIPs("proxy1", []string{"10.1.0.4"})
	elasticsearchOutput2.PublishIPs("proxy2", []string{"10.1.0.9"})
	elasticsearchOutput3.PublishIPs("proxy3", []string{"192.168.1.2"})

	name3 := elasticsearchOutput3.GetNameByIP("192.168.1.2")
	if name3 != "proxy3" {
		t.Errorf("Failed to add a new IP")
	}

	name3 = elasticsearchOutput3.GetNameByIP("10.1.0.10")
	if name3 != "" {
		t.Errorf("Failed to delete old IP of proxy3: %s", name3)
	}

	name2 = elasticsearchOutput3.GetNameByIP("fe80::4e8d:79ff:fef2:de6a")
	if name2 != "" {
		t.Errorf("Failed to delete old IP of proxy2: %s", name2)
	}
}

func TestOneEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping events publish in short mode, because they require Elasticsearch")
	}
	if testing.Verbose() {
		logp.LogInit(logp.LOG_DEBUG, "", false, true, []string{"elasticsearch", "output_elasticsearch"})
	}

	ts := time.Now()

	elasticsearchOutput := createElasticsearchConnection(0, 0)

	event := common.MapStr{}
	event["type"] = "redis"
	event["status"] = "OK"
	event["responsetime"] = 34
	event["dst_ip"] = "192.168.21.1"
	event["dst_port"] = 6379
	event["src_ip"] = "192.168.22.2"
	event["src_port"] = 6378
	event["shipper"] = "appserver1"
	r := common.MapStr{}
	r["request"] = "MGET key1"
	r["response"] = "value1"

	index := fmt.Sprintf("%s-%d.%02d.%02d", elasticsearchOutput.Index, ts.Year(), ts.Month(), ts.Day())
	logp.Debug("output_elasticsearch", "index = %s", index)
	elasticsearchOutput.Conn.CreateIndex(index, common.MapStr{
		"settings": common.MapStr{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
	})

	err := elasticsearchOutput.PublishEvent(nil, ts, event)
	if err != nil {
		t.Errorf("Failed to publish the event: %s", err)
	}

	// give control to the other goroutine, otherwise the refresh happens
	// before the refresh. We should find a better solution for this.
	time.Sleep(200 * time.Millisecond)

	_, err = elasticsearchOutput.Conn.Refresh(index)
	if err != nil {
		t.Errorf("Failed to refresh: %s", err)
	}

	defer func() {
		_, err = elasticsearchOutput.Conn.Delete(index, "", "", nil)
		if err != nil {
			t.Errorf("Failed to delete index: %s", err)
		}
	}()

	params := map[string]string{
		"q": "shipper:appserver1",
	}
	resp, err := elasticsearchOutput.Conn.SearchURI(index, "", params)

	if err != nil {
		t.Errorf("Failed to query elasticsearch for index(%s): %s", index, err)
		return
	}
	logp.Debug("output_elasticsearch", "resp = %s", resp)
	if resp.Hits.Total != 1 {
		t.Errorf("Wrong number of results: %d", resp.Hits.Total)
	}

}

func TestEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping events publish in short mode, because they require Elasticsearch")
	}
	if testing.Verbose() {
		logp.LogInit(logp.LOG_DEBUG, "", false, true, []string{"topology", "output_elasticsearch"})
	}

	ts := time.Now()

	elasticsearchOutput := createElasticsearchConnection(0, 0)

	event := common.MapStr{}
	event["type"] = "redis"
	event["status"] = "OK"
	event["responsetime"] = 34
	event["dst_ip"] = "192.168.21.1"
	event["dst_port"] = 6379
	event["src_ip"] = "192.168.22.2"
	event["src_port"] = 6378
	event["shipper"] = "appserver1"
	r := common.MapStr{}
	r["request"] = "MGET key1"
	r["response"] = "value1"
	event["redis"] = r

	index := fmt.Sprintf("%s-%d.%02d.%02d", elasticsearchOutput.Index, ts.Year(), ts.Month(), ts.Day())
	elasticsearchOutput.Conn.CreateIndex(index, common.MapStr{
		"settings": common.MapStr{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
	})

	err := elasticsearchOutput.PublishEvent(nil, ts, event)
	if err != nil {
		t.Errorf("Failed to publish the event: %s", err)
	}

	r = common.MapStr{}
	r["request"] = "MSET key1 value1"
	r["response"] = 0
	event["redis"] = r

	err = elasticsearchOutput.PublishEvent(nil, ts, event)
	if err != nil {
		t.Errorf("Failed to publish the event: %s", err)
	}

	// give control to the other goroutine, otherwise the refresh happens
	// before the refresh. We should find a better solution for this.
	time.Sleep(200 * time.Millisecond)

	elasticsearchOutput.Conn.Refresh(index)

	params := map[string]string{
		"q": "shipper:appserver1",
	}

	defer func() {
		_, err = elasticsearchOutput.Conn.Delete(index, "", "", nil)
		if err != nil {
			t.Errorf("Failed to delete index: %s", err)
		}
	}()

	resp, err := elasticsearchOutput.Conn.SearchURI(index, "", params)

	if err != nil {
		t.Errorf("Failed to query elasticsearch: %s", err)
	}
	if resp.Hits.Total != 2 {
		t.Errorf("Wrong number of results: %d", resp.Hits.Total)
	}
}

func testBulkWithParams(t *testing.T, elasticsearchOutput elasticsearchOutput) {
	ts := time.Now()
	index := fmt.Sprintf("%s-%d.%02d.%02d", elasticsearchOutput.Index, ts.Year(), ts.Month(), ts.Day())

	elasticsearchOutput.Conn.CreateIndex(index, common.MapStr{
		"settings": common.MapStr{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
	})

	for i := 0; i < 10; i++ {

		event := common.MapStr{}
		event["type"] = "redis"
		event["status"] = "OK"
		event["responsetime"] = 34
		event["dst_ip"] = "192.168.21.1"
		event["dst_port"] = 6379
		event["src_ip"] = "192.168.22.2"
		event["src_port"] = 6378
		event["shipper"] = "appserver" + strconv.Itoa(i)
		r := common.MapStr{}
		r["request"] = "MGET key" + strconv.Itoa(i)
		r["response"] = "value" + strconv.Itoa(i)
		event["redis"] = r

		err := elasticsearchOutput.PublishEvent(nil, ts, event)
		if err != nil {
			t.Errorf("Failed to publish the event: %s", err)
		}

	}

	// give control to the other goroutine, otherwise the refresh happens
	// before the index. We should find a better solution for this.
	time.Sleep(200 * time.Millisecond)

	elasticsearchOutput.Conn.Refresh(index)

	params := map[string]string{
		"q": "type:redis",
	}

	defer func() {
		_, err := elasticsearchOutput.Conn.Delete(index, "", "", nil)
		if err != nil {
			t.Errorf("Failed to delete index: %s", err)
		}
	}()

	resp, err := elasticsearchOutput.Conn.SearchURI(index, "", params)

	if err != nil {
		t.Errorf("Failed to query elasticsearch: %s", err)
		return
	}
	if resp.Hits.Total != 10 {
		t.Errorf("Wrong number of results: %d", resp.Hits.Total)
	}
}

func TestBulkEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping events publish in short mode, because they require Elasticsearch")
	}
	if testing.Verbose() {
		logp.LogInit(logp.LOG_DEBUG, "", false, true, []string{"topology", "output_elasticsearch", "elasticsearch"})
	}

	elasticsearchOutput := createElasticsearchConnection(50, 2)
	testBulkWithParams(t, elasticsearchOutput)

	elasticsearchOutput = createElasticsearchConnection(50, 1000)
	testBulkWithParams(t, elasticsearchOutput)

	elasticsearchOutput = createElasticsearchConnection(50, 5)
	testBulkWithParams(t, elasticsearchOutput)
}

func TestEnableTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping events publish in short mode, because they require Elasticsearch")
	}
	if testing.Verbose() {
		logp.LogInit(logp.LOG_DEBUG, "", false, true, []string{"topology", "output_elasticsearch", "elasticsearch"})
	}

	elasticsearchOutput := createElasticsearchConnection(0, 0)
	elasticsearchOutput.Conn.Delete(".packetbeat-topology", "", "", nil)

	err := elasticsearchOutput.EnableTTL()
	if err != nil {
		t.Errorf("Fail to enable TTL: %s", err)
	}

	// should succeed also when index already exists
	err = elasticsearchOutput.EnableTTL()
	if err != nil {
		t.Errorf("Fail to enable TTL: %s", err)
	}

}

func TestGetUrl(t *testing.T) {

	// List of inputs / outputs that must match after fetching url
	// Setting a path without a scheme is not allowed. Example: 192.168.1.1:9200/hello
	inputOutput := map[string]string{
		// shema + hostname
		"":                     "http://localhost:9200",
		"http://localhost":     "http://localhost:9200",
		"http://localhost:80":  "http://localhost:80",
		"http://localhost:80/": "http://localhost:80/",
		"http://localhost/":    "http://localhost:9200/",

		// no schema + hostname
		"localhost":        "http://localhost:9200",
		"localhost:80":     "http://localhost:80",
		"localhost:80/":    "http://localhost:80/",
		"localhost/":       "http://localhost:9200/",
		"localhost/mypath": "http://localhost:9200/mypath",

		// shema + ipv4
		"http://192.168.1.1:80":        "http://192.168.1.1:80",
		"https://192.168.1.1:80/hello": "https://192.168.1.1:80/hello",
		"http://192.168.1.1":           "http://192.168.1.1:9200",
		"http://192.168.1.1/hello":     "http://192.168.1.1:9200/hello",

		// no schema + ipv4
		"192.168.1.1":          "http://192.168.1.1:9200",
		"192.168.1.1:80":       "http://192.168.1.1:80",
		"192.168.1.1/hello":    "http://192.168.1.1:9200/hello",
		"192.168.1.1:80/hello": "http://192.168.1.1:80/hello",

		// schema + ipv6
		"http://[2001:db8::1]:80":                              "http://[2001:db8::1]:80",
		"http://[2001:db8::1]":                                 "http://[2001:db8::1]:9200",
		"https://[2001:db8::1]:9200":                           "https://[2001:db8::1]:9200",
		"http://FE80:0000:0000:0000:0202:B3FF:FE1E:8329":       "http://[FE80:0000:0000:0000:0202:B3FF:FE1E:8329]:9200",
		"http://[2001:db8::1]:80/hello":                        "http://[2001:db8::1]:80/hello",
		"http://[2001:db8::1]/hello":                           "http://[2001:db8::1]:9200/hello",
		"https://[2001:db8::1]:9200/hello":                     "https://[2001:db8::1]:9200/hello",
		"http://FE80:0000:0000:0000:0202:B3FF:FE1E:8329/hello": "http://[FE80:0000:0000:0000:0202:B3FF:FE1E:8329]:9200/hello",

		// no schema + ipv6
		"2001:db8::1":            "http://[2001:db8::1]:9200",
		"[2001:db8::1]:80":       "http://[2001:db8::1]:80",
		"[2001:db8::1]":          "http://[2001:db8::1]:9200",
		"2001:db8::1/hello":      "http://[2001:db8::1]:9200/hello",
		"[2001:db8::1]:80/hello": "http://[2001:db8::1]:80/hello",
		"[2001:db8::1]/hello":    "http://[2001:db8::1]:9200/hello",
	}

	for input, output := range inputOutput {
		urlNew, err := getURL("http", "", input)
		assert.Nil(t, err)
		assert.Equal(t, output, urlNew, fmt.Sprintf("input: %v", input))
	}

	inputOutputWithDefaults := map[string]string{
		"http://localhost":                          "http://localhost:9200/hello",
		"192.156.4.5":                               "https://192.156.4.5:9200/hello",
		"http://username:password@es.found.io:9324": "http://username:password@es.found.io:9324/hello",
	}
	for input, output := range inputOutputWithDefaults {
		urlNew, err := getURL("https", "/hello", input)
		assert.Nil(t, err)
		assert.Equal(t, output, urlNew)
	}
}
