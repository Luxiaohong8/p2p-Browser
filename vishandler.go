package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.TextFormatter{ForceColors: true})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	//log.SetLevel(log.WarnLevel)

}

type VisNode struct {
	Id      string        `json:"id"`
	Parents []interface{} `json:"parents"`
	Info    VisNodeInfo   `json:"info"`
}

type VisNodeInfo struct {
	IP       string `json:"IP"`
	ISP      string `json:"ISP"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	UploadBW int64  `json:"ul_bw"`
}

func (this *VisClient) handle(message []byte) {
	log.Printf("[VisClient.handle] %s", string(message))
	action := struct {
		Action string `json:"action"`
	}{}
	if err := json.Unmarshal(message, &action); err != nil {
		//logrus.Errorf("[Client.handle] json.Unmarshal %s", err.Error())
		log.Printf("json.Unmarshal %s", err.Error())
		return
	}

	this.CreateHandler(action.Action, message).Handle()
}

func (this *VisClient) CreateHandler(action string, payload []byte) Handler {

	//log.WithFields(log.Fields{
	//	"animal": "walrus",
	//}).Warn("A walrus appears")

	switch action {
	case "get_topology":
		return &GETTOPOHandler{this}
	case "get_stats":
		return &GETStatsHandler{this}
	}

	return &ExceptionHandler{message: fmt.Sprintf("visclient unregnized action %s", action)}
}

type GETTOPOHandler struct {
	visclient *VisClient
}

func (this *GETTOPOHandler) Handle() {

	nodes := make([]VisNode, 0)
	allnode := 0
	p2pnode := 0

	this.visclient.hub.clients.Range(func(key, value interface{}) bool {
		client := value.(*Client)
		log.Printf("[GETTOPOHandler] v.id %s", client.PeerId)
		node := VisNode{
			Id:      client.PeerId,
			Parents: make([]interface{}, 0),
			Info: VisNodeInfo{
				IP:       client.conn.RemoteAddr().String(),
				ISP:      client.IpInfo.ISP,
				Country:  client.IpInfo.Country,
				Province: client.IpInfo.Province,
				City:     client.IpInfo.City,
				UploadBW: client.UploadBW,
			},
		}

		allnode = allnode + 1
		if client.isP2P {
			p2pnode = p2pnode + 1
		}
		for _, parent := range client.treeNode.parents {
			//log.Warn(client.streamMap[parent.id])
			node.Parents = append(node.Parents, map[string]interface{}{
				"id":         parent.id,
				"substreams": client.streamMap[parent.id],
			})
		}
		nodes = append(nodes, node)
		return true
	})

	p2pratio := 100 * float64(p2pnode) / float64(allnode)
	log.Printf("--p2p ratio-----%d,%d,%f %%", allnode, p2pnode, p2pratio)
	resp := map[string]interface{}{
		"action":       "topology",
		"nodes":        nodes,
		"totalstreams": this.visclient.hub.P2pConfig.Live.Substreams,
		"p2pratio":     p2pratio,
	}
	this.visclient.conn.WriteJSON(resp)
}

type GETStatsHandler struct {
	visclient *VisClient
}

func (this *GETStatsHandler) Handle() {

	resp := map[string]interface{}{
		"action": "statistics",
		"result": map[string]interface{}{
			"source": this.visclient.hub.Stats.CDN,
			"p2p":    this.visclient.hub.Stats.P2p,
		},
	}
	this.visclient.conn.WriteJSON(resp)
}

func serveVisHttp(hub *Hub, w http.ResponseWriter, r *http.Request) {
	nodes := make([]VisNode, 0)
	hub.clients.Range(func(key, value interface{}) bool {
		client := value.(*Client)
		log.Printf("[GETTOPOHandler] v.id %s", client.PeerId)
		node := VisNode{
			Id:      client.PeerId,
			Parents: make([]interface{}, 0),
			Info: VisNodeInfo{
				ISP:      client.IpInfo.ISP,
				Country:  client.IpInfo.Country,
				Province: client.IpInfo.Province,
				City:     client.IpInfo.City,
			},
		}
		for _, parent := range client.treeNode.parents {
			node.Parents = append(node.Parents, parent.id)
		}
		nodes = append(nodes, node)
		return true
	})
	resp := map[string]interface{}{
		"action": "topology",
		"nodes":  nodes,
	}
	b, _ := json.Marshal(resp)
	w.Write(b)
}
