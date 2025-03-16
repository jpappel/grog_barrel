package server

import (
	"encoding/binary"
	"errors"
	"log/slog"

	"github.com/jpappel/grog_barrel/pkg/grog"
	"github.com/jpappel/grog_barrel/pkg/util"
)

type Driver interface {
	WriteError(string) error
	ParseClient() (grog.Client, error)
}

var ErrIncompatibleVersion error = errors.New("incompatible version")
var ErrInvalidClientName error = errors.New("invalid client name")
var ErrInvalidRoomName error = errors.New("invalid room name")

func parseClient(message []byte, addr string, logger *slog.Logger) (grog.Client, error) {
	client := grog.Client{Addr: addr}

	client.Version = util.SemVer{Major: message[0], Minor: message[1], Patch: message[2]}
	if !ServerVersion.Compatible(client.Version) {
		logger.Info("Incompatible client version",
			slog.String("remote", client.Addr),
			slog.String("serverVersion", ServerVersion.String()),
			slog.String("clientVersion", client.Version.String()),
		)

		return client, ErrIncompatibleVersion
	}

	client.Name = string(message[3:])
	// NOTE: len of a string is byte length
	if len(client.Name) == 0 || len(client.Name) > 255 {
		return client, ErrInvalidClientName
	}

	return client, nil
}

func parseStatusMessage(p []byte, id byte) grog.ClientStatusMessage {
	return grog.ClientStatusMessage{
		Offset:      binary.BigEndian.Uint16(p[:2]),
		PlayerState: grog.PlayerState(p[2]),
		Id:          id,
	}
}
