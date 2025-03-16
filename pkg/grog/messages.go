package grog

import (
	"encoding/binary"
	"fmt"

	"github.com/jpappel/grog_barrel/pkg/util"
)

type PlayerState byte
type MessageType byte

const (
	UNKNOWN_STATUS PlayerState = iota
	PLAYING_STATUS
	PAUSED_STATUS
	LOADING_STATUS
)

const (
	EMPTY_MSG MessageType = iota
	ANNOUNCE_MSG
	STATUS_MSG
)

type ClientStatusMessage struct {
	Offset      uint16      // current timestamp in file
	PlayerState PlayerState // playerState
	Id          byte        // consistent id from client
}

type ClientAnnounceMessage struct {
	Version util.SemVer
	Name    string
}

type ServerStatusMessage struct {
	Statuses []ClientStatusMessage
}

type ServerAnnounceMessage struct {
	Connections byte
	Clients     []struct {
		Id   byte
		Name string
	}
}

func (m ClientStatusMessage) String() string {
	status := ""
	if m.PlayerState == UNKNOWN_STATUS {
		status = "UNKNOWN"
	} else if m.PlayerState == PLAYING_STATUS {
		status = "PLAYING"
	} else if m.PlayerState == PAUSED_STATUS {
		status = "PAUSED"
	} else if m.PlayerState == LOADING_STATUS {
		status = "LOADING"
	}
	return fmt.Sprintf("%s %d", status, m.Offset)
}

// Append the correct transport encoding of a client status message to a slice
func (m ClientStatusMessage) WriteBytes(p []byte) []byte {
	p = binary.BigEndian.AppendUint16(p, m.Offset)
	p = append(p, byte(m.PlayerState))
	p = append(p, m.Id)
	return p
}

func (m ClientAnnounceMessage) String() string {
	return fmt.Sprintf("%s (%s)", m.Name, m.Version.String())
}

func (m ServerStatusMessage) WriteBytes(p []byte) []byte {
	for _, status := range m.Statuses {
		p = status.WriteBytes(p)
	}
	return p
}

func (m ServerAnnounceMessage) WriteBytes(p []byte) []byte {
	p = append(p, m.Connections)
	for _, client := range m.Clients {
		p = append(p, client.Id)
		p = append(p, byte(len(client.Name)))
		p = append(p, client.Name...)
	}
	return p
}
