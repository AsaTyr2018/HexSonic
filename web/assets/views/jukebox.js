(function(ns) {
  const { state, $, escapeHtml, headers, apiFetch, fmt } = ns;
  const openTrackDetail = (...args) => ns.openTrackDetail(...args);
  function isJukeboxActive() {
    return !!(state.me && state.jukebox && state.jukebox.session_id);
  }

  function modeLabel(mode) {
    switch (String(mode || '').trim()) {
      case 'genre': return 'Genre Radio';
      case 'creator': return 'Creator Radio';
      case 'album': return 'Album Radio';
      case 'try_me': return 'Try me!';
      default: return 'For You';
    }
  }

  function modeLane(snapshot) {
    if (snapshot.seed_genre) return snapshot.seed_genre;
    if (snapshot.mode === 'creator') return 'Creator focus';
    if (snapshot.mode === 'album') return 'Album flow';
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
  }

  function syncJukeboxFromPlayerState() {
    const sourceContext = String(state.playerDeck?.currentSourceContext || '');
    if (!sourceContext.startsWith('jukebox')) return;
    if (!Array.isArray(state.queue) || !state.queue.length) return;
    state.jukebox.queue = state.queue.slice();
    if (Number.isFinite(Number(state.queueIndex)) && Number(state.queueIndex) >= 0) {
      state.jukebox.current_position = Number(state.queueIndex);
    }
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
    state.queue = state.jukebox.queue.slice();
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
      return;
    }
    const audio = typeof ns.activeAudio === 'function' ? ns.activeAudio() : $('audio');
    const isLive = String(state.currentView || '') === 'jukebox' && state.nowPlayingTrackId && String(state.nowPlayingTrackId) === String(current.id || '');
    const currentTime = isLive ? Number(audio?.currentTime || 0) : 0;
    const duration = isLive ? Number(audio?.duration || current.duration_seconds || 0) : Number(current.duration_seconds || 0);
    const progress = duration > 0 ? Math.max(0, Math.min(100, (currentTime / duration) * 100)) : 0;
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
      <div class="jukebox-now-progress">
        <div class="jukebox-now-progressbar"><div class="jukebox-now-progressfill" style="width:${progress.toFixed(2)}%"></div></div>
        <div class="jukebox-now-progressmeta">
          <span>${fmt(currentTime)}</span>
          <button class="jukebox-reason-chip" type="button" data-jukebox-detail="${escapeHtml(current.id || '')}">${escapeHtml(current.reason || '-')}</button>
          <span>${fmt(duration)}</span>
        </div>
      </div>
    `;
    const detail = box.querySelector('[data-jukebox-detail]');
    if (detail) {
      detail.onclick = async (e) => {
        e.preventDefault();
        await openTrackDetail(detail.getAttribute('data-jukebox-detail'));
      };
    }
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
    const payload = {
      mode: state.jukebox.mode || 'for_you'
    };
    const res = await apiFetch('/api/v1/jukebox/start', {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      alert(`Jukebox start failed (${res.status}).`);
      return;
    }
    applyJukeboxSnapshot(await res.json());
    state.jukebox.current_position = 0;
    if (state.jukebox.queue.length) {
      await startJukeboxTrack(0);
    }
  }

  async function refreshJukeboxQueue() {
    if (!isJukeboxActive()) return;
    const res = await apiFetch('/api/v1/jukebox/next', {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({
        session_id: state.jukebox.session_id,
        current_position: Number(state.jukebox.current_position || 0) + 1
      })
    });
    if (!res.ok) {
      alert(`Jukebox refresh failed (${res.status}).`);
      return;
    }
    applyJukeboxSnapshot(await res.json());
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
    state.queue = queue.slice();
    await ns.startTrackById(track.id, queue, `jukebox:${state.jukebox.mode}`);
    renderJukeboxNowPlaying();
    renderJukeboxQueue();
    if ((queue.length - idx) <= 3) {
      await refreshJukeboxQueue();
    }
  }

  async function handleJukeboxAdvance() {
    if (!isJukeboxActive()) return false;
    let nextIndex = Number(state.jukebox.current_position || 0) + 1;
    if (nextIndex >= state.jukebox.queue.length) {
      await refreshJukeboxQueue();
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
    if (state.me && (!Array.isArray(state.jukebox.queue) || !state.jukebox.queue.length)) {
      await startJukeboxSession();
    }
  }

  Object.assign(ns, {
    syncJukeboxModeControls,
    syncJukeboxFromPlayerState,
    applyJukeboxSnapshot,
    renderJukeboxNowPlaying,
    renderJukeboxQueue,
    startJukeboxSession,
    refreshJukeboxQueue,
    sendJukeboxFeedback,
    startJukeboxTrack,
    handleJukeboxAdvance,
    renderJukebox
  });
})(window.HexSonic = window.HexSonic || {});
