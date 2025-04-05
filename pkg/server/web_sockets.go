package server

import (
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jpappel/grog_barrel/pkg/grog"
)

type WsDriver struct {
	conn   *websocket.Conn
	logger *slog.Logger
}

func (d WsDriver) WriteError(msg string) error {
	buf := make([]byte, 0, 1+len(msg))
	buf = append(buf, byte(grog.ERROR_MSG))
	buf = append(buf, msg...)

	deadline := time.Now().Add(1 * time.Second)
	return d.conn.WriteControl(websocket.CloseMessage, buf, deadline)
}

func (d WsDriver) ParseClient() (grog.Client, error) {
	addr := d.conn.RemoteAddr().String()
	_, message, err := d.conn.ReadMessage()
	if err != nil {
		d.logger.Error("Error while reading announce message",
			slog.String("remote", addr),
			slog.String("error", err.Error()),
		)
		return grog.Client{}, err
	}

	client, err := parseClient(message, addr, d.logger)
	if err != nil {
		return client, err
	}

	return client, nil
}
