package server

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gorilla/websocket"
	"github.com/jpappel/grog_barrel/pkg/grog"
	"github.com/jpappel/grog_barrel/pkg/util"
)

var ErrIncompatibleVersion error = errors.New("incompatible version")
var ErrInvalidName error = errors.New("invalid client name")

func parseClient(c *websocket.Conn, logger *slog.Logger) (grog.Client, error) {
	client := grog.Client{Addr: c.RemoteAddr().String()}
	_, message, err := c.ReadMessage()
	if err != nil {
		logger.Error("Error while reading announce message",
			slog.String("remote", client.Addr),
			slog.String("error", err.Error()),
		)
		return client, err
	}

	client.Version = util.SemVer{Major: message[0], Minor: message[1], Patch: message[2]}
	if !ServerVersion.Compatible(client.Version) {
		logger.Info("Incompatible client version",
			slog.String("remote", client.Addr),
			slog.String("serverVersion", ServerVersion.String()),
			slog.String("clientVersion", client.Version.String()),
		)
		closeMsg := websocket.FormatCloseMessage(websocket.CloseAbnormalClosure,
			fmt.Sprintf("Incompatible client version: %s <= %s",
				client.Version.String(),
				ServerVersion.String()),
		)
		c.WriteMessage(websocket.CloseAbnormalClosure, closeMsg)

		return client, ErrIncompatibleVersion
	}

	client.Name = string(message[3:])
	// NOTE: len of a string is byte length
	if len(client.Name) == 0 || len(client.Name) > 255 {
		return client, ErrInvalidName
	}

	return client, nil
}
