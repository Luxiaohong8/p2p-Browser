// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	//"github.com/sirupsen/logrus"

	"github.com/mohong122/ip2region/binding/golang"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 5120
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool { //跨域
		return true
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	PeerId   string //唯一标识
	UploadBW int64  //单位bps
	Level    int    //节点在网络结构中的层级
	IpInfo   IpInfo

	treeNode TreeNode

	isActive bool //记录是否是活跃节点，用于筛选
	isP2P    bool //记录是否p2p

	Stat StatMsg

	ResidualBW int64

	streamMap     map[string]int //peerId --> substreams
	subStreamRate int64
}

type IpInfo struct {
	Country  string
	Region   string
	Province string
	City     string
	ISP      string
}

func (c *Client) sendMessage(msg []byte) error {
	select {
	case c.send <- msg:
		return nil
	default:
		close(c.send)

		//		delete(c.hub.clients, c.id)
		c.hub.clients.Delete(c.PeerId)

		//		logrus.Errorf("[Client.sendMessage] send to chan error")
		return fmt.Errorf("send to chan error")
	}
}

func (this *Client) jsonResponse(value interface{}) {
	b, err := json.Marshal(value)
	if err != nil {
		//logrus.Errorf("[Client.jsonResponse] Marshal err: %s", err.Error())
		return
	}
	if err := this.sendMessage(b); err != nil {
		//logrus.Errorf("[Client.jsonResponse] sendMessage err: %s", err.Error())
	}
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		//c.hub.broadcast <- message
		c.handle(message)
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			//n := len(c.send)
			//for i := 0; i < n; i++ {
			//	temp := <-c.send
			//	fmt.Printf("temp %s", message)
			//	w.Write(newline)
			//	w.Write(temp)
			//}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{
		hub:        hub,
		conn:       conn,
		send:       make(chan []byte, 256),
		PeerId:     UniqueId(),
		UploadBW:   hub.P2pConfig.Live.DefaultUploadBW, //默认值，应该由节点上报
		ResidualBW: hub.P2pConfig.Live.DefaultUploadBW,
		streamMap:  make(map[string]int),
		isP2P:      false,
		isActive:   false,
	}
	client.treeNode.id = client.PeerId

	//记录client的region信息
	fmt.Printf("RemoteAddr %s\n", client.conn.RemoteAddr().String())
	region, err := ip2region.New("ip2region.db")
	defer region.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	ip, err := region.BtreeSearch(client.conn.RemoteAddr().String())
	if err == nil {
		ipInfo := &client.IpInfo
		ipInfo.Country = ip.Country
		ipInfo.Region = ip.Region
		ipInfo.Province = ip.Province
		ipInfo.City = ip.City
		ipInfo.ISP = ip.ISP
	}
	//fmt.Println(this.client.IpInfo.Province)

	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}

func UniqueId() string {
	b := make([]byte, 48)

	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}

	h := md5.New()
	h.Write([]byte(base64.URLEncoding.EncodeToString(b)))
	return hex.EncodeToString(h.Sum(nil))[:16]

}
