const version = {
    major: 0,
    minor: 5,
    patch: 0
}

/**
 * @typedef StatusMsg
 * @type {Object}
 * @property {Number} id
 * @property {Number} state
 * @property {Number} offset
 */

const URL = location.host + "/barrel/testroom"
const playerStates = {
    UNKNOWN: 0,
    PLAYING: 1,
    PAUSED: 2,
    LOADING: 3,
    lookup: function(num) {
        switch (num) {
            case 0:
                return "Unknown";
            case 1:
                return "Playing";
            case 2:
                return "Paused";
            case 3:
                return "Loading";
            default:
                return "Invalid";
        }
    }
}
const messageTypes = {
    EMPTY: 0,
    ANNOUNCE: 1,
    STATUS: 2,
    ERROR: 3,
    lookup: function(type) {
        switch (type) {
            case 0:
                return "Empty";
            case 1:
                return "Announce";
            case 2:
                return "Status";
            case 3:
                return "Error";
            default:
                return "Invalid";
        }
    }
}

let websocket
let flaggons = []
/** @type Map<Number, string> */
let activeClients = new Map();

const log = {
    /** @param {String} string */
    append: (s) => {
        document.getElementById("recvdMessages").value += s;
    },
    appendln: (s) => {
        log.append(s + "\n");
    },
    clear: () => {
        document.getElementById("recvdMessages").value = "";
    }
}

/** Convert a number into a big endian byte array
 * @param {Number} number - the number to convert into bytes
 * @param {Number} length - length of byte array
 * @returns Uint8Array
 */
function toBytes(number, length) {
    if (number > (1 << (8 * length)) || number < 0) {
        throw "Value not in bound";
    }

    let arr = new Uint8Array(length)
    for (let i = 0; i < length; i++) {
        arr[1 - i] = number % 256;
        number = Math.floor(number / 256)
    }
    return arr
}

/** Convert a byte pair into a number
 * @param {Number} highByte
 * @param {Number} lowByte
 * @returns Number
 */
function fromBytes(highByte, lowByte) {
    return highByte * 256 + lowByte
}

/** Connect to a grog barrel web socket
 * @param {string} url - url of a grog barrel server
 * @returns WebSocket
 */
function connect(url) {
    let socket = new WebSocket(`ws://${url}`);
    socket.binaryType = "arraybuffer";

    socket.addEventListener("open", () => {
        log.clear();
        log.appendln("Opened Connection " + new Date());
        log.appendln("-------");
        let name = Math.random().toString();
        sendAnnounce(socket, name)
    });
    socket.addEventListener("message", (e) => {
        log.appendln("recieved message " + new Date());


        const data = new Uint8Array(e.data);
        const msgType = data[0];
        const msg = data.subarray(1);

        switch (msgType) {
            case messageTypes.EMPTY:
                break;
            case messageTypes.ANNOUNCE:
                parseAnnounce(msg, activeClients);
                updateClients(activeClients);
                break;
            case messageTypes.STATUS:
                let statuses = parseStatus(msg);
                updateStatuses(statuses, activeClients);
                break;
            case messageTypes.ERROR:
                log.append("An Error Occured: ")

                let error = new TextDecoder().decode(msg);

                log.appendln(error);
                log.appendln("closing connection");
                break;
            default:
                console.error("Recieved unknown message type:", msgType);
        }
    });
    socket.addEventListener("close", () => {
        log.appendln("-------\nClosed Connection " + new Date());
    });

    return socket
}

/** Send an announce message to a grog barrel server
 * @param {WebSocket} socket
 * @param {string} name - the name to register the client as
 */
function sendAnnounce(socket, name) {
    // PERF: reuse encoder instance
    const encoder = new TextEncoder();

    const msg = new Uint8Array(3 + name.length);
    msg.set([version.major, version.minor, version.patch]);
    msg.set(encoder.encode(name), 3);

    socket.send(msg);
}

/** Handle recieving an announce message
 *  @param {Uint8Array} data - the recieved data
 *  @param {Map<Number, String>} clients - client map to update
 */
function parseAnnounce(data, clients) {
    // PERF: reuse decoder instance
    const decoder = new TextDecoder("utf-8");
    let numClients = data[0];
    clients.clear();

    console.debug(data);

    let pos = 1;
    for (let clientNum = 0; clientNum < numClients; clientNum++) {
        let id = data[pos];
        pos++;
        let strLen = data[pos];
        pos++;
        let name = decoder.decode(data.subarray(pos, pos + strLen))
        clients.set(id, name);
        pos += strLen;
    };
}

/** Send a state message to a grog barrel server
 * @param {WebSocket} socket
 * @param {Number} offset
 * @param {Number} state
 */
function sendStatus(socket, offset, state) {
    if (socket.readyState != socket.OPEN) {
        throw "Attempting to send on non-OPEN socket";
    }
    const msg = new Uint8Array(3);
    msg.set(toBytes(offset, 2));
    msg[2] = state;
    socket.send(msg);
}

/** Handle recieving a state message
 * @param {Uint8Array} data - the recieved data
 */
function parseStatus(data) {
    let numUsers = data.length / 4;
    let statuses = [];
    for (let i = 0; i < numUsers; i++) {
        /** @type StatusMsg */
        let msg = {
            id: data[4 * i + 3],
            state: data[4 * i + 2],
            offset: fromBytes(data[4 * i], data[4 * i + 1])
        };
        statuses.push(msg);
    }

    return statuses
}

/** Handle parsing error messages
 * @param {Uint8Array} data - the recieved data
 * @returns String
 */
function parseError(data) {
}

/** Construct message form input objects
 */
function buildStatus() {
    const playerOffset = document.getElementById("playerOffset");
    const playerState = document.getElementById("playerState");
    return {
        offset: (playerOffset) ? parseInt(playerOffset.value) : 0,
        state: (playerState) ? parseInt(playerState.value) : 0
    }
}

/**
 * @param {Map<Number, String>} clients
 */
function updateClients(clients) {
    const existingClients = document.getElementsByClassName("grogFlaggon");
    const existingIds = new Set();
    for (let client of existingClients) {
        let id = parseInt(client.id.substring("grogFlaggon".length));
        if (clients.has(id)) {
            existingIds.add(id);
        } else {
            client.remove();
        }
    }

    const flaggonList = document.getElementById("grogBarrel");
    clients.forEach((name, id) => {
        if (!existingIds.has(id)) {
            let newFlaggon = constructFlaggon(id, playerStates.UNKNOWN, 0, name);
            flaggonList.appendChild(newFlaggon)
        }
    });

}
/**
 * Update status boxes for connected clients
 *
 * @param {StatusMsg[]} statuses
 * @param {Map<Number, String>} clients - map of ids to names
 */
function updateStatuses(statuses, clients) {
    // update/add flaggons
    statuses.forEach((status) => {
        log.append("idx: " + status.id);
        log.append("\tstate: " + playerStates.lookup(status.state));
        log.appendln("\toffset: " + status.offset);

        let name = clients.get(status.id) || "unknown";
        updateFlaggon(status, name);
    });
}

/** Construct a flaggon element but do not add it to the DOM
 * @param {Number} id
 * @param {Number} initState
 * @param {Number} initOffset
 * @param {String} name
 * @returns HTMLLIElement
 */
function constructFlaggon(id, initState, initOffset, name) {
    const el = document.createElement("li");
    el.id = "grogFlaggon" + id;
    el.classList.add("grogFlaggon");
    el.innerText = `${name}#${id}[${playerStates.lookup(initState)}]: ${initOffset}`
    return el
}

/** Attempt to update a flaggon, constructing one if necessary
 * @param {StatusMsg} status
 * @param {String} name
 */
function updateFlaggon(status, name) {
    const flaggon = document.getElementById("grogFlaggon" + status.id);
    const list = document.getElementById("grogBarrel");

    if (flaggon == null) {
        list.appendChild(constructFlaggon(status.id, status.state, status.offset, name));
        return
    }

    flaggon.innerText = `${name}#${status.id}[${playerStates.lookup(status.state)}]: ${status.offset}`
}

document.addEventListener("DOMContentLoaded", () => {
    const connStatus = document.getElementById("connStatus");
    const connBttn = document.getElementById("connBttn");
    const disconnBttn = document.getElementById("disconnBttn");
    const sendBttn = document.getElementById("sendBttn");
    const pollBttn = document.getElementById("pollBttn");
    const stopPollBttn = document.getElementById("stopPollBttn");
    connBttn.addEventListener("click", () => {
        websocket = connect(URL);
        connStatus.innerText = "Connected"
        connBttn.setAttribute("disabled", "");
        disconnBttn.removeAttribute("disabled");
        sendBttn.removeAttribute("disabled");
        pollBttn.removeAttribute("disabled");
    })
    disconnBttn.addEventListener("click", () => {
        websocket.close(1000);
        connStatus.innerText = "Disconnected"
        disconnBttn.setAttribute("disabled", "");
        sendBttn.setAttribute("disabled", "");
        connBttn.removeAttribute("disabled");
        pollBttn.setAttribute("disabled", "");
        stopPollBttn.setAttribute("disabled", "");
    })

    sendBttn.addEventListener("click", () => {
        let msg = buildStatus()
        sendStatus(websocket, msg.offset, msg.state)
    });
    let pollId = 0;
    pollBttn.addEventListener("click", () => {
        pollBttn.setAttribute("disabled", "");
        stopPollBttn.removeAttribute("disabled", "");
        pollId = setInterval(() => {
            let msg = buildStatus();
            sendStatus(websocket, msg.offset, msg.state)
        }, 1000)
    });
    stopPollBttn.addEventListener("click", () => {
        stopPollBttn.setAttribute("disabled", "");
        pollBttn.removeAttribute("disabled", "");
        if (pollId) {
            clearInterval(pollId);
            pollId = 0;
        }
    });
})
