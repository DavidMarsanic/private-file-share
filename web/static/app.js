const el = (id) => document.getElementById(id);

function showError(msg) {
  el("error").textContent = msg;
  el("error").classList.remove("hidden");
}
function clearError() {
  el("error").classList.add("hidden");
}

// ---- mode switching ------------------------------------------------

const panels = { nearby: el("panelNearby"), send: el("panelSend"), receive: el("panelReceive") };
const modeBtns = { nearby: el("modeNearby"), send: el("modeSend"), receive: el("modeReceive") };

function setMode(mode) {
  clearError();
  for (const m in panels) {
    panels[m].classList.toggle("hidden", m !== mode);
    modeBtns[m].classList.toggle("active", m === mode);
  }
}
modeBtns.nearby.addEventListener("click", () => setMode("nearby"));
modeBtns.send.addEventListener("click", () => setMode("send"));
modeBtns.receive.addEventListener("click", () => setMode("receive"));

// ---- helpers ------------------------------------------------------

function postJSON(url, body) {
  return fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  }).then((res) =>
    res.json().catch(() => ({})).then((data) => {
      if (!res.ok) throw new Error(data.error || "request failed");
      return data;
    })
  );
}

function renderSelection(el_, paths) {
  if (!paths || paths.length === 0) {
    el_.classList.add("hidden");
    return;
  }
  el_.innerHTML = paths.length === 1
    ? paths[0].split("/").pop()
    : `${paths.length} items selected`;
  el_.classList.remove("hidden");
}

// ---- share nearby ------------------------------------------------

let nearbyPaths = [];

el("chooseFilesNearby").addEventListener("click", async () => {
  clearError();
  try {
    const data = await postJSON("/api/dialog/files");
    if (data.paths && data.paths.length) {
      nearbyPaths = data.paths;
      renderSelection(el("nearbySelection"), nearbyPaths);
      el("startNearby").classList.remove("hidden");
    }
  } catch (err) {
    showError(String(err.message || err));
  }
});
el("chooseFolderNearby").addEventListener("click", async () => {
  clearError();
  try {
    const data = await postJSON("/api/dialog/folder");
    if (data.path) {
      nearbyPaths = [data.path];
      renderSelection(el("nearbySelection"), nearbyPaths);
      el("startNearby").classList.remove("hidden");
    }
  } catch (err) {
    showError(String(err.message || err));
  }
});

el("startNearby").addEventListener("click", async () => {
  clearError();
  try {
    const data = await postJSON("/api/local-share/start", { paths: nearbyPaths });
    el("nearbyPicker").classList.add("hidden");
    el("nearbySelection").classList.add("hidden");
    el("startNearby").classList.add("hidden");
    el("nearbyActive").classList.remove("hidden");
    el("nearbyQr").src = "data:image/png;base64," + data.qrCodePng;
    el("nearbyUrl").href = data.url;
    el("nearbyUrl").textContent = data.url;
    pollNearbyStatus();
  } catch (err) {
    showError(String(err.message || err));
  }
});

let nearbyPollTimer = null;
function pollNearbyStatus() {
  clearInterval(nearbyPollTimer);
  nearbyPollTimer = setInterval(async () => {
    try {
      const res = await fetch("/api/local-share/status");
      const data = await res.json();
      if (!data.active) {
        clearInterval(nearbyPollTimer);
        return;
      }
      const n = data.downloads || 0;
      el("nearbyDownloads").textContent = n === 0 ? "No downloads yet" : `Downloaded ${n} time${n === 1 ? "" : "s"}`;
    } catch (e) {
      // transient network hiccup while polling — not worth surfacing
    }
  }, 2000);
}

el("stopNearby").addEventListener("click", async () => {
  clearInterval(nearbyPollTimer);
  await fetch("/api/local-share/stop", { method: "POST" }).catch(() => {});
  nearbyPaths = [];
  el("nearbyActive").classList.add("hidden");
  el("nearbyPicker").classList.remove("hidden");
});

// ---- send anywhere ------------------------------------------------

let sendPaths = [];
let sendJobId = null;
let sendEventSource = null;

el("chooseFilesSend").addEventListener("click", async () => {
  clearError();
  try {
    const data = await postJSON("/api/dialog/files");
    if (data.paths && data.paths.length) {
      sendPaths = data.paths;
      renderSelection(el("sendSelection"), sendPaths);
      el("startSend").classList.remove("hidden");
    }
  } catch (err) {
    showError(String(err.message || err));
  }
});
el("chooseFolderSend").addEventListener("click", async () => {
  clearError();
  try {
    const data = await postJSON("/api/dialog/folder");
    if (data.path) {
      sendPaths = [data.path];
      renderSelection(el("sendSelection"), sendPaths);
      el("startSend").classList.remove("hidden");
    }
  } catch (err) {
    showError(String(err.message || err));
  }
});

el("startSend").addEventListener("click", async () => {
  clearError();
  try {
    const data = await postJSON("/api/send", { paths: sendPaths });
    sendJobId = data.jobId;
    el("sendPicker").classList.add("hidden");
    el("sendSelection").classList.add("hidden");
    el("startSend").classList.add("hidden");
    el("sendCode").textContent = data.code;
    el("sendActive").classList.remove("hidden");
    el("sendProgress").classList.remove("hidden");
    subscribeTransfer(sendJobId, el("sendProgressFill"), el("sendProgressLabel"), () => {
      el("sendActive").classList.add("hidden");
      el("sendPicker").classList.remove("hidden");
      sendPaths = [];
    });
  } catch (err) {
    showError(String(err.message || err));
  }
});

el("cancelSend").addEventListener("click", () => {
  if (sendJobId) fetch(`/api/jobs/${sendJobId}/cancel`, { method: "POST" }).catch(() => {});
});

// ---- receive ------------------------------------------------------

let receiveJobId = null;
let receivedPath = "";

el("startReceive").addEventListener("click", async () => {
  clearError();
  const code = el("receiveCode").value.trim();
  if (!code) {
    showError("Enter the code you were given.");
    return;
  }
  try {
    const data = await postJSON("/api/receive", { code });
    receiveJobId = data.jobId;
    el("startReceive").disabled = true;
    el("receiveProgress").classList.remove("hidden");
    el("receiveDone").classList.add("hidden");
    subscribeTransfer(receiveJobId, el("receiveProgressFill"), el("receiveProgressLabel"), (finalEvent) => {
      el("startReceive").disabled = false;
      if (finalEvent && finalEvent.stage === "done") {
        receivedPath = finalEvent.path || "";
        el("receiveDone").classList.remove("hidden");
      }
    });
  } catch (err) {
    showError(String(err.message || err));
  }
});

el("cancelReceive").addEventListener("click", () => {
  if (receiveJobId) fetch(`/api/jobs/${receiveJobId}/cancel`, { method: "POST" }).catch(() => {});
});

el("showReceiveFolder").addEventListener("click", () => {
  if (receivedPath) postJSON("/api/reveal", { path: receivedPath }).catch(() => {});
});

// ---- shared transfer progress subscription ------------------------

function subscribeTransfer(jobId, fillEl, labelEl, onDone) {
  const es = new EventSource(`/api/jobs/${jobId}/events`);
  es.onmessage = (msg) => {
    const e = JSON.parse(msg.data);
    updateTransferProgress(e, fillEl, labelEl);
    if (e.stage === "done" || e.stage === "error" || e.stage === "canceled") {
      es.close();
      if (e.stage === "error") showError(e.message || "Transfer failed.");
      if (onDone) onDone(e);
    }
  };
  es.onerror = () => {
    es.close();
    showError("Lost connection to the local server.");
  };
}

function updateTransferProgress(e, fillEl, labelEl) {
  if (e.stage === "waiting") {
    fillEl.classList.add("indeterminate");
    labelEl.textContent = "Waiting for the other side…";
  } else if (e.stage === "transferring") {
    fillEl.classList.remove("indeterminate");
    fillEl.style.width = (e.percent || 0).toFixed(0) + "%";
    labelEl.textContent = `Transferring ${(e.percent || 0).toFixed(0)}%`;
  } else if (e.stage === "done") {
    fillEl.classList.remove("indeterminate");
    fillEl.style.width = "100%";
    labelEl.textContent = "Done.";
  } else if (e.stage === "canceled") {
    labelEl.textContent = "Canceled.";
  }
}
