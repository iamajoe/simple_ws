package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type (
	wsModuleAction func(*wsManager, *wsClient, map[string]interface{}) error
	wsModule       map[string]wsModuleAction
	wsModules      map[string]wsModule
)

type wsMsg struct {
	Kind string                 `json:"kind"`
	Data map[string]interface{} `json:"data"`
}

type wsClient struct {
	authDone chan string
	done     chan bool
	id       string
	conn     *websocket.Conn
}

func (c *wsClient) send(msg wsMsg) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	err = c.conn.WriteMessage(1, data)
	return err
}

func (c *wsClient) close() {
	if len(c.done) != cap(c.done) {
		c.done <- true
	}
}

func (c *wsClient) closeWithError(err error) {
	log.Printf("error: %v", err)
	c.close()
}

func (c *wsClient) readMessages(manager *wsManager, modules wsModules) {
	for {
		// already closed so...
		if len(c.done) == cap(c.done) {
			break
		}

		// parse connection messages incoming
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure,
			) {
				c.closeWithError(err)
			} else {
				c.close()
			}

			break
		}

		// parse the msg
		var data wsMsg
		err = json.Unmarshal(message, &data)
		if err != nil {
			c.closeWithError(errors.New("bad request"))
			break
		}

		kindArr := strings.Split(data.Kind, ":")
		if len(kindArr) != 2 {
			c.closeWithError(errors.New("bad request"))
			break
		}
		moduleName := kindArr[0]
		actionName := kindArr[1]

		if c.id == "" && moduleName != "auth" && actionName != "validate" {
			c.closeWithError(errors.New("not authorized"))
			break
		}

		// map the message to the right place
		if mod, ok := modules[moduleName]; ok {
			if action, ok := mod[actionName]; ok {
				err = action(manager, c, data.Data)
			}
		}

		if err != nil {
			c.closeWithError(err)
			break
		}
	}
}

func NewWsClient(conn *websocket.Conn) *wsClient {
	return &wsClient{
		authDone: make(chan string, 1),
		done:     make(chan bool, 1),
		conn:     conn,
	}
}

type wsManager struct {
	modules      wsModules
	clients      map[string]*wsClient
	clientsMutex sync.Mutex
}

func (m *wsManager) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// TODO: how to handle error
		log.Println(err)
		return
	}

	client := NewWsClient(conn)
	go client.readMessages(m, m.modules)

	m.clientsMutex.Lock()
	select {
	case id := <-client.authDone:
		m.clients[id] = client
		break
	case <-client.done:
		delete(m.clients, client.id)
		close(client.done)
		conn.Close()
		break
	}
	m.clientsMutex.Unlock()
}

func (m *wsManager) sendMessageToUsers(msg wsMsg, ids []string) error {
	wg := sync.WaitGroup{}
	wg.Add(len(ids))

	m.clientsMutex.Lock()

	for _, id := range ids {
		if client, ok := m.clients[id]; ok {
			go func() {
				// TODO: as an exercise, transform this so that client has a sendingChannel
				//       buffered by 1 and keeps waiting for data to be sent
				err := client.send(msg)
				if err != nil {
					// TODO: handle the error
					log.Println(err)
				}

				wg.Done()
			}()
		}
	}

	wg.Wait()
	m.clientsMutex.Unlock()

	return nil
}

func NewWsManager(modules wsModules) *wsManager {
	return &wsManager{
		modules: modules,
		clients: make(map[string]*wsClient),
	}
}
