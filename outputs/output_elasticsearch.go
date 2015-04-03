package outputs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/infrabeat/common"
	"github.com/elastic/infrabeat/logp"

	"github.com/packetbeat/elastigo/api"
	"github.com/packetbeat/elastigo/core"
)

type ElasticsearchOutputType struct {
	OutputInterface
	Index          string
	TopologyExpire int

	TopologyMap map[string]string
}

type PublishedTopology struct {
	Name string
	IPs  string
}

func (out *ElasticsearchOutputType) Init(config MothershipConfig, topology_expire int) error {

	api.Domain = config.Host
	api.Port = fmt.Sprintf("%d", config.Port)
	api.Username = config.Username
	api.Password = config.Password
	api.BasePath = config.Path

	if config.Protocol != "" {
		api.Protocol = config.Protocol
	}

	if config.Index != "" {
		out.Index = config.Index
	} else {
		out.Index = "packetbeat"
	}

	out.TopologyExpire = 15000
	if topology_expire != 0 {
		out.TopologyExpire = topology_expire /*sec*/ * 1000 // millisec
	}

	err := out.EnableTTL()
	if err != nil {
		logp.Err("Fail to set _ttl mapping: %s", err)
		return err
	}

	logp.Info("[ElasticsearchOutput] Using Elasticsearch %s://%s:%s%s", api.Protocol, api.Domain, api.Port, api.BasePath)
	logp.Info("[ElasticsearchOutput] Using index pattern [%s-]YYYY.MM.DD", out.Index)
	logp.Info("[ElasticsearchOutput] Topology expires after %ds", out.TopologyExpire/1000)

	return nil
}

func (out *ElasticsearchOutputType) EnableTTL() error {
	setting := map[string]interface{}{
		"server-ip": map[string]interface{}{
			"_ttl": map[string]string{"enabled": "true", "default": "15000"},
		},
	}

	// Make sure the index exists, but ignore errors (probably exists already)
	core.Index(".packetbeat-topology", "", "", nil, nil)

	_, err := core.Index(".packetbeat-topology", "server-ip", "_mapping", nil, setting)
	if err != nil {
		return err
	}
	return nil
}

func (out *ElasticsearchOutputType) GetNameByIP(ip string) string {
	name, exists := out.TopologyMap[ip]
	if !exists {
		return ""
	}
	return name
}
func (out *ElasticsearchOutputType) PublishIPs(name string, localAddrs []string) error {
	logp.Debug("output_elasticsearch", "Publish IPs %s with expiration time %d", localAddrs, out.TopologyExpire)
	_, err := core.IndexWithParameters(
		".packetbeat-topology", /*index*/
		"server-ip",            /*type*/
		name,                   /* id */
		"",                     /*parent id */
		0,                      /* version */
		"",                     /* op_type */
		"",                     /* routing */
		"",                     /* timestamp */
		out.TopologyExpire,     /*ttl*/
		"",                     /* percolate */
		"",                     /* timeout */
		false,                  /*refresh */
		nil,                    /*args */
		PublishedTopology{name, strings.Join(localAddrs, ",")} /* data */)

	if err != nil {
		logp.Err("Fail to publish IP addresses: %s", err)
		return err
	}

	out.UpdateLocalTopologyMap()

	return nil
}

func (out *ElasticsearchOutputType) UpdateLocalTopologyMap() {

	// get all agents IPs from Elasticsearch
	TopologyMapTmp := make(map[string]string)

	res, err := core.SearchUri(".packetbeat-topology", "server-ip", nil)
	if err == nil {
		for _, server := range res.Hits.Hits {
			var pub PublishedTopology
			err = json.Unmarshal([]byte(*server.Source), &pub)
			if err != nil {
				logp.Err("json.Unmarshal fails with: %s", err)
			}
			// add mapping
			ipaddrs := strings.Split(pub.IPs, ",")
			for _, addr := range ipaddrs {
				TopologyMapTmp[addr] = pub.Name
			}
		}
	} else {
		logp.Err("Getting topology map fails with: %s", err)
	}

	// update topology map
	out.TopologyMap = TopologyMapTmp

	logp.Debug("output_elasticsearch", "Topology map %s", out.TopologyMap)
}

func (out *ElasticsearchOutputType) PublishEvent(ts time.Time, event common.MapStr) error {

	index := fmt.Sprintf("%s-%d.%02d.%02d", out.Index, ts.Year(), ts.Month(), ts.Day())
	_, err := core.Index(index, event["type"].(string), "", nil, event)
	logp.Debug("output_elasticsearch", "Publish event")
	return err
}
