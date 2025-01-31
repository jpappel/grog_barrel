const playerState = {
    UNKNOWN: 0,
    PLAYING: 1,
    PAUSED: 2,
    LOADING: 3
}

/** Connect to a grog barrel web socket
 * @param {string} url - url of a grog barrel server
 * @returns {WebSocket}
 */
function connect(url) {
    let socket = new WebSocket(`ws://${url}`);
    socket.addEventListener("message", (e) => {
        console.log("recieved message: ", e.data);
    });

    return socket
}

/** Send a state message to a grog barrel server
 * @param {WebSocket} socket
 * @param {Number} offset
 */
function sendMsg(socket, offset, state) {
    const arr = new Uint8Array(3)
    // TODO: figure out how to convert a number to a uint16 in this god forsaken language
    // TODO: same thing but for playerState into a byte
    socket.send(blob)
}
