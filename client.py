#!/usr/bin/env python3

import argparse
from enum import Enum
import os
import socket
from pathlib import Path
import sys
from time import sleep
import datetime as dt
from typing import Iterable, TypedDict

VERSION = {"Major": 0, "Minor": 5, "Patch": 0}
VERSION_STRING = "v" + ".".join(
    map(str, [VERSION["Major"], VERSION["Minor"], VERSION["Patch"]])
)


class PlayerState(Enum):
    UNKNOWN = 0
    PLAYING = 1
    PAUSED = 2
    LOADING = 3


class MessageType(Enum):
    EMPTY = 0
    ANNOUNCE = 1
    STATUS = 2
    ERROR = 3


class Client(TypedDict):
    name: str
    offset: int
    state: PlayerState


class State(TypedDict):
    name: str
    room: str
    socket: Path
    connected: bool
    current_time: dt.datetime
    last_info: str
    error: str
    clients: dict[int, Client]


def negotiate(client_name: str, room_name: str, join_sock_path: Path) -> Path:
    """Negotiate with a grogbarrel server for a client socket"""
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
        s.connect(join_sock_path.resolve().as_posix())
        s.sendall(
            VERSION["Major"].to_bytes()
            + VERSION["Minor"].to_bytes()
            + VERSION["Patch"].to_bytes()
            + client_name.encode()
        )
        sleep(0.25)
        s.sendall(room_name.encode())
        sleep(0.1)

        resp = s.recv(1024)
        if resp[0] == MessageType.ERROR:
            err_msg = resp[1:].decode()
            print(
                "Error occured while negotiating connection:", err_msg, file=sys.stderr
            )
            exit(1)
        else:
            client_socket_path = Path(resp.decode())
            print("Client succesfully negotiated")
            return client_socket_path


def status_to_bytes(offset: int, state: PlayerState) -> bytes:
    """Construct a clientStatus message"""
    return offset.to_bytes(2) + state.value.to_bytes()


def connect(
    client_path: Path,
    status_generator: Iterable[tuple[int, PlayerState]],
    name: str,
    room: str,
    sleep_duration: float = 1.5,
) -> Iterable[State]:
    # NOTE: this buffer size **HAS** to change when serverAnnounce or serverStatus schema are changed
    #       currently it is the maximum size of a serverAnnounce
    buf = bytearray(65536)
    state: State = {
        "name": name,
        "room": room,
        "socket": client_path,
        "connected": False,
        "current_time": dt.datetime.now(),
        "last_info": "",
        "clients": {},
        "error": "",
    }
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
        s.connect(client_path.resolve().as_posix())

        state["connected"] = True
        state["last_info"] = "Succesfully connected to client socket"

        for status in status_generator:
            recieved = s.recv_into(buf)
            state["current_time"] = dt.datetime.now()
            try:
                messageType = MessageType(buf[0])
            except ValueError:
                messageType = MessageType.EMPTY

            match messageType:
                case MessageType.ANNOUNCE:
                    state["last_info"] = "recieved serverAnnounce"
                    state["clients"].clear()

                    num_clients = buf[1]
                    pos = 2
                    for _ in range(num_clients):
                        client_id = buf[pos]
                        pos += 1
                        name_length = buf[pos]
                        pos += 1
                        name = buf[pos : pos + name_length].decode()
                        state["clients"][client_id] = {
                            "name": name,
                            "offset": -1,
                            "state": PlayerState.UNKNOWN,
                        }

                case MessageType.STATUS:
                    state["last_info"] = "recieved serverStatus"

                    offset = 1
                    n = recieved - offset
                    for i in range(n // 4):
                        start = 4 * i + offset
                        client_time = int.from_bytes(buf[start : start + 2])
                        try:
                            client_state = PlayerState(buf[start + 2])
                        except ValueError:
                            client_state = PlayerState.UNKNOWN
                        client_id = buf[start + 3]
                        # FIXME: random clients sometimes get added
                        state["clients"].setdefault(
                            client_id,
                            {
                                "name": f"Client {client_id}",
                                "offset": -1,
                                "state": PlayerState.UNKNOWN,
                            },
                        )
                        state["clients"][client_id]["offset"] = client_time
                        state["clients"][client_id]["state"] = client_state

                case MessageType.ERROR:
                    state["last_info"] = "recieved error"
                    state["error"] = buf[1:recieved].decode()
            yield state
            sleep(sleep_duration)
            state["last_info"] = "sending clientStatus"
            s.sendall(status_to_bytes(*status))


def get_info():
    client_name = input("Enter a name: ")
    if 1 < len(client_name) < 255:
        raise ValueError()
    room_name = input("Enter a room name: ")

    base_dir = input("Enter the grog base directory: ")
    base_path = Path(base_dir)
    if not base_path.is_dir():
        raise FileNotFoundError()

    return client_name, room_name, base_path


def render(state: State, rows: int, cols: int) -> None:
    # TODO: implement text wrapping
    # TODO: colorize text
    def build_header():
        left = f"{state['name']} [{state['room']}]"
        right = ("Online " if state["connected"] else "Offline ") + state[
            "current_time"
        ].isoformat()
        remaining = cols - (len(left) + len(right))
        center = " " * remaining if remaining > 2 else "\n"
        return left + center + right

    def build_body():
        body = f"{len(state['clients'])} Clients\n"

        def build_client(client: Client) -> str:
            return client["name"] + "[" + client["state"].name + f"] {client['offset']}"

        clients = map(
            lambda pair: pair[1],
            sorted(state["clients"].items(), key=lambda pair: pair[0]),
        )

        body += "\n".join(map(build_client, clients))
        body += "\n" + state["error"]
        return body

    def build_footer():
        version = (
            f"| grogbarrel v{VERSION['Major']}.{VERSION['Minor']}.{VERSION['Patch']} |"
        )
        return version + " " + state["socket"].as_posix() + " : " + state["last_info"]

    header = build_header()
    body = build_body()
    footer = build_footer()

    height = header.count("\n") + body.count("\n") + 4

    top_padding = ""
    remaining = rows - height - 1
    bottom_padding = "\n" * remaining if remaining > 0 else ""
    # TODO: make screen clear smoother
    print("\033c", end="\n\n", flush=True)
    print(header, flush=False)
    print(top_padding, end="", flush=False)
    print(body, flush=False)
    print(bottom_padding, end="", flush=False)
    print(footer, flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        prog="grogbarrel Client",
        description="a python client for grogbarrel",
        epilog="Supports version " + VERSION_STRING,
    )
    parser.add_argument("-n", "--client", default="", help="client name")
    parser.add_argument("-r", "--room", default="", help="room name")
    parser.add_argument(
        "-b",
        "--base-dir",
        type=Path,
        default=Path("/tmp/grogbarrel"),
        dest="base",
        help="base directory for grogbarrel sockets",
    )
    args = parser.parse_args()

    client_name = args.client
    room_name = args.room
    base_path = args.base

    # client_name, room_name, base_path = get_info()
    join_path: Path = base_path / "join.sock"
    if not join_path.is_socket():
        print(join_path, "is not a socket", file=sys.stderr)
        exit(1)

    client_path = negotiate(client_name, room_name, join_path)

    for state in connect(
        client_path,
        ((i, PlayerState(i % 4)) for i in range(10000)),
        client_name,
        room_name,
        sleep_duration=1,
    ):
        cols, rows = os.get_terminal_size()
        render(state, rows, cols)
