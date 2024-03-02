var timeOffset = 0;
// the minimal time between update functions in ms
var minUpdate = 1000;
// timer for the <time> updater
var timeUpdateTimer = 0;
// timer for the <progress> updater
var progressUpdateTimer = 0;
// Stream instance for the stream audio player
var stream = undefined;

function now() {
    return Date.now() - timeOffset
}

htmx.createEventSource = function (url) {
    console.log(url);
    es = new EventSource(url);
    es.addEventListener("streamer", (event) => {
        console.log(event.data);
    });
    es.addEventListener("listeners", (event) => {
        console.log(event.data, Date.now());
    });
    es.addEventListener("metadata", (event) => {
        console.log(event.data);
    });
    es.addEventListener("lastplayed", (event) => {
        console.log(event.data);
    });
    es.addEventListener("queue", (event) => {
        console.log(event.data);
    });
    es.addEventListener("time", (event) => {
        timeOffset = Date.now() - event.data;
        console.log("using time offset of:", timeOffset);
    })
    return es;
}
//htmx.logAll();
htmx.on('htmx:load', (event) => {
    // unhide any elements that want to be visible if there is javascript
    document.querySelectorAll(".is-hidden-without-js").forEach((elt) => {
        elt.classList.remove("is-hidden-without-js");
    });
    // hide any elements that want to be hidden if there is javascript
    document.querySelectorAll(".is-hidden-with-js").forEach((elt) => {
        elt.classList.add("is-hidden")
    });
    // update any progress or times
    updateTimes();
    updateProgress();
    // register page-specific events in here
    let submit = document.getElementById('submit');
    if (submit) {
        // submission page progress bar
        console.log("registering submission progress handler");
        submit.addEventListener('htmx:xhr:progress', (event) => {
            htmx.find('#submit-progress').setAttribute('value', event.detail.loaded / event.detail.total * 100);
        });
        // submission page daypass handling, move it into the header instead of the form
        console.log("registering submission daypass-header handler");
        submit.addEventListener('htmx:beforeRequest', (event) => {
            let daypass = document.querySelector("input[name='daypass']").value
            if (daypass != "") {
                event.detail.xhr.setRequestHeader("X-Daypass", daypass);
            }
        });
    }
    let initStream = document.getElementById("stream-audio")
    if (initStream && !stream) {
        console.log("creating stream player");
        stream = new Stream(initStream.getElementsByTagName("source")[0].src);
    }
    if (stream && stream.button() && !stream.button().dataset.hasclick) {
        console.log("registering stream play/stop button handler");
        stream.button().onclick = stream.playStop;
        stream.button().dataset.hasclick = true;
        if (stream.audio && !stream.audio.paused) {
            stream.setButton("Stop Stream");
        }
    }
});

htmx.on('htmx:afterSettle', (event) => {
    if (event.target.getAttribute("sse-swap") == "metadata") {
        if (stream) {
            let metadata = document.getElementById("metadata");
            if (metadata) {
                stream.updateMediaSessionMetadata(metadata.textContent);
            }
        }
    }
});

function prettyDuration(d) {
    return rtf.format(Math.floor(d / 60), "minute");
}

const rtf = new Intl.RelativeTimeFormat("en", {
    localeMatcher: "best fit", // other values: "lookup"
    numeric: "always", // other values: "auto"
    style: "long", // other values: "short" or "narrow"
});

const dtf = new Intl.DateTimeFormat("default", {
    timeStyle: "long",
    hour12: false,
})

const dtfLong = new Intl.DateTimeFormat("default", {
    timeStyle: "long",
    dateStyle: "short",
})

function absoluteTime(d) {
    let today = new Date();
    today.setHours(0, 0, 0, 0);

    let date = new Date(d * 1000);
    if (date < today) {
        return dtfLong.format(date);
    } else {
        return dtf.format(date);
    }
}

function prettyProgress(d) {
    d = d / 1000;
    var mins = Math.floor(d / 60), secs = Math.floor(d % 60);
    return String(mins).padStart(2, "0") + ":" + String(secs).padStart(2, "0");
}

// updateTimes looks for all <time> elements and applies timeago logic to it
function updateTimes() {
    if (timeUpdateTimer) {
        clearTimeout(timeUpdateTimer);
        timeUpdateTimer = 0;
    }

    var n = now() / 1000;
    var nextUpdate = 60;

    document.querySelectorAll("time").forEach((node) => {
        if (node.dataset.timeset) {
            return
        }
        var d = node.dateTime - n;
        switch (node.dataset.type) {
            case "absolute":
                node.textContent = absoluteTime(node.dateTime);
                node.dataset.timeset = true;
                break;
            default:
                node.textContent = prettyDuration(d);
                break;
        }
        nextUpdate = Math.min(nextUpdate, Math.abs(d) % 60);
    })

    // convert to ms
    nextUpdate *= 1000
    // don't go below minUpdate
    nextUpdate = Math.max(nextUpdate, minUpdate);
    timeUpdateTimer = setTimeout(updateTimes, nextUpdate);
}

// updateProgress updates the progress bar for the playing song
//
// Uses #progress-current for the duration text and #current-song-progress.value for
// the progress in seconds
function updateProgress() {
    if (progressUpdateTimer) {
        clearTimeout(progressUpdateTimer);
        progressUpdateTimer = 0;
    }

    // update the text underneath the progress bar
    var current = document.getElementById("progress-current");
    if (current != null) {
        currentProgress = now() - current.dataset.start;
        current.textContent = prettyProgress(currentProgress);
        // update the progress bar
        document.getElementById("current-song-progress").value = Math.floor(currentProgress / 1000);
    }
    progressUpdateTimer = setTimeout(updateProgress, minUpdate);
}

setTimeout(updateTimes, 1000);
setTimeout(updateProgress, 1000);

// Stream <audio> element handling
class Stream {
    constructor(url) {
        this.url = url;
        this.audio = null;

        // volume state
        this.volume = 0.8; // default to 80%
        try { // but try to load the prefered value from local storage
            let storedVol = localStorage.getItem('volume');
            if (storedVol) {
                this.volume = parseInt(storedVol) / 100.0;
            }
        } catch (err) { }

        // fade-in state
        this.fadeVolume = 0.0;
        this.fadeTimer = undefined;

        // recover state
        this.recoverLast = 0;
        this.recoverGrace = 3 * 1000; // 3 seconds between recover attempts

        try {
            // setup phone action handlers, these are shown in the notification area
            navigator.mediaSession.setActionHandler("pause", (event) => {
                this.playStop();
            });
            navigator.mediaSession.setActionHandler("stop", (event) => {
                this.playStop();
            });
            navigator.mediaSession.setActionHandler("play", (event) => {
                this.playStop();
            })
        } catch (err) { }
    }

    updateMediaSessionMetadata = (metadata) => {
        if (!this.audio || this.audio.paused) {
            return
        }
        try {
            navigator.mediaSession.metadata = new MediaMetadata({
                title: metadata,
                artwork: [
                    {
                        "src": "/assets/images/logo_image_small.png",
                        "type": "image/png",
                    },
                ],
            })
        } catch (err) { }
    }

    cacheAvoidURL = () => {
        let url = new URL(this.url);
        url.searchParams.set("_t", Date.now());
        return url.href;
    }

    createAudio = () => {
        let audio = new Audio();
        audio.crossOrigin = 'anonymous';
        audio.preload = "none";
        audio.addEventListener('error', () => {
            this.recover(true);
        }, true);
        return audio;
    }

    playStop = (event) => {
        if (!this.audio || this.audio.paused) {
            this.play(true);
        } else {
            this.stop(true);
        }
    }

    play = async (newAudio) => {
        if (newAudio) {
            this.audio = this.createAudio();
        }
        // change the url slightly to avoid a firefox cache bug
        this.audio.src = this.cacheAvoidURL();

        let pp = this.audio.play();
        this.setButton("Connecting...");
        if (!pp || !pp.then || !pp.catch) {
            this.checkStarted();
            return;
        }

        try {
            await pp;
            this.checkStarted();
        } catch (err) {
            this.setButton("Error Connecting");
            console.log(Date.now(), 'error connecting to stream: ' + err);
        }
    }

    stop = async (deleteAudio) => {
        this.audio.pause()
        if (deleteAudio) {
            // due to eager buffering by browsers they will keep the stream 'playing'
            // in the background if we don't do the below things.
            this.audio.src = "";
            this.audio.removeAttribute("src");
            this.audio.load();
            this.audio = null;
        }

        this.setButton("Start Stream");
        this.monitorLastTime = 0;

        try {
            navigator.mediaSession.playbackState = "paused";
        } catch (err) { }
    }

    checkStarted = () => {
        if (this.audio.paused) {
            this.setButton("Something went wrong, try again");
            return
        }

        this.fadeVolume = 0.0;
        this.fadeIn();

        this.gracePeriod = 3;
        this.monitor();

        try {
            navigator.mediaSession.playbackState = "playing";
            this.updateMediaSessionMetadata(document.getElementById("metadata").textContent);
        } catch (err) { }

        this.setButton("Stop Stream");
    }

    fadeIn = () => {
        clearTimeout(this.fadeTimer);

        let cur = this.audio.currentTime;
        if (cur != this.fadePos) {
            this.fadePos = cur;
            if (this.volume - this.fadeVolume < 0.01) {
                // we're at our target volume
                this.setVolume(this.volume, false);
                return
            }
            this.fadeVolume = Math.min(this.fadeVolume + 0.03, this.volume);
            this.setVolume(this.fadeVolume, false);
        }

        this.fadeTimer = setTimeout(this.fadeIn, 20);
    }

    setVolume = (newVol, storeVol) => {
        let calculatedVol = Math.pow(newVol, 2.0);
        if (this.audio) {
            this.audio.volume = calculatedVol;
        }

        if (storeVol) {
            this.volume = newVol;
            try {
                localStorage.setItem('volume', Math.floor(newVol * 100));
            } catch (err) { }
        }
    }

    recover = (fromErrorHandler) => {
        if (!this.audio) { // we got called while there isn't supposed to be a stream
            return
        }
        this.setButton("Reconnecting...");

        if (this.audio.error) {
            if (this.audio.error.message) {
                console.log(Date.now(), 'stream error: ' + this.audio.error.code, this.audio.error.message);
            } else {
                console.log(Date.now(), 'stream error: ' + this.audio.error.code);
            }
        }

        if (fromErrorHandler) {
            // if we're coming from the error handler a server or our network
            // probably went missing, start a monitor and see if we can periodically
            // reconnect
            this.monitor();
        }
        if (this.recoverLast + this.recoverGrace >= Date.now()) {
            // don't recover if we've recently been called
            return
        }
        this.recoverLast = Date.now();

        this.stop();
        this.play();
    }

    setButton = (text) => {
        let button = this.button();
        if (button) {
            button.textContent = text;
        }
    }

    button = () => {
        return document.getElementById("stream-play-pause");
    }

    monitor = () => {
        if (!this.audio) {
            return;
        }
        clearTimeout(this.monitorTimer);

        if (this.gracePeriod > 0) {
            // wait for grace period to end before trying to reconnect
            this.gracePeriod--;
        } else {
            let cur = this.audio.currentTime
            if (cur <= this.monitorLastTime) {
                console.log(Date.now(), "reconnecting", cur);
                this.monitorLastTime = 0;
                this.recover(false);
            } else {
                this.monitorLastTime = cur;
            }
        }
        this.monitorTimer = setTimeout(this.monitor, 3000);
    }
}