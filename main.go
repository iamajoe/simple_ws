package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
)

var addr = flag.String("addr", ":8080", "http service address")

func handleAuth(manager *wsManager, c *wsClient, msg map[string]interface{}) error {
	// check the data and validate
	_, ok := msg["t"]
	if !ok {
		return errors.New("token missing")
	}

	// TODO: validate the token
	c.id = "example of user id"
	c.authDone <- c.id

	return nil
}

func handleChatSendMessage(manager *wsManager, c *wsClient, msg map[string]interface{}) error {
	// check the data and validate
	m, ok := msg["message"]
	if !ok {
		return errors.New("message missing")
	}

	manager.sendMessageToUsers(wsMsg{
		Kind: "chat:receive_msg",
		Data: map[string]interface{}{
			"message": m.(string),
		},
	}, []string{c.id})

	return nil
}

func main() {
	flag.Parse()

	// setup a frontend to easily test
	http.Handle("/", http.FileServer(http.Dir("./static")))

	// setup websocket modules
	modules := wsModules{
		"auth": {
			"validate": handleAuth,
		},
		"chat": {
			"send_msg": handleChatSendMessage,
		},
	}

	manager := NewWsManager(modules)
	http.HandleFunc("/ws", manager.serveWS)

	log.Println("Listening on " + *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
		return
	}
}
