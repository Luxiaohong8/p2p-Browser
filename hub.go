// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

type Stats struct {
	CDN uint64
	P2p uint64
}

// hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	//clients map[*Client]bool
	clients sync.Map

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	//算法可视化客户端
	visclients map[*VisClient]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	registerVis chan *VisClient

	// Unregister requests from clients.
	unregisterVis chan *VisClient

	VisClientNum uint16

	//count of client
	ClientNum uint16

	P2pConfig struct {
		Live struct {
			MaxLayers       int
			MinRPNodes      int
			MaxRPNodes      int
			DefaultUploadBW int64
			RPBWThreshold   int64
			Substreams      int
		}
	}

	fastMesh FastMesh

	Room string

	Mode string

	Stats Stats
}

func newHub(room, mode string) *Hub {
	return &Hub{
		broadcast:     make(chan []byte),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		visclients:    make(map[*VisClient]bool),
		registerVis:   make(chan *VisClient),
		unregisterVis: make(chan *VisClient),
		Room:          room,
		Mode:          mode,
	}
}

// send json object to a client with peerId
func (this *Hub) sendJsonToClient(peerId string, value interface{}) {
	b, err := json.Marshal(value)
	if err != nil {
		//logrus.Errorf("[Client.jsonResponse] Marshal err: %s", err.Error())
		return
	}
	client, ok := this.clients.Load(peerId)
	if !ok {
		log.Printf("sendJsonToClient error")
		return
	}
	if err := client.(*Client).sendMessage(b); err != nil {
		//logrus.Errorf("[Client.jsonResponse] sendMessage err: %s", err.Error())
	}
	//if err := client.(*Client).conn.WriteJSON(value); err != nil {
	//	//logrus.Errorf("[Client.jsonResponse] sendMessage err: %s", err.Error())
	//}
}

func (this *Hub) run() {
	for {
		select {
		case client := <-this.register:
			this.doRegister(client)
		case client := <-this.unregister:
			this.doUnregister(client)
		case visclient := <-this.registerVis:
			this.visclients[visclient] = true
			this.VisClientNum++
			log.Printf("registerVis")
		case visclient := <-this.unregisterVis:
			if _, ok := this.visclients[visclient]; ok {
				delete(this.visclients, visclient)
				close(visclient.send)
				this.VisClientNum--
			}
			//case message := <-this.broadcast:
			//	log.Printf("hub broadcast message: %s visclients.length %d", string(message), len(this.visclients))
			//	for visclient := range this.visclients {
			//		select {
			//		case visclient.send <- message:
			//			log.Printf("case visclient.send <- message")
			//		default:
			//			log.Printf("case default")
			//			close(visclient.send)
			//			delete(this.visclients, visclient)
			//		}
			//	}

		}

		// 遍历map
		fmt.Println("######################### All clients in the room###########################")
		this.clients.Range(func(key, client interface{}) bool {
			fmt.Println(key, client.(*Client).conn.RemoteAddr().String(), "level:", client.(*Client).treeNode.layer, "fathernode:", client.(*Client).treeNode.parents)
			//fmt.Println(key, )
			return true
		})
	}
}

func (this *Hub) doRegister(client *Client) {
	//	logrus.Debugf("[Hub.doRegister] %s", client.id)
	if client.PeerId != "" {
		log.Printf("id: %s", client.PeerId)
		this.clients.Store(client.PeerId, client)
		this.ClientNum++
		log.Printf("VisClientNum: %d", this.VisClientNum)
	}
}

func (this *Hub) doUnregister(client *Client) {
	//	logrus.Debugf("[Hub.doUnregister] %s", client.id)

	if client.PeerId == "" {
		return
	}

	_, ok := this.clients.Load(client.PeerId)

	if ok {
		//delRecordCh <- client.id

		//记录子流信息
		for _, v := range client.treeNode.parents {
			this.fastMesh.DeleteEdge(v, &client.treeNode)
			parent, ok := this.clients.Load(v.id)
			if ok {
				parent.(*Client).ResidualBW += client.subStreamRate * int64(client.streamMap[v.id])
				log.Warnf("client %v ResidualBW %v", parent.(*Client).PeerId, parent.(*Client).ResidualBW)
			}
		}
		for _, v := range client.treeNode.children {
			this.fastMesh.DeleteEdge(&client.treeNode, v)
			child, ok := this.clients.Load(v.id)
			if ok {
				child.(*Client).streamMap[client.PeerId] = 0
			}
		}

		this.clients.Delete(client.PeerId)
		close(client.send)
		this.ClientNum--
	}

	//广播节点离开信息
	if this.VisClientNum > 0 {
		//向所有visclient广播
		resp := map[string]interface{}{
			"action": "leave",
			"node": map[string]interface{}{
				"id": client.PeerId,
			},
		}
		b, err := json.Marshal(resp)
		if err != nil {
			panic(err)
		}
		log.Printf("broadcast to visclients: %s", string(b))
		//this.broadcast <- b
		for visclient := range this.visclients {
			visclient.send <- b
		}
	}
}
