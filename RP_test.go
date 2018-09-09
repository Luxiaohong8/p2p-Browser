package main

import (
	"testing"
	"log"
	"time"
	"os"
	"os/signal"
	"github.com/gorilla/websocket"
	"fmt"
	"encoding/json"
	"net/url"
)

type AcceptMsg struct {
	Peer_id string `json:"peer_id"`
}

type ParentsMsg struct {
	Nodes []AcceptMsg    `json:"nodes"`
}

func createWs()  {

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	channel := "https://video-dev.github.io/streams/x36xhzz/x36xhzz.m3u8"
	_url := "ws://127.0.0.1:8080/ws?key=free&room=" + url.QueryEscape(channel)
	log.Printf("connecting to %s", _url)
	var (
		peerId string
		remotePeerId string
	)


	c, _, err := websocket.DefaultDialer.Dial(_url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer c.Close()
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
			action := struct {
				Action     string `json:"action"`
			}{}
			if err := json.Unmarshal(message, &action); err != nil {
				//logrus.Errorf("[Client.handle] json.Unmarshal %s", err.Error())
				log.Printf("json.Unmarshal %s", err.Error())
				return
			}
			switch action.Action {
			case "accept":
				//获取peer id
				msg := AcceptMsg{}
				json.Unmarshal(message, &msg)
				peerId = msg.Peer_id
				getParents := map[string]interface{}{
					"action": "get_parents",
				}
				error := c.WriteJSON(getParents)
				if error != nil {
					log.Println("write close:", err)
					return
				}
				break
			case "parents":
				msg := ParentsMsg{}
				json.Unmarshal(message, &msg)
				remotePeerId = msg.Nodes[0].Peer_id
				dcId := peerId + "-" + remotePeerId
				resp := map[string]interface{} {
					"action": "dc_opened",
					"dc_id": dcId,
				}
				error := c.WriteJSON(resp)
				if error != nil {
					log.Println("write close:", err)
					return
				}
				break
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	enter := map[string]interface{}{
		"action": "enter",
		"channel": "abc" ,
		"browser":   "chrome" ,
		"device": "PC",
		"os": "Mac OS",

	}
	error := c.WriteJSON(enter)
	if error != nil {
		log.Println("write close:", err)
		return
	}
	for {
		select {
		case  <-ticker.C:
			//err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
			//if err != nil {
			//	log.Println("write:", err)
			//	return
			//}
		case <-interrupt:
			log.Println("interrupt")
			// To cleanly close a connection, a client should send a close
			// frame and wait for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			c.Close()
			return
		}
	}

}



func TestLayer(t *testing.T)  {

	createWs()
	fmt.Println()
	//node, ok := hub.clients.Load(clients[2])
	//result, _ := hub.filterByLayer(&clients[2])
	//if len(result) != 10 {
	//	t.Fatalf("expect 10 eles, got %d", len(result))
	//}
}

//func TestISP(t *testing.T)  {
//	initHub()
//	source, _ := hub.filterByLayer(&clients[2])
//	result, err := hub.filterByISP(source, "电信")
//	if len(result) != 5 {
//		t.Fatalf("expect 5 eles, got %d err %s", len(result), err)
//	}
//}
