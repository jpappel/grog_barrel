package grog

import (
	"github.com/gorilla/websocket"
)

type WebSocketDriver struct {
	Status        *websocket.PreparedMessage
	Announcements *websocket.PreparedMessage
}

func (d *WebSocketDriver) WriteStatus(msg []byte) error {
	var err error
	d.Status, err = websocket.NewPreparedMessage(2, msg)
	if err != nil {
		return err
	}

	return nil
}

func (d *WebSocketDriver) WriteAnnounce(msg []byte) error {
	var err error
	d.Announcements, err = websocket.NewPreparedMessage(2, msg)
	if err != nil {
		return err
	}

	return nil
}
