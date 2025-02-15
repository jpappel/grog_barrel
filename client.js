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

let websocket
let flaggons = []

/** Convert a number into a big endian byte array
 * @param {Number} number
 * @returns Uint8Array
 */
function toBytes(number) {
    if (number > 1 << 16 || number < 0) {
        // TODO: throw error
    }

    let arr = new Uint8Array(2)
    for (let i = 0; i < 2; i++) {
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
    console.log("high:", highByte, "low:", lowByte);
    return highByte * 256 + lowByte
}

/** Connect to a grog barrel web socket
 * @param {string} url - url of a grog barrel server
 * @returns WebSocket
 */
function connect(url) {
    let socket = new WebSocket(`ws://${url}`);
    socket.binaryType = "arraybuffer";
    let msgbox = document.getElementById("recvdMessages");

    socket.addEventListener("open", () => {
        // TODO: publish name to server
        msgbox.value = "Opened Connection " + new Date();
        msgbox.value += "\n-------"
    });
    socket.addEventListener("message", (e) => {
        msgbox.value += "\nrecieved message " + new Date();

        let recvd = new Uint8Array(e.data);
        let numConns = recvd.length / 4;

        for (let i = 0; i < numConns; i++) {
            let msg = {
                id: i,
                state: recvd[4 * i + 2],
                offset: fromBytes(recvd[4 * i], recvd[4 * i + 1])
            }
            msgbox.value += "\nidx: " + msg.id;
            msgbox.value += "\tstate: " + playerStates.lookup(msg.state);
            msgbox.value += "\toffset: " + msg.offset;

            updateFlaggon(msg.id, msg.state, msg.offset);
            // TODO: delete flaggons if connection drops
        }
    });
    socket.addEventListener("close", () => {
        // TODO: unpublish name from server
        msgbox.value += "\n-------\nClosed Connection " + new Date();
    });

    return socket
}

/** Send a state message to a grog barrel server
 * @param {WebSocket} socket
 * @param {Number} offset
 * @param {Number} state
 */
function sendMsg(socket, offset, state) {
    // TODO: check for ws state
    const msg = new Uint8Array(3);
    let arr = toBytes(offset);
    msg[0] = arr[0];
    msg[1] = arr[1];
    msg[2] = state;
    socket.send(msg);
}

/** Construct message form input objects
 */
function getMsg() {
    const playerOffset = document.getElementById("playerOffset");
    const playerState = document.getElementById("playerState");
    return {
        offset: (playerOffset) ? parseInt(playerOffset.value) : 0,
        state: (playerState) ? parseInt(playerState.value) : 0
    }
}

/** Construct a flaggon element but do not add it to the DOM
 * @param {Number} id
 * @param {Number} initState
 * @param {Number} initOffset
 * @returns HTMLLIElement
 */
function constructFlaggon(id, initState, initOffset) {
    const el = document.createElement("li");
    el.setAttribute("id", "grogFlaggon" + id);
    el.classList.add("grogFlaggon");
    el.innerText = `${id} [${playerStates.lookup(initState)}]: ${initOffset}`
    return el
}

function appendFlaggon(el) {
    const list = document.getElementById("grogBarrel");
    list.appendChild(el);
}

/** Attempt to update a flaggon, constructing one if necessary
 * @param {Number} id
 * @param {Number} state
 * @param {Number} offset
 */
function updateFlaggon(id, state, offset) {
    const flaggon = document.getElementById("grogFlaggon" + id);
    if (flaggon == null) {
        appendFlaggon(constructFlaggon(id, state, offset));
        return
    }

    flaggon.innerText = `${id} [${playerStates.lookup(state)}]: ${offset}`
}

document.addEventListener("DOMContentLoaded", () => {
    const connStatus = document.getElementById("connStatus");
    const connBttn = document.getElementById("connBttn");
    const disconnBttn = document.getElementById("disconnBttn");
    const sendBttn = document.getElementById("sendBttn");
    const pollBttn = document.getElementById("pollBttn");
    const stopPollBttn = document.getElementById("stopPollBttn");
    connBttn.addEventListener("click", () => {
        // TODO: check for ws state
        websocket = connect(URL);
        connStatus.innerText = "Connected"
        connBttn.setAttribute("disabled", "");
        disconnBttn.removeAttribute("disabled");
        sendBttn.removeAttribute("disabled");
        pollBttn.removeAttribute("disabled");
    })
    disconnBttn.addEventListener("click", () => {
        // TODO: check for ws state
        websocket.close();
        connStatus.innerText = "Disconnected"
        disconnBttn.setAttribute("disabled", "");
        sendBttn.setAttribute("disabled", "");
        connBttn.removeAttribute("disabled");
        pollBttn.setAttribute("disabled", "");
        stopPollBttn.setAttribute("disabled", "");
    })

    sendBttn.addEventListener("click", () => {
        let msg = getMsg()
        sendMsg(websocket, msg.offset, msg.state)
    });
    let pollId = 0;
    pollBttn.addEventListener("click", () => {
        pollBttn.setAttribute("disabled", "");
        stopPollBttn.removeAttribute("disabled", "");
        pollId = setInterval(() => {
            let msg = getMsg();
            sendMsg(websocket, msg.offset, msg.state)
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
