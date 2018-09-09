package main

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"os"
	"strings"
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

type Handler interface {
	Handle()
}

type EnterMsg struct {
	Channel string `json:"channel"`
	Ul_bw   int64  `json:"ul_bw"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
	Os      string `json:"os"`
}

type SignalMsg struct {
	To_peer_id string      `json:"to_peer_id"`
	Data       interface{} `json:"data"`
}

type DCOpenCloseMsg struct {
	Dc_id       string `json:"dc_id"`
	Substreams  int    `json:"substreams"`
	Stream_rate int64  `json:"stream_rate"`
}

type StatMsg struct {
	//Level float32              `json:"level"`
	Source uint32           `json:"source"`
	P2p    uint32           `json:"p2p"`
	Ul_srs map[string]int64 `json:"ul_srs"`
	Plr    float32          `json:"plr"`
	Bw     int64            `json:"bw"`
}

type AdoptMsg struct {
	To_peer_id string `json:"to_peer_id"`
}

func (this *Client) handle(message []byte) {
	log.Printf("[Client.handle] %s", string(message))
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

func (this *Client) CreateHandler(action string, payload []byte) Handler {
	//	logrus.Debugf("[Client.CreateHandler] action:%s, toPeerId: %s", action, toPeerId)

	//this.hub.statsd(action)

	//log.WithFields(log.Fields{
	//	"animal": "walrus",
	//}).Warn("A walrus appears")

	switch action {
	case "enter":
		msg := EnterMsg{}
		if err := json.Unmarshal(payload, &msg); err != nil {
			//logrus.Errorf("[PullHandler.Handle] json.Unmarshal %s", err.Error())
			return &ExceptionHandler{err.Error()}
		}
		return &EnterHandler{msg, this}
	case "signal":
		msg := SignalMsg{}
		if err := json.Unmarshal(payload, &msg); err != nil {
			//logrus.Errorf("[PullHandler.Handle] json.Unmarshal %s", err.Error())

			return &ExceptionHandler{err.Error()}
		}
		return &SignalHandler{msg, this}
	case "dc_opened":
		msg := DCOpenCloseMsg{}
		if err := json.Unmarshal(payload, &msg); err != nil {
			//logrus.Errorf("[PullHandler.Handle] json.Unmarshal %s", err.Error())

			return &ExceptionHandler{err.Error()}
		}
		return &DCOpenHandler{msg, this}
	case "dc_closed":
		msg := DCOpenCloseMsg{}
		if err := json.Unmarshal(payload, &msg); err != nil {
			//logrus.Errorf("[PullHandler.Handle] json.Unmarshal %s", err.Error())

			return &ExceptionHandler{err.Error()}
		}
		return &DCCloseHandler{msg, this}
	case "adopt":
		msg := AdoptMsg{}
		if err := json.Unmarshal(payload, &msg); err != nil {
			//logrus.Errorf("[PullHandler.Handle] json.Unmarshal %s", err.Error())

			return &ExceptionHandler{err.Error()}
		}
		return &AdoptHandler{msg, this}
	case "get_parents":
		return &GetParentsHandler{this}
	case "statistics":
		msg := StatMsg{}
		if err := json.Unmarshal(payload, &msg); err != nil {
			//logrus.Errorf("[PullHandler.Handle] json.Unmarshal %s", err.Error())

			return &ExceptionHandler{err.Error()}
		}
		return &StatHandler{msg, this}
	}

	return &ExceptionHandler{message: fmt.Sprintf("unregnized action %s", action)}
}

type ExceptionHandler struct {
	message string
}

func (this *ExceptionHandler) Handle() {
	log.Printf("[ExceptionHandler] err %s", this.message)
}

type EnterHandler struct {
	message EnterMsg
	client  *Client
}

func (this *EnterHandler) Handle() {
	log.Printf("EnterHandler Handle %v", this.message)

	if this.message.Ul_bw != 0 {
		this.client.UploadBW = this.message.Ul_bw
		this.client.ResidualBW = this.message.Ul_bw
	}

	response := map[string]interface{}{
		"action":     "accept",
		"peer_id":    this.client.PeerId,
		"speed_test": "",
		"substreams": this.client.hub.P2pConfig.Live.Substreams,
	}

	this.client.jsonResponse(response)

	//fmt.Printf("ClientNum %v\n", this.client.hub.ClientNum)
	if this.client.hub.VisClientNum > 0 {
		//向所有visclient广播
		ipInfo := this.client.IpInfo
		node := VisNode{
			Id: this.client.PeerId,
			Info: VisNodeInfo{
				ISP:      ipInfo.ISP,
				Country:  ipInfo.Country,
				Province: ipInfo.Province,
				City:     ipInfo.City,
				UploadBW: this.client.UploadBW,
			},
		}
		resp := map[string]interface{}{
			"action": "join",
			"node":   node,
		}
		b, err := json.Marshal(resp)
		if err != nil {
			panic(err)
		}
		log.Printf("broadcast to visclients: %s", string(b))
		//this.broadcast <- b
		for visclient := range this.client.hub.visclients {
			visclient.send <- b
		}
	}

}

type SignalHandler struct {
	message SignalMsg
	client  *Client
}

func (this *SignalHandler) Handle() {
	//log.Printf("SignalHandler Handle %v", this.message)

	response := map[string]interface{}{
		"action":       "signal",
		"from_peer_id": this.client.PeerId, //对等端的Id
		"data":         this.message.Data,  //需要传送的数据
	}
	this.client.hub.sendJsonToClient(this.message.To_peer_id, response)
}

type DCOpenHandler struct {
	message DCOpenCloseMsg
	client  *Client
}

func (this *DCOpenHandler) Handle() {
	fmt.Println(this.message.Dc_id)
	s := strings.Split(this.message.Dc_id, "-")
	child, ok := this.client.hub.clients.Load(s[0])
	//记录子流信息
	child.(*Client).streamMap[s[1]] = this.message.Substreams
	child.(*Client).subStreamRate = this.message.Stream_rate

	if ok {
		parent, ok := this.client.hub.clients.Load(s[1])
		if ok {
			this.client.hub.fastMesh.AddEdge(&parent.(*Client).treeNode, &child.(*Client).treeNode)
			parent.(*Client).ResidualBW -= this.message.Stream_rate * int64(this.message.Substreams)
			log.Warnf("client %v ResidualBW %v Stream_rate %v Substreams %v", parent.(*Client).PeerId, parent.(*Client).ResidualBW, this.message.Stream_rate, this.message.Substreams)
		}

		this.client.isP2P = true
		//广播节点连接信息
		if this.client.hub.VisClientNum > 0 {
			//向所有visclient广播
			resp := map[string]interface{}{
				"action": "connect",
				"edge": map[string]interface{}{
					"from":       parent.(*Client).PeerId,
					"to":         child.(*Client).PeerId,
					"substreams": this.message.Substreams,
				},
			}
			b, err := json.Marshal(resp)
			if err != nil {
				panic(err)
			}
			log.Printf("broadcast to visclients: %s", string(b))
			//this.broadcast <- b
			for visclient := range this.client.hub.visclients {
				visclient.send <- b
			}
		}
	}
	//log.Printf("node %s layer %d", s[0], child.(*Client).treeNode.layer)

}

type DCCloseHandler struct {
	message DCOpenCloseMsg
	client  *Client
}

func (this *DCCloseHandler) Handle() {
	fmt.Println(this.message.Dc_id)
	s := strings.Split(this.message.Dc_id, "-")
	child, ok := this.client.hub.clients.Load(s[0])
	if ok {
		parent, ok := this.client.hub.clients.Load(s[1])
		if ok {
			//记录子流信息
			child.(*Client).streamMap[s[1]] = 0
			this.client.hub.fastMesh.DeleteEdge(&parent.(*Client).treeNode, &child.(*Client).treeNode)
			parent.(*Client).ResidualBW += this.message.Stream_rate * int64(this.message.Substreams)
			log.Warnf("client %v ResidualBW %v", parent.(*Client).PeerId, parent.(*Client).ResidualBW)
		}
		this.client.isP2P = true
		//广播节点断开连接信息
		if this.client.hub.VisClientNum > 0 && parent != nil && child != nil {
			//向所有visclient广播
			resp := map[string]interface{}{
				"action": "disconnect",
				"edge": map[string]interface{}{
					"from": s[1],
					"to":   s[0],
				},
			}
			b, err := json.Marshal(resp)
			if err != nil {
				panic(err)
			}
			log.Printf("broadcast to visclients: %s", string(b))
			//this.broadcast <- b
			for visclient := range this.client.hub.visclients {
				visclient.send <- b
			}
		}
	}

	//log.Printf("node %s layer %d", s[0], child.(*Client).treeNode.layer)
}

type AdoptHandler struct {
	message AdoptMsg
	client  *Client
}

func (this *AdoptHandler) Handle() {
	log.Printf("AdoptHandler Handle %v", this.message)
	response := map[string]interface{}{
		"action":      "adopt",
		"parent_id":   this.client.PeerId,
		"residual_bw": this.client.ResidualBW,
	}
	this.client.hub.sendJsonToClient(this.message.To_peer_id, response)
}

type GetParentsHandler struct {
	client *Client
}

func (this *GetParentsHandler) Handle() {
	log.Printf("GetParentsHandler Handle")
	this.client.isActive = true
	if this.client.hub.ClientNum >= 2 {
		parents := this.client.hub.GenParents(this.client)
		var sliceP []interface{}
		for _, value := range parents {
			sliceP = append(sliceP, map[string]interface{}{"peer_id": value.PeerId, "residual_bw": value.ResidualBW}) //当前带宽
		}
		log.Printf("parents: %v", sliceP)
		this.client.hub.sendJsonToClient(this.client.PeerId, map[string]interface{}{
			"action": "parents",
			"nodes":  sliceP,
		})
		//delete inactive node in room
		this.client.hub.clients.Range(func(key, c interface{}) bool {
			if !c.(*Client).isActive {
				this.client.hub.clients.Delete(c.(*Client).PeerId)
				c.(*Client).hub.ClientNum--
			}
			return true
		})
	}
}

type StatHandler struct {
	message StatMsg
	client  *Client
}

func (this *StatHandler) Handle() {
	log.Printf("StatHandler Handle")
	this.client.Stat = this.message
	log.Println(this.message.Bw)
	if this.message.Bw != 0 {
		this.client.UploadBW = this.message.Bw
	}
	this.client.hub.Stats.CDN += uint64(this.message.Source)
	this.client.hub.Stats.P2p += uint64(this.message.P2p)
	this.client.ResidualBW = this.client.getResidualBW()
	//广播节点连接信息
	//if this.client.hub.VisClientNum > 0 {
	//	//向所有visclient广播
	//	resp := map[string]interface{} {
	//		"action": "statistics",
	//		"id": this.client.PeerId,
	//		"info": this.message,
	//	}
	//	b, err := json.Marshal(resp)
	//	if err != nil {
	//		panic(err)
	//	}
	//	log.Printf("broadcast to visclients: %s", string(b))
	//	//this.broadcast <- b
	//	for visclient := range this.client.hub.visclients {
	//		visclient.send <- b
	//	}
	//}
}
