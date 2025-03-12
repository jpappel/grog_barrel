package grog

import (
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

type StatusMessage struct {
	Offset      uint16      // current timestamp in file
	PlayerState PlayerState // playerState
	Id          byte        // consistent id from client
}

type AnnounceMessage struct {
	Version util.SemVer
	Name    string
}

func (m StatusMessage) String() string {
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

func (m AnnounceMessage) String() string {
	return fmt.Sprintf("%s (%s)", m.Name, m.Version.String())
}
