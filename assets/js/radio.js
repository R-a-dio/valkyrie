var timeOffset = 0;
// the minimal time between update functions in ms
var minUpdate = 1000;
// timer for the <time> updater
var timeUpdateTimer = timeUpdateTimer || 0;
// timer for the <progress> updater
var progressUpdateTimer = progressUpdateTimer || 0;
// Stream instance for the stream audio player
var stream = stream || undefined;
// Audio element for the admin player
var admin_player = admin_player || new Audio();

function now() {
    return Date.now() - timeOffset
}

function debugEventSource(url) {
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
    });
    es.addEventListener("thread", (event) => {
        console.log(event.data);
    });
}

function displayError(message, identifier) {
    let container = document.getElementById("error-container");
    // first check if we have a notification marked by the identifier
    if (identifier) {
        let existing = container.querySelector(`#${identifier}`);
        if (existing) {
            let msgbox = existing.querySelector(".error-message");

            if (!msgbox.dataset.errorCount) {
                msgbox.dataset.errorCount = 1;
            }
            msgbox.dataset.errorCount++;
            message += `\r\n[${msgbox.dataset.errorCount} times so far]`;

            msgbox.textContent = message;
            return
        }
    }

    // otherwise make a new one
    let tmpl = document.getElementById("error-template");
    let n = tmpl.content.cloneNode(true);

    if (identifier) {
        n.firstElementChild.id = identifier;
    }

    let msgbox = n.querySelector('.error-message')
    msgbox.textContent = message;
    // and add support for newlines
    msgbox.style.whiteSpace = "pre-line";

    htmx.process(n);
    container.appendChild(n);
}

function clearError(identifier) {
    let container = document.getElementById("error-container");

    let el = container.querySelector(`#${identifier}`);
    if (!el) {
        // nothing to do if it exists
        return
    }

    el.parentElement.removeChild(el);
}

function addEventListener(name, event, node, fn) {
    let key = `radioListener${name}`;
    if (!node.dataset[key]) {
        node.addEventListener(event, fn);
        node.dataset[key] = true;
    }
}

htmx.createEventSource = function (url) {
    current = new URL(window.location.href);
    theme = current.searchParams.get("theme");
    if (!theme) {
        es = new EventSource(url);
        return es
    }

    // construct a new url with theme parameter
    uri = new URL(url, window.location.origin);
    params = new URLSearchParams(uri.search);
    params.append("theme", theme);
    uri.search = params.toString();

    es = new EventSource(uri);
    return es;
}

htmx.on('htmx:responseError', (event) => {
    displayError(`[Error] an error has occurred`);
})

htmx.on('htmx:sendError', (event) => {
    displayError(`[Error: ${event.detail.xhr.status}] server is unreachable`);
})

htmx.on('htmx:sseError', (event) => {
    // don't show an error if the close was on purpose
    if (event.detail.source.readyState === event.detail.source.CLOSED) return;
    displayError("[SSE Error]\r\nyou'll not receive live page updates while this box is showing\r\nserver might be down or your internet, it will retry every 5 seconds.", "error-sse");
})

htmx.on('htmx:sseOpen', (event) => {
    clearError("error-sse");
})

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
        addEventListener("Progress", "htmx:xhr:progress", submit, (event) => {
            htmx.find('#submit-progress').setAttribute('value', event.detail.loaded / event.detail.total * 100);
        });
        // submission page daypass handling, move it into the header instead of the form
        addEventListener("BeforeRequest", "htmx:beforeRequest", submit, (event) => {
            let daypass = document.querySelector("input[name='daypass']").value
            if (daypass != "") {
                event.detail.xhr.setRequestHeader("X-Daypass", daypass);
            }
        });
    }

    // create a stream instance if one doesn't exist yet
    let initStream = document.getElementById("stream-audio")
    if (initStream && !stream) {
        stream = new Stream(initStream.getElementsByTagName("source")[0].src);
    }
    // and register event handlers on it
    if (stream && stream.button() && !stream.button().dataset.hasclick) {
        stream.button().onclick = async (event) => { await stream.playStop(event) };
        stream.button().dataset.hasclick = true;
        if (stream.audio && !stream.audio.paused) {
            stream.setButton("Stop Stream");
        }
    }
    // register volume slider
    let volume = document.getElementById("stream-volume");
    if (volume) {
        volume.value = localStorage.getItem("volume");
        addEventListener("Volume", "input", volume, (ev) => {
            vol = parseFloat(ev.target.value) / 100.0;
            if (stream) {
                stream.setVolume(vol, true);
            }
        });
    }

    if (!admin_player.dataset.haslistener) {
        admin_player = new Audio();
        admin_player.crossOrigin = 'anonymous';
        admin_player.preload = "none";
        admin_player.dataset.haslistener = true;
        admin_player.volume = localStorage.getItem("admin-player-volume") === null ? 0.1 : localStorage.getItem("admin-player-volume") / 100;
        admin_player.onended = () => { document.querySelectorAll(".adminPlayerPlayPauseButton").forEach((elt) => { elt.textContent = "Play"; elt.dataset.state = "Play"; }) };
        let update = (ev) => {
            if (!document.querySelector("#admin-player-time"))
                return;
            curMin = Math.floor(admin_player.currentTime / 60).toString();
            curSec = Math.floor(admin_player.currentTime % 60).toString();
            durMin = Math.floor(admin_player.duration / 60).toString();
            durSec = Math.floor(admin_player.duration % 60).toString();
            document.querySelector("#admin-player-time").innerText =
                curMin + ":" + curSec.padStart(2, '0') + " / " +
                durMin + ":" + durSec.padStart(2, '0');
            document.querySelector("#admin-player-progress").value =
                (admin_player.currentTime / admin_player.duration) * 100.0;
        };
        admin_player.addEventListener("timeupdate", update);
        admin_player.addEventListener("durationchange", update);
    }

    let admin_volume = document.querySelector("#admin-player-volume");
    if (admin_volume && !admin_volume.dataset.haslistener) {
        admin_volume.dataset.haslistener = true;
        admin_volume.value = 10.0;
        admin_volume.addEventListener("input", (ev) => {
            vol = parseFloat(ev.target.value) / 100.0;
            if (admin_player)
                admin_player.volume = vol;
        });
    } else {
        // If we lost track of the volume slider, we should reset the player.
        admin_player.pause();
        admin_player.src = "";
        admin_player.removeAttribute("src");
        admin_player.load();
    }

    let progress = document.querySelector("#admin-player-progress");
    if (progress && !progress.dataset.haslistener) {
        progress.dataset.haslistener = true;
        progress.addEventListener("input", (ev) => {
            prog = parseFloat(ev.target.value) / 100.0;
            if (admin_player && admin_player.duration)
                admin_player.currentTime = admin_player.duration * prog;
        });
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

// this error means the target element doesn't exist in the page
htmx.on('htmx:targetError', (event) => {
    // if we're broken, reload the current page
    let target = document.location.href
    // or if we somehow can figure out where we intended to go (what triggered this error)
    // use that instead
    if (event.srcElement.href) {
        target = event.srcElement.href;
    }

    htmx.ajax('GET', target, {target: 'body', swap: 'outerHTML', headers: {'HX-Request': 'false'}});
});

htmx.on('htmx:configRequest', (event) => {
    // if we overwrote the theme with a GET param we need to carry this thing over to other
    // htmx requests, otherwise stuff will not function or look weird

    // check if the theme parameter already exists in what htmx gave us
    if (event.detail.parameters.has("theme")) {
        return
    }

    // htmx doesn't include path query arguments in the parameters object so we need to
    // check that ourselves
    target = new URL(event.detail.path, window.location.origin);
    if (target.searchParams.has("theme")) {
        return
    }

    // now check if we originally had a theme parameter on our current path
    uri = new URL(window.location.href);
    if (!uri.searchParams.has("theme")) {
        // if not we don't want to add anything
        return
    }

    event.detail.parameters["theme"] = uri.searchParams.get("theme");
})

function prettyDuration(d) {
    if (d < 0) {
        if (d > -60) {
            return "<1 minute ago"
        }
        return rtf.format(Math.floor(d / 60), "minute")
    }
    if (d < 60) {
        return "in <1 minute"
    }
    return rtf.format(Math.floor(d / 60), "minute");
}

function mediumDuration(seconds) {
    if (seconds < 0) seconds = -seconds;
    
    const units = [
        { name: 'year', seconds: 31536000 },
        { name: 'day', seconds: 86400 },
        { name: 'hour', seconds: 3600 },
        { name: 'minute', seconds: 60 }/*,
        { name: 'second', seconds: 1 }*/
    ];
    
    const parts = [];
    let remaining = seconds;
    
    for (const unit of units) {
        const value = Math.floor(remaining / unit.seconds);
        remaining %= unit.seconds;
        
        if (value > 0) {
            parts.push({
                value,
                unit: unit.name
            });
        }
    }
    
    if (parts.length === 0) {
        return '0 minutes';
    }
    
    return parts
        .map(({ value, unit }, index) => {
            const isLast = index === parts.length - 1;
            const plural = value !== 1 ? 's' : '';
            return `${value} ${unit}${plural}${isLast ? '' : ','}`;
        })
        .join(' ');
}

function smallDuration(s) {
    if (s < 0) s = -s;
    const time = {
      day: Math.floor(s / 86400),
      h: Math.floor(s / 3600) % 24,
      m: Math.floor(s / 60) % 60,
      s: Math.floor(s % 60)
    };

    return Object.entries(time)
      .filter(val => val[1] !== 0)
      .map(([key, val]) => {
        if (key.length > 1) {
            return `${val} ${key}${val !== 1 ? 's' : ''}`
        }
        return `${val}${key}`
      })
      .join(' ');
}

var rtf = new Intl.RelativeTimeFormat("en", {
    localeMatcher: "best fit", // other values: "lookup"
    numeric: "always", // other values: "auto"
    style: "long", // other values: "short" or "narrow"
});

var dtf = new Intl.DateTimeFormat("default", {
    timeStyle: "long",
    hour12: false,
})

var dtfLong = new Intl.DateTimeFormat("default", {
    timeStyle: "long",
    dateStyle: "short",
})

var dtfSchedule = new Intl.DateTimeFormat("default", {
    weekday: "short",
    day: "numeric",
    month: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "numeric",
    hour12: false,
})

var dtfScheduleDur = new Intl.DateTimeFormat("default", {
    hour: "numeric",
    minute: "numeric",
    hour12: false,
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

function localTime(node) {
    var ltimeDur = node.getAttribute("data-dur")
    var ltime = new Date(node.getAttribute("datetime")*1000)
    var ltimeFuture = new Date(node.getAttribute("datetime")*1000)
    ltimeFuture.setMilliseconds(ltimeFuture.getMilliseconds() + ltimeDur)

    var out;
    if (ltimeDur == 0) {
        out = dtfSchedule.format(ltime)
    } else {
        if (ltimeFuture.getDay() > ltime.getDay()) {
            out = dtfSchedule.format(ltime) + " - " + dtfSchedule.format(ltimeFuture)
        } else {
            out = dtfSchedule.format(ltime) + " - " + dtfScheduleDur.format(ltimeFuture)
        }
    }

    return out
}

function prettyProgress(d) {
    d = Math.max(d, 0) / 1000;
    var mins = Math.floor(d / 60), secs = Math.floor(d % 60);
    return String(mins).padStart(2, "0") + ":" + String(secs).padStart(2, "0");
}

function adminPlayerPlayPause(src) {
    if (admin_player) {
        if (src && admin_player.src.includes(src)) {
            if (admin_player.paused) {
                admin_player.play();
            } else {
                admin_player.pause();
            }
        } else {
            admin_player.pause();
            admin_player.currentTime = 0;
            admin_player.src = src;
            admin_player.play();
        }
    }
}

function adminShowSpectrogram(src) {
    let img = document.querySelector("#admin-player-spec-image");
    let modal = document.querySelector("#admin-player-spec-modal");

    if (src && src.includes("pending")) {
        img.src = src + "?spectrum=true";
        modal.classList.add("is-active");
    }
}

// updateTimes looks for all <time> elements and determines whether to
// apply timeago or localization logic
function updateTimes() {
    if (timeUpdateTimer) {
        clearTimeout(timeUpdateTimer);
        timeUpdateTimer = 0;
    }

    var n = now() / 1000;
    var nextUpdate = 60;

    document.querySelectorAll("time:not(.htmx-settling)").forEach((node) => {
        if (node.dataset.timeset) {
            return
        }
        var d = node.dateTime - n;
        switch (node.dataset.type) {
            case "absolute":
                node.textContent = absoluteTime(node.dateTime);
                node.dataset.timeset = true;
                break;
            case "local":
                node.textContent = localTime(node);
                node.dataset.timeset = true;
                break;
            case "medium":
                node.textContent = mediumDuration(d);
                nextUpdate = Math.min(nextUpdate, 1);
                break;
            case "small":
                node.textContent = smallDuration(d);
                nextUpdate = Math.min(nextUpdate, 1);
                break;
            case "pretty":
                node.textContent = prettyDuration(d);
                nextUpdate = Math.min(nextUpdate, Math.abs(d) % 60);
                break;
            default:
                break;
        }
    })

    // convert to ms
    nextUpdate *= 1000;
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
var Stream = class {
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
            navigator.mediaSession.setActionHandler("pause", async (event) => {
                await this.playStop();
            });
            navigator.mediaSession.setActionHandler("stop", async (event) => {
                await this.playStop();
            });
            navigator.mediaSession.setActionHandler("play", async (event) => {
                await this.playStop();
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
        audio.addEventListener('error', async () => {
            await this.recover(true);
        }, true);
        return audio;
    }

    playStop = async (event) => {
        if (!this.audio || this.audio.paused) {
            await this.play(true);
        } else {
            await this.stop(true);
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

    recover = async (fromErrorHandler) => {
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

        await this.stop();
        await this.play();
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

    monitor = async () => {
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
                await this.recover(false);
            } else {
                this.monitorLastTime = cur;
            }
        }
        this.monitorTimer = setTimeout(this.monitor, 3000);
    }
}
