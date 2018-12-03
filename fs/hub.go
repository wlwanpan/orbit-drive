package fs

import (
	"log"
	"net/http"
	"net/url"

	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/websocket"
	"github.com/orbit-drive/orbit-drive/common"
	"github.com/orbit-drive/orbit-drive/fs/sys"
	"github.com/orbit-drive/orbit-drive/fs/vtree"
	"github.com/orbit-drive/orbit-drive/pb"
)

// Hub represents a interface for the backend hub service.
type Hub struct {
	// HostAddr is the address of the backend hub service.
	HostAddr string

	// AuthToken is the user authentication token.
	AuthToken string

	// conn holds the websocket connection.
	conn *websocket.Conn

	// updates
	updates chan []byte
}

// NewHub creates and start a websocket connection to backend hub.
func NewHub(addr string, authToken string) (*Hub, error) {
	hub := &Hub{
		HostAddr:  addr,
		AuthToken: authToken,
	}
	if err := hub.Connect(); err != nil {
		return &Hub{}, err
	}
	return hub, nil
}

// Header generate the hub request header.
func (h *Hub) Header() http.Header {
	header := http.Header{}
	header.Set("user-token", h.AuthToken)
	return header
}

// URL generate the hub request url.
func (h *Hub) URL() url.URL {
	return url.URL{
		Scheme: "ws",
		Host:   h.HostAddr,
		Path:   "/device-sync",
	}
}

// Connect dial the backend hub and establish a websocket connection
// and stores the connection to the hub conn.
func (h *Hub) Connect() error {
	url := h.URL()
	conn, _, err := websocket.DefaultDialer.Dial(url.String(), h.Header())
	if err != nil {
		return err
	}
	h.conn = conn
	return nil
}

// Sync listens to incoming traffics from the backend hub and
// call the appropriate handler to mutate the vtree.
func (h *Hub) Sync(vt *vtree.VTree) {
	for {
		_, msg, err := h.conn.ReadMessage()
		if err != nil {
			sys.Alert(err.Error())
		}
		log.Printf("Sync read message: %s", common.ToStr(msg))
		h.updates <- msg
	}
}

// Updates returns a parsed channel, parsing ws bytes to proto hub message.
func (h *Hub) Updates() (<-chan pb.Payload, <-chan error) {
	updates := make(chan pb.Payload)
	errs := make(chan error)
	go func() {
		update := <-h.updates
		hubMsg := &pb.Payload{}
		err := proto.Unmarshal(update, hubMsg)
		if err != nil {
			errs <- err
		}
		updates <- *hubMsg
	}()
	return updates, errs
}

// Stop closes the hub websocket connection to the backend hub.
func (h *Hub) Stop() {
	defer h.conn.Close()
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	err := h.conn.WriteMessage(websocket.CloseMessage, closeMsg)
	if err != nil {
		sys.Alert(err.Error())
	}
}

// Push send a msg to websocket connection
func (h *Hub) Push(msg []byte) error {
	return h.conn.WriteMessage(websocket.TextMessage, msg)
}
