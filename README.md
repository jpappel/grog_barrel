# Grog Barrel

A synchronization tool for browser based video playback

> [!WARNING]
> This ship has yet to make its maiden voyage! Beware!

## Notes

* Message types
    * client -> server
        * updateStatus (polled)
        * identifySelf (on connect)
    * server -> client
        * statusUpdate (polled)
        * announceIdentities (on client connect or disconnect)

### Unix Socket based ideas

1. Server Opens Welcome Unix Socket
    * `/tmp/grogbarrel/join.sock`
2. Client connects to socket, sends clientAnnounce
3. Server reads clientAnnounce responds with path to new socket or error, then closes the connection
    * `/tmp/grogbarrel/clientX.sock`
4. Client connects to `clientX.sock` , or server times out and closes `clientX.sock`
5. Server sends initial serverAnnounce to `clientX.sock`
6. Client sends clientStatus, standard loop proceeds


## Protocol

* clientAnnounce
    * 0x00-0x02: major, minor, patch bytes
    * 0x03-0xXX: utf-8 encoded name (max length 255)
* serverAnnounce
    * 0x00: 0x01
    * 0x01: number of clients
    * client list
        * 0x00: client id
        * 0x01: name length
        * 0x2-0xXX: name
* clientStatus
    * 0x00-0x01: Big endian client time
    * 0x02: client state
* serverStatus
    * 0x00: 0x02
    * 0x01: number of statuses
    * status list
        * 0x00-0x01: Big endian client time
        * 0x02: client state
        * 0x03: client id

### Server to Client Message Types

* Empty: 0
* Announce: 1
* Status: 2
* Error: 3

### Client States

* Unknown: 0
* Playing: 1
* Paused: 2
* Loading: 3

## Build

```bash
make
```

## TODO

* [x] add client names/aliases to protocol
* [ ] create links to jump local client to remote
* [ ] improve remote client statuses
    * indicator light/icon
    * reasonable styling
* [ ] fix project layout
