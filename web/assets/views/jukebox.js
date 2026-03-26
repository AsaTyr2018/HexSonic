(function(ns) {
  const { state, $, escapeHtml, headers, apiFetch, fmt } = ns;
  const openTrackDetail = (...args) => ns.openTrackDetail(...args);
  const jukeboxActiveAudio = (...args) => ns.jukeboxActiveAudio(...args);
  const resumeAudioContextIfNeeded = (...args) => ns.resumeAudioContextIfNeeded(...args);
  const emitJukeboxState = (...args) => ns.emitJukeboxState(...args);
  const emitJukeboxTick = (...args) => ns.emitJukeboxTick(...args);
  const openJukeboxPopout = (...args) => ns.openJukeboxPopout(...args);
  function isJukeboxActive() {
    return !!(state.me && state.jukebox && state.jukebox.session_id);
  }

  function modeLabel(mode) {
    switch (String(mode || '').trim()) {
      case 'radio': return 'Radio';
      case 'try_me': return 'Try me!';
      default: return 'For You';
    }
  }

  function modeLane(snapshot) {
    if (snapshot.mode === 'radio') return 'Most played mix';
    if (snapshot.mode === 'try_me') return 'Low-play discovery';
    return 'Profile driven';
  }

  function syncJukeboxModeControls() {
    const mode = state.jukebox.mode || 'for_you';
    document.querySelectorAll('[data-jukebox-mode]').forEach((btn) => {
      btn.classList.toggle('active', btn.getAttribute('data-jukebox-mode') === mode);
    });
    const modeEl = $('jukeboxModeLabel');
    if (modeEl) modeEl.textContent = modeLabel(mode);
    const feedback = $('jukeboxFeedbackControls');
    if (feedback) feedback.classList.toggle('hidden', mode === 'radio');
  }

  function resetJukeboxSessionState() {
    state.jukebox = {
      ...state.jukebox,
      session_id: '',
      queue: [],
      seed_genre: '',
      seed_creator_sub: '',
      seed_album_id: 0,
      current_position: 0
    };
    state.jukeboxPlayer.queue = [];
    state.jukeboxPlayer.queueIndex = -1;
    $('jukeboxSessionMeta').textContent = 'No active session.';
    $('jukeboxQueueMeta').textContent = 'No queued tracks yet.';
    $('jukeboxLaneLabel').textContent = modeLane(state.jukebox);
    renderJukeboxNowPlaying();
    renderJukeboxQueue();
  }

  function syncJukeboxFromPlayerState() {
    if (!Array.isArray(state.jukeboxPlayer?.queue) || !state.jukeboxPlayer.queue.length) return;
    const currentID = String(state.jukeboxPlayer.nowPlayingTrackId || '').trim();
    if (!Array.isArray(state.jukebox.queue) || !state.jukebox.queue.length) {
      state.jukebox.queue = state.jukeboxPlayer.queue.slice();
      state.jukebox.current_position = Math.max(0, Number(state.jukeboxPlayer.queueIndex || 0));
      return;
    }
    if (!currentID) return;
    const idx = state.jukebox.queue.findIndex((item) => String(item?.id || '').trim() === currentID);
    if (idx >= 0) {
      state.jukebox.current_position = idx;
      return;
    }
    state.jukebox.queue = state.jukeboxPlayer.queue.slice();
    state.jukebox.current_position = Math.max(0, Number(state.jukeboxPlayer.queueIndex || 0));
  }

  function updateJukeboxControls() {
    const audio = jukeboxActiveAudio();
    if (!audio) return;
    $('jukeboxElapsedLabel').textContent = fmt(Number(audio.currentTime || 0));
    $('jukeboxDurationLabel').textContent = fmt(Number(audio.duration || 0));
    const d = Number(audio.duration || 0);
    const p = d > 0 ? Number(audio.currentTime || 0) / d : 0;
    const fill = $('jukeboxProgressFill');
    if (fill) fill.style.width = `${Math.max(0, Math.min(100, p * 100)).toFixed(2)}%`;
  }

  function applyJukeboxSnapshot(json) {
    state.jukebox = {
      ...state.jukebox,
      mode: json.mode || state.jukebox.mode || 'for_you',
      session_id: json.session_id || '',
      queue: Array.isArray(json.queue) ? json.queue : [],
      seed_genre: json.seed_genre || '',
      seed_creator_sub: json.seed_creator_sub || '',
      seed_album_id: Number(json.seed_album_id || 0)
    };
    const currentID = String(state.jukeboxPlayer?.nowPlayingTrackId || '').trim();
    state.jukeboxPlayer.queue = Array.isArray(state.jukebox.queue) ? state.jukebox.queue.slice() : [];
    if (currentID && state.jukeboxPlayer.queue.length) {
      const idx = state.jukeboxPlayer.queue.findIndex((item) => String(item?.id || '').trim() === currentID);
      state.jukeboxPlayer.queueIndex = idx >= 0 ? idx : 0;
    } else if (state.jukeboxPlayer.queue.length) {
      state.jukeboxPlayer.queueIndex = 0;
    } else {
      state.jukeboxPlayer.queueIndex = -1;
    }
    $('jukeboxSummary').textContent = json.summary || 'Adaptive radio for logged-in listeners. Public tracks only.';
    $('jukeboxLaneLabel').textContent = modeLane(state.jukebox);
    $('jukeboxSessionMeta').textContent = state.jukebox.session_id
      ? `Session ${state.jukebox.session_id.slice(0, 8)} · Mode: ${state.jukebox.mode.replaceAll('_', ' ')}`
      : 'No active session.';
    $('jukeboxQueueMeta').textContent = state.jukebox.queue.length
      ? `${state.jukebox.queue.length} queued public tracks. Rolling window: 6.`
      : 'No queued tracks yet.';
    syncJukeboxModeControls();
    renderJukeboxNowPlaying();
    renderJukeboxQueue();
  }

  function renderJukeboxNowPlaying() {
    syncJukeboxFromPlayerState();
    const box = $('jukeboxNowPlaying');
    if (!box) return;
    if (!state.me) {
      box.innerHTML = '<div class="muted">Login required.</div>';
      return;
    }
    const queue = Array.isArray(state.jukebox.queue) ? state.jukebox.queue : [];
    const current = queue[Number(state.jukebox.current_position || 0)] || null;
    if (!current) {
      box.innerHTML = '<div class="muted">No active track yet.</div>';
      updateJukeboxControls();
      return;
    }
    const audio = jukeboxActiveAudio();
    box.innerHTML = `
      <div class="jukebox-now-kicker">Live selection</div>
      <div class="jukebox-now-title">${escapeHtml(current.title || '-')}</div>
      <div class="jukebox-now-meta">
        <span>${escapeHtml(current.artist || '-')}</span>
        <span>·</span>
        <span>${escapeHtml(current.album || '-')}</span>
        <span>·</span>
        <span>${escapeHtml(current.genre || '-')}</span>
      </div>
      <div class="jukebox-now-progressmeta">
        <span class="muted">Reason</span>
        <button class="jukebox-reason-chip" type="button" data-jukebox-detail="${escapeHtml(current.id || '')}">${escapeHtml(current.reason || '-')}</button>
      </div>
    `;
    const detail = box.querySelector('[data-jukebox-detail]');
    if (detail) {
      detail.onclick = async (e) => {
        e.preventDefault();
        await openTrackDetail(detail.getAttribute('data-jukebox-detail'));
      };
    }
    updateJukeboxControls();
  }

  function renderJukeboxQueue() {
    syncJukeboxFromPlayerState();
    const body = $('jukeboxQueueBody');
    if (!body) return;
    const queue = Array.isArray(state.jukebox.queue) ? state.jukebox.queue : [];
    body.innerHTML = '';
    if (!state.me) {
      body.innerHTML = '<tr><td colspan="6" class="muted">Login required.</td></tr>';
      return;
    }
    if (!queue.length) {
      body.innerHTML = '<tr><td colspan="6" class="muted">No active session yet.</td></tr>';
      return;
    }
    const currentPosition = Number(state.jukebox.current_position || 0);
    const upcoming = queue.slice(currentPosition + 1);
    if (!upcoming.length) {
      body.innerHTML = '<tr><td colspan="6" class="jukebox-upnext-empty">Queue is rolling. More tracks will be added automatically.</td></tr>';
      return;
    }
    upcoming.forEach((track, idx) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td>${idx + 1}</td>
        <td>
          <div class="track-main-title">${escapeHtml(track.title || '-')}</div>
          <div class="uploader-inline">${escapeHtml(track.artist || '-')}</div>
        </td>
        <td>${escapeHtml(track.album || '-')}</td>
        <td>${escapeHtml(track.genre || '-')}</td>
        <td>${Number(track.score || 0).toFixed(1)}</td>
        <td><button class="jukebox-reason-chip" type="button" data-jukebox-detail="${escapeHtml(track.id || '')}">${escapeHtml(track.reason || '-')}</button></td>
      `;
      const detail = tr.querySelector('[data-jukebox-detail]');
      if (detail) {
        detail.onclick = async (e) => {
          e.preventDefault();
          await openTrackDetail(detail.getAttribute('data-jukebox-detail'));
        };
      }
      body.appendChild(tr);
    });
  }

  async function startJukeboxSession() {
    if (!state.me) return;
    resetJukeboxSessionState();
    const payload = {
      mode: state.jukebox.mode || 'for_you'
    };
    const res = await apiFetch('/api/v1/jukebox/start', {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      let message = `Jukebox start failed (${res.status}).`;
      try {
        const text = (await res.text()).trim();
        if (text) message = text;
      } catch (_) {}
      alert(message);
      return;
    }
    applyJukeboxSnapshot(await res.json());
    state.jukebox.current_position = 0;
    if (state.jukebox.queue.length) {
      await startJukeboxTrack(0);
    }
  }

  async function refreshJukeboxQueue(playedCount = Number(state.jukebox.current_position || 0)) {
    if (!state.me) return;
    if (!isJukeboxActive()) {
      await startJukeboxSession();
      return;
    }
    const trimmed = Math.max(0, Number(playedCount || 0));
    const res = await apiFetch('/api/v1/jukebox/next', {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: state.jukebox.session_id,
        current_position: trimmed
      })
    });
    if (!res.ok) {
      if (res.status === 400 || res.status === 404) {
        resetJukeboxSessionState();
        await startJukeboxSession();
        return;
      }
      let message = `Jukebox refresh failed (${res.status}).`;
      try {
        const text = (await res.text()).trim();
        if (text) message = text;
      } catch (_) {}
      alert(message);
      return;
    }
    applyJukeboxSnapshot(await res.json());
    if (state.jukebox.queue.length) {
      state.jukebox.current_position = 0;
      if (state.jukeboxPlayer.queue.length) {
        state.jukeboxPlayer.queueIndex = 0;
      }
    }
  }

  async function sendJukeboxFeedback(action) {
    if (!isJukeboxActive()) return;
    const current = state.jukebox.queue[state.jukebox.current_position] || null;
    if (!current) return;
    const res = await apiFetch('/api/v1/jukebox/feedback', {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: state.jukebox.session_id,
        track_id: current.id,
        action
      })
    });
    if (!res.ok) {
      alert(`Jukebox feedback failed (${res.status}).`);
      return;
    }
    applyJukeboxSnapshot(await res.json());
    if (action === 'skip' || action === 'less_like_this') {
      await handleJukeboxAdvance();
    }
  }

  async function startJukeboxTrack(index) {
    const queue = Array.isArray(state.jukebox.queue) ? state.jukebox.queue : [];
    const idx = Number(index || 0);
    const track = queue[idx];
    if (!track) return;
    state.jukebox.current_position = idx;
    await ns.startJukeboxTrackById(track.id, queue, `jukebox:${state.jukebox.mode}`);
    renderJukeboxNowPlaying();
    renderJukeboxQueue();
    if ((queue.length - idx) <= 3) {
      await refreshJukeboxQueue(idx);
    }
  }

  async function handleJukeboxAdvance() {
    if (!isJukeboxActive()) return false;
    let nextIndex = Number(state.jukebox.current_position || 0) + 1;
    if (nextIndex >= state.jukebox.queue.length) {
      await refreshJukeboxQueue(Number(state.jukebox.current_position || 0) + 1);
      nextIndex = 0;
    }
    if (nextIndex >= state.jukebox.queue.length) {
      return false;
    }
    await startJukeboxTrack(nextIndex);
    return true;
  }

  async function renderJukebox() {
    syncJukeboxFromPlayerState();
    syncJukeboxModeControls();
    $('jukeboxLaneLabel').textContent = modeLane(state.jukebox);
    renderJukeboxNowPlaying();
    renderJukeboxQueue();
  }

  Object.assign(ns, {
    syncJukeboxModeControls,
    syncJukeboxFromPlayerState,
    updateJukeboxControls,
    applyJukeboxSnapshot,
    renderJukeboxNowPlaying,
    renderJukeboxQueue,
    resetJukeboxSessionState,
    startJukeboxSession,
    refreshJukeboxQueue,
    sendJukeboxFeedback,
    startJukeboxTrack,
    handleJukeboxAdvance,
    renderJukebox
  });
})(window.HexSonic = window.HexSonic || {});
