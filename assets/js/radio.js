var timeOffset = 0;
// the minimal time between update functions in ms
var minUpdate = 1000;

function now() {
    return Date.now() - timeOffset
}

htmx.createEventSource = function (url) {
    console.log(url);
    es = new EventSource(url);
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

function prettyDuration(d) {
    if (d > 0) {
        if (d <= 60) {
            return "in less than a min";
        }
        if (d < 120) {
            return "in 1 min";
        }
        return "in " + Math.floor(d / 60) + " mins"
    }

    d = Math.abs(d)
    if (d <= 60) {
        return "less than a min ago";
    }
    if (d < 120) {
        return "1 min ago"
    }
    return Math.floor(d / 60) + " mins ago";
}

function prettyProgress(d) {
    d = d / 1000;
    var mins = Math.floor(d / 60), secs = Math.floor(d % 60);
    return String(mins).padStart(2, "0") + ":" + String(secs).padStart(2, "0");
}

function updateTimes() {
    var n = now() / 1000;
    var nextUpdate = 60;

    document.querySelectorAll("time").forEach((node) => {
        var d = node.dateTime - n;
        node.textContent = prettyDuration(d);
        nextUpdate = Math.min(nextUpdate, Math.abs(d) % 60);
    })

    // convert to ms
    nextUpdate *= 1000
    // don't go below minUpdate
    nextUpdate = Math.max(nextUpdate, minUpdate);
    setTimeout(updateTimes, nextUpdate);
}

function updateProgress(interval) {
    // update the text underneath the progress bar
    var current = document.getElementById("progress-current");
    currentProgress = now() - current.dataset.start;
    current.textContent = prettyProgress(currentProgress);
    // update the progress bar
    document.getElementById("current-song-progress").value = Math.floor(currentProgress / 1000);
    setTimeout(updateProgress, minUpdate, minUpdate);
}

setTimeout(updateTimes, 1000);
setTimeout(updateProgress, 1000, 1000);