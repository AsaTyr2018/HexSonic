(function(ns) {
const { state, $, escapeHtml, ICON_PLAY, ICON_PAUSE, ICON_MUTE, ICON_UNMUTE, ICON_DETAIL, ICON_PLAYLIST, PLAYER_BRIDGE_CHANNEL, JUKEBOX_BRIDGE_CHANNEL, PLAYER_TICK_MS, headers, apiFetch, fmt, normText } = ns;
const LAST_PLAYER_SNAPSHOT_KEY = 'hex_last_player_snapshot_v1';
const LAST_JUKEBOX_SNAPSHOT_KEY = 'hex_last_jukebox_snapshot_v1';
const finalizeListeningSession = (...args) => ns.finalizeListeningSession(...args);
const beginListeningSession = (...args) => ns.beginListeningSession(...args);
const renderPlaylistTracks = (...args) => ns.renderPlaylistTracks(...args);
const renderPlaylistDock = (...args) => ns.renderPlaylistDock(...args);
const selectAlbum = (...args) => ns.selectAlbum(...args);
const sourceContextForPlayback = (...args) => ns.sourceContextForPlayback(...args);
const getSelectedAlbumTracks = (...args) => ns.getSelectedAlbumTracks(...args);

    function audioById(id) {
      return $(id);
    }

    function engineState(name = state.activeEngine || 'main') {
      return name === 'jukebox' ? state.jukeboxPlayer : state.mainPlayer;
    }

    function syncLegacyPlayerMirror(name = state.activeEngine || 'main') {
      const engine = engineState(name);
      state.activeEngine = name;
      state.queue = Array.isArray(engine.queue) ? engine.queue.slice() : [];
      state.queueIndex = Number(engine.queueIndex || -1);
      state.nowPlayingTrackId = String(engine.nowPlayingTrackId || '');
      state.currentLyrics = engine.currentLyrics || { track_id: '', plain: '', srt: '', cues: [] };
    }

    function mainAudio() {
      return $('audio');
    }

    function jukeboxActiveAudio() {
      return audioById(state.jukeboxPlayer.activeAudioId || 'audioJukeboxA') || $('audioJukeboxA');
    }

    function jukeboxStandbyAudio() {
      return audioById(state.jukeboxPlayer.standbyAudioId || 'audioJukeboxB') || $('audioJukeboxB');
    }

    function activeAudio() {
      if ((state.activeEngine || 'main') === 'jukebox') return jukeboxActiveAudio();
      return mainAudio();
    }

    function standbyAudio() {
      if ((state.activeEngine || 'main') === 'jukebox') return jukeboxStandbyAudio();
      return null;
    }

    function swapJukeboxDecks() {
      const currentActive = state.jukeboxPlayer.activeAudioId || 'audioJukeboxA';
      state.jukeboxPlayer.activeAudioId = state.jukeboxPlayer.standbyAudioId || 'audioJukeboxB';
      state.jukeboxPlayer.standbyAudioId = currentActive;
    }

    function syncJukeboxDeckVolumes() {
      const fx = state.audioFX;
      const active = jukeboxActiveAudio();
      const standby = jukeboxStandbyAudio();
      if (!active || !standby) return;
      if (fx && fx.gainA && fx.gainB) {
        const gainActive = state.jukeboxPlayer.activeAudioId === 'audioJukeboxA' ? fx.gainA : fx.gainB;
        const gainStandby = state.jukeboxPlayer.activeAudioId === 'audioJukeboxA' ? fx.gainB : fx.gainA;
        if (!state.jukeboxPlayer.crossfading) {
          gainActive.gain.value = active.muted ? 0 : 1;
          gainStandby.gain.value = 0;
        }
      }
    }

    function isJukeboxPlayback() {
      const audio = jukeboxActiveAudio();
      return !!(audio && audio.src);
    }

    function stopJukeboxCrossfade() {
      if (state.jukeboxPlayer?.crossfadeTimer) {
        window.clearInterval(state.jukeboxPlayer.crossfadeTimer);
        state.jukeboxPlayer.crossfadeTimer = null;
      }
      state.jukeboxPlayer.crossfading = false;
      state.jukeboxPlayer.crossfadeTrackId = '';
      state.jukeboxPlayer.crossfadeTargetIndex = -1;
      syncJukeboxDeckVolumes();
    }

    function stopMainPlayback() {
      const audio = mainAudio();
      if (!audio) return;
      try { audio.pause(); } catch (_) {}
      audio.currentTime = 0;
      audio.src = '';
    }

    function stopJukeboxPlayback() {
      [jukeboxActiveAudio(), jukeboxStandbyAudio()].forEach((audio) => {
        if (!audio) return;
        try { audio.pause(); } catch (_) {}
        audio.currentTime = 0;
        audio.src = '';
      });
      stopJukeboxCrossfade();
      state.jukeboxPlayer.activeAudioId = 'audioJukeboxA';
      state.jukeboxPlayer.standbyAudioId = 'audioJukeboxB';
    }

    function isPopoutPlayerMode() {
      const u = new URL(window.location.href);
      return (u.searchParams.get('popout_player') || '') === '1';
    }

    function isJukeboxPopoutMode() {
      const u = new URL(window.location.href);
      return (u.searchParams.get('popout_jukebox') || '') === '1';
    }

    function applyPopoutPlayerMode() {
      const anyPopout = !!state.popoutMode || !!state.jukeboxPopoutMode;
      document.body.classList.toggle('popout-player', anyPopout);
      $('popoutPlayer').classList.toggle('hidden', !state.popoutMode);
      $('popoutJukeboxPlayer').classList.toggle('hidden', !state.jukeboxPopoutMode);
      if (state.popoutMode) {
        document.title = 'HEXSONIC Popout Player';
      } else if (state.jukeboxPopoutMode) {
        document.title = 'HEXSONIC Jukebox Popout';
      }
    }

    function parseSRTTimeToMs(v) {
      const m = String(v || '').trim().match(/^(\d{2}):(\d{2}):(\d{2})[,.](\d{1,3})$/);
      if (!m) return 0;
      const hh = Number(m[1] || 0);
      const mm = Number(m[2] || 0);
      const ss = Number(m[3] || 0);
      const ms = Number(String(m[4] || '0').padEnd(3, '0').slice(0, 3));
      return (((hh * 60 + mm) * 60) + ss) * 1000 + ms;
    }

    function parseSRTLyrics(srt) {
      const src = String(srt || '').replace(/\r/g, '').trim();
      if (!src) return [];
      const blocks = src.split(/\n{2,}/);
      const cues = [];
      blocks.forEach((block) => {
        const lines = block.split('\n').map((x) => x.trim()).filter(Boolean);
        if (lines.length < 2) return;
        let timeLineIdx = 0;
        if (!lines[0].includes('-->') && lines[1] && lines[1].includes('-->')) {
          timeLineIdx = 1;
        }
        const timeLine = lines[timeLineIdx];
        if (!timeLine || !timeLine.includes('-->')) return;
        const parts = timeLine.split('-->');
        if (parts.length !== 2) return;
        const startMs = parseSRTTimeToMs(parts[0]);
        const endMs = parseSRTTimeToMs(parts[1]);
        if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) return;
        const text = lines.slice(timeLineIdx + 1).join(' ').trim();
        if (!text) return;
        cues.push({ start_ms: startMs, end_ms: endMs, text });
      });
      return cues.sort((a, b) => a.start_ms - b.start_ms);
    }

    function currentTrackFromQueue() {
      const engine = state.mainPlayer;
      if (engine.queueIndex < 0 || !engine.queue[engine.queueIndex]) return null;
      return engine.queue[engine.queueIndex];
    }

    function currentTrackMeta() {
      return state.currentTrackMeta || currentTrackFromQueue();
    }

    function currentJukeboxTrackFromQueue() {
      const engine = state.jukeboxPlayer;
      if (engine.queueIndex < 0 || !engine.queue[engine.queueIndex]) return null;
      return engine.queue[engine.queueIndex];
    }

    function normTrackID(v) {
      if (v === null || v === undefined) return '';
      return String(v).trim();
    }

    function currentQueueTrackID() {
      const track = currentTrackFromQueue();
      return track ? normTrackID(track.id) : '';
    }

    function normalizeRepeatMode(value) {
      const raw = String(value || '').trim().toLowerCase();
      if (raw === 'all' || raw === 'one') return raw;
      return 'off';
    }

    function resolveMainQueueIndex() {
      const engine = state.mainPlayer;
      if (!Array.isArray(engine.queue) || !engine.queue.length) return -1;
      const direct = Number(engine.queueIndex);
      if (Number.isInteger(direct) && direct >= 0 && direct < engine.queue.length) return direct;
      const nowID = normTrackID(engine.nowPlayingTrackId || state.nowPlayingTrackId);
      if (!nowID) return -1;
      const idx = engine.queue.findIndex((item) => normTrackID(item?.id) === nowID);
      if (idx >= 0) {
        engine.queueIndex = idx;
        syncLegacyPlayerMirror('main');
      }
      return idx;
    }

    function cycleMainRepeatMode() {
      const engine = state.mainPlayer;
      const current = normalizeRepeatMode(engine.repeatMode);
      const next = current === 'off' ? 'all' : current === 'all' ? 'one' : 'off';
      engine.repeatMode = next;
      localStorage.setItem('hex_main_repeat_mode', next);
      syncLegacyPlayerMirror('main');
      updatePlayerButtons();
      emitPlayerState(true);
    }

    function toggleMainShuffle() {
      const engine = state.mainPlayer;
      engine.shuffle = !engine.shuffle;
      localStorage.setItem('hex_main_shuffle', engine.shuffle ? '1' : '0');
      syncLegacyPlayerMirror('main');
      updatePlayerButtons();
      emitPlayerState(true);
    }

    function pickMainAdjacentIndex(direction = 1) {
      const engine = state.mainPlayer;
      const queue = Array.isArray(engine.queue) ? engine.queue : [];
      if (!queue.length) return -1;
      const idx = resolveMainQueueIndex();
      if (idx < 0) return -1;
      const repeatMode = normalizeRepeatMode(engine.repeatMode);
      if (repeatMode === 'one') return idx;
      if (engine.shuffle && queue.length > 1) {
        const choices = queue.map((_, i) => i).filter((i) => i !== idx);
        return choices[Math.floor(Math.random() * choices.length)];
      }
      const candidate = idx + (direction < 0 ? -1 : 1);
      if (candidate < 0) return repeatMode === 'all' ? queue.length - 1 : -1;
      if (candidate >= queue.length) return repeatMode === 'all' ? 0 : -1;
      return candidate;
    }

    function isActiveTrack(trackID) {
      const id = normTrackID(trackID);
      if (!id) return false;
      if (id === normTrackID(state.nowPlayingTrackId)) return true;
      return id === currentQueueTrackID();
    }

    function visualizerBins() {
      const fx = state.audioFX;
      if (!fx || !fx.analyser) return [];
      const bins = new Uint8Array(fx.analyser.frequencyBinCount);
      fx.analyser.getByteFrequencyData(bins);
      const take = Math.min(72, bins.length);
      return Array.from(bins.slice(0, take));
    }

    function buildPlayerSnapshot() {
      const audio = mainAudio();
      const engine = state.mainPlayer;
      const track = currentTrackMeta();
      return {
        at: Date.now(),
        engine: 'main',
        track: track ? {
          id: track.id,
          title: track.title || 'Untitled',
          artist: track.artist || '-',
          album: track.album || '-',
          uploader_name: track.uploader_name || track.owner_sub || '-'
        } : null,
        queue: engine.queue.map((q) => ({
          id: q.id,
          title: q.title || 'Untitled',
          artist: q.artist || '-',
          album: q.album || '-'
        })),
        queue_index: engine.queueIndex,
        shuffle: !!engine.shuffle,
        repeat_mode: normalizeRepeatMode(engine.repeatMode),
        paused: audio.paused,
        muted: audio.muted,
        volume: Math.round((audio.volume || 0) * 100),
        current_time: Number(audio.currentTime || 0),
        duration: Number(audio.duration || 0),
        cover_url: track ? (state.currentCoverURL || coverURLForTrack(track) || '') : '',
        eq: $('eqPreset').value || 'flat',
        lyrics: {
          track_id: engine.currentLyrics?.track_id || '',
          plain: engine.currentLyrics?.plain || '',
          cues: Array.isArray(engine.currentLyrics?.cues) ? engine.currentLyrics.cues : []
        },
        bins: visualizerBins()
      };
    }

    function buildJukeboxSnapshot() {
      const audio = jukeboxActiveAudio();
      const engine = state.jukeboxPlayer;
      const track = currentJukeboxTrackFromQueue();
      return {
        at: Date.now(),
        engine: 'jukebox',
        mode: state.jukebox.mode || 'for_you',
        track: track ? {
          id: track.id,
          title: track.title || 'Untitled',
          artist: track.artist || '-',
          album: track.album || '-',
          uploader_name: track.uploader_name || track.owner_sub || '-',
          reason: track.reason || ''
        } : null,
        queue: engine.queue.map((q) => ({
          id: q.id,
          title: q.title || 'Untitled',
          artist: q.artist || '-',
          album: q.album || '-',
          reason: q.reason || ''
        })),
        queue_index: engine.queueIndex,
        paused: !!audio.paused,
        muted: !!audio.muted,
        volume: Math.round((audio.volume || 0) * 100),
        current_time: Number(audio.currentTime || 0),
        duration: Number(audio.duration || 0),
        cover_url: state.jukeboxCurrentCoverURL || '',
        lyrics: {
          track_id: engine.currentLyrics?.track_id || '',
          plain: engine.currentLyrics?.plain || '',
          cues: Array.isArray(engine.currentLyrics?.cues) ? engine.currentLyrics.cues : []
        },
        bins: visualizerBins()
      };
    }

    function cachePlayerSnapshot(snapshot = null) {
      const payload = snapshot || buildPlayerSnapshot();
      try {
        localStorage.setItem(LAST_PLAYER_SNAPSHOT_KEY, JSON.stringify(payload));
      } catch (_) {}
      return payload;
    }

    function cacheJukeboxSnapshot(snapshot = null) {
      const payload = snapshot || buildJukeboxSnapshot();
      try {
        localStorage.setItem(LAST_JUKEBOX_SNAPSHOT_KEY, JSON.stringify(payload));
      } catch (_) {}
      return payload;
    }

    function emitPlayerState(force = false) {
      const now = Date.now();
      if (!force && now - state.lastBridgeStateAt < 120) return;
      state.lastBridgeStateAt = now;
      const snapshot = cachePlayerSnapshot();
      if (typeof renderManualQueueDock === 'function') renderManualQueueDock();
      if (!state.playerBridge) return;
      state.playerBridge.postMessage({ type: 'player_state', payload: snapshot });
    }

    function buildPlayerTick() {
      const audio = mainAudio();
      return {
        at: Date.now(),
        current_time: Number(audio.currentTime || 0),
        duration: Number(audio.duration || 0),
        paused: !!audio.paused,
        muted: !!audio.muted,
        volume: Math.round((audio.volume || 0) * 100),
        shuffle: !!state.mainPlayer.shuffle,
        repeat_mode: normalizeRepeatMode(state.mainPlayer.repeatMode),
        bins: visualizerBins()
      };
    }

    function emitPlayerTick() {
      if (!state.playerBridge || state.popoutMode) return;
      state.playerBridge.postMessage({ type: 'player_tick', payload: buildPlayerTick() });
    }

    function buildJukeboxTick() {
      const audio = jukeboxActiveAudio();
      return {
        at: Date.now(),
        current_time: Number(audio.currentTime || 0),
        duration: Number(audio.duration || 0),
        paused: !!audio.paused,
        muted: !!audio.muted,
        volume: Math.round((audio.volume || 0) * 100),
        bins: visualizerBins()
      };
    }

    function emitJukeboxState(force = false) {
      const snapshot = cacheJukeboxSnapshot();
      if (!state.jukeboxBridge) return;
      state.jukeboxBridge.postMessage({ type: 'jukebox_state', payload: snapshot, force: !!force });
    }

    function emitJukeboxTick() {
      if (!state.jukeboxBridge || state.jukeboxPopoutMode) return;
      state.jukeboxBridge.postMessage({ type: 'jukebox_tick', payload: buildJukeboxTick() });
    }

    function startBridgeTicker() {
      if (state.bridgeTickTimer || state.popoutMode) return;
      state.bridgeTickTimer = window.setInterval(() => {
        const a = mainAudio();
        if (!a || !a.src) return;
        emitPlayerTick();
        const now = Date.now();
        if (!state.lastFullBridgeStateAt || (now - state.lastFullBridgeStateAt) >= 1000) {
          state.lastFullBridgeStateAt = now;
          emitPlayerState(true);
        }
      }, PLAYER_TICK_MS);
    }

    async function applyPlayerControl(action, value) {
      const audio = mainAudio();
      const engine = state.mainPlayer;
      if (action === 'play_pause') {
        if (!audio.src) return;
        if (audio.paused) {
          await resumeAudioContextIfNeeded();
          await audio.play();
          showPlayer();
        } else {
          audio.pause();
        }
        updatePlayerButtons();
        emitPlayerState(true);
        return;
      }
      if (action === 'prev') {
        if (!engine.queue.length) return;
        const prevIdx = pickMainAdjacentIndex(-1);
        if (prevIdx < 0) return;
        engine.queueIndex = prevIdx;
        syncLegacyPlayerMirror('main');
        await startTrackById(engine.queue[engine.queueIndex].id, engine.queue);
        return;
      }
      if (action === 'next') {
        if (!engine.queue.length) return;
        const nextIdx = pickMainAdjacentIndex(1);
        if (nextIdx < 0) {
          audio.pause();
          updatePlayerButtons();
          emitPlayerState(true);
          return;
        }
        engine.queueIndex = nextIdx;
        syncLegacyPlayerMirror('main');
        await startTrackById(engine.queue[engine.queueIndex].id, engine.queue);
        return;
      }
      if (action === 'toggle_shuffle') {
        toggleMainShuffle();
        return;
      }
      if (action === 'cycle_repeat') {
        cycleMainRepeatMode();
        return;
      }
      if (action === 'seek') {
        if (!audio.duration || Number.isNaN(audio.duration)) return;
        const p = Math.max(0, Math.min(1, Number(value || 0)));
        audio.currentTime = audio.duration * p;
        emitPlayerState(true);
        return;
      }
      if (action === 'volume') {
        const v = Math.max(0, Math.min(100, Number(value || 0)));
        audio.volume = v / 100;
        $('volumeRange').value = String(v);
        emitPlayerState(true);
        return;
      }
      if (action === 'mute_toggle') {
        audio.muted = !audio.muted;
        updatePlayerButtons();
        emitPlayerState(true);
        return;
      }
      if (action === 'set_eq') {
        const preset = String(value || 'flat');
        $('eqPreset').value = preset;
        applyEQPreset(preset);
        emitPlayerState(true);
        return;
      }
      if (action === 'jump_queue_index') {
        const idx = Number(value);
        if (!Number.isFinite(idx) || idx < 0 || idx >= engine.queue.length) return;
        engine.queueIndex = idx;
        syncLegacyPlayerMirror('main');
        await startTrackById(engine.queue[engine.queueIndex].id, engine.queue);
      }
    }

    function openPopoutPlayer() {
      const u = new URL(window.location.href);
      u.searchParams.set('popout_player', '1');
      const w = window.open(u.toString(), 'hexsonic_popout_player', 'width=1280,height=880,resizable=yes,scrollbars=no');
      if (!w) {
        alert('Popup blocked. Please allow popups for this site.');
        return;
      }
      state.popoutWindow = w;
      emitPlayerState(true);
    }

    function openJukeboxPopout() {
      const u = new URL(window.location.href);
      u.searchParams.set('popout_jukebox', '1');
      const w = window.open(u.toString(), 'hexsonic_jukebox_popout', 'width=1280,height=880,resizable=yes,scrollbars=no');
      if (!w) {
        alert('Popup blocked. Please allow popups for this site.');
        return;
      }
      cacheJukeboxSnapshot();
      emitJukeboxState(true);
    }

    function lyricActiveIndex(cues, nowMs) {
      for (let i = 0; i < cues.length; i += 1) {
        const c = cues[i];
        if (nowMs >= Number(c.start_ms || 0) && nowMs < Number(c.end_ms || 0)) return i;
      }
      return -1;
    }

    function drawCanvasLyrics(ctx, w, h, snapshot, currentTimeSec) {
      const lyrics = snapshot && snapshot.lyrics ? snapshot.lyrics : {};
      const cues = Array.isArray(lyrics.cues) ? lyrics.cues : [];
      const baseY = Math.floor(h * 0.5);
      const fontPx = Math.max(11, Math.min(22, Number(state.popLyricFontPx || 13)));
      const lineH = Math.round(fontPx * 1.62);

      // Keep the lyric focus band always visible so brightness does not jump between cue gaps.
      const gradTop = ctx.createLinearGradient(0, baseY - lineH * 2.2, 0, baseY);
      gradTop.addColorStop(0, 'rgba(8,12,20,0)');
      gradTop.addColorStop(1, 'rgba(8,12,20,0.36)');
      ctx.fillStyle = gradTop;
      ctx.fillRect(0, baseY - lineH * 2.2, w, lineH * 2.2);
      const gradBot = ctx.createLinearGradient(0, baseY, 0, baseY + lineH * 2.2);
      gradBot.addColorStop(0, 'rgba(8,12,20,0.36)');
      gradBot.addColorStop(1, 'rgba(8,12,20,0)');
      ctx.fillStyle = gradBot;
      ctx.fillRect(0, baseY, w, lineH * 2.2);

      if (!cues.length) {
        $('popLyricMode').textContent = 'No SRT';
        return;
      }
      $('popLyricMode').textContent = 'SRT sync active';
      const nowMs = Math.round(Number(currentTimeSec || 0) * 1000);
      const activeIdx = lyricActiveIndex(cues, nowMs);
      if (activeIdx < 0) return;

      const activeCue = cues[activeIdx];
      const from = Math.max(0, activeIdx - 2);
      const to = Math.min(cues.length - 1, activeIdx + 2);
      const cueFade = Math.max(0, Math.min(1, (nowMs - Number(activeCue.start_ms || 0)) / 300));

      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.font = `600 ${fontPx}px "Segoe UI", Tahoma, sans-serif`;
      for (let i = from; i <= to; i += 1) {
        const rel = i - activeIdx;
        const y = Math.round(baseY + rel * lineH);
        if (y < 20 || y > h - 12) continue;
        const d = Math.abs(rel);
        const alpha = Math.max(0.08, 1 - Math.pow(d * 0.55, 1.24));
        const active = rel === 0;
        const sizeBoost = active ? 2 : 0;
        ctx.font = `${active ? 700 : 550} ${fontPx + sizeBoost}px "Segoe UI", Tahoma, sans-serif`;
        let dynamicAlpha = alpha;
        if (active) dynamicAlpha = Math.max(0.1, Math.min(1, (0.74 + (cueFade * 0.26))));
        else if (rel === -1) dynamicAlpha = Math.max(0.08, alpha * (0.92 - (cueFade * 0.22)));
        else if (rel === 1) dynamicAlpha = Math.max(0.08, alpha * (0.82 + (cueFade * 0.08)));
        ctx.fillStyle = active ? `rgba(245,251,255,${Math.min(1, dynamicAlpha + 0.08)})` : `rgba(165,192,230,${dynamicAlpha})`;
        if (active) {
          ctx.strokeStyle = `rgba(12,18,28,${Math.min(0.9, alpha + 0.2)})`;
          ctx.lineWidth = 3;
          ctx.strokeText(String(cues[i].text || ''), Math.floor(w / 2), y);
        }
        ctx.fillText(String(cues[i].text || ''), Math.floor(w / 2), y);
      }
    }

    function preparePopVizCanvas() {
      const cvs = $('popViz');
      if (!cvs) return { ctx: null, w: 0, h: 0 };
      const rect = cvs.getBoundingClientRect();
      const cssW = Math.max(320, Math.floor(rect.width || 0));
      const cssH = Math.max(220, Math.floor(rect.height || 0));
      const dpr = Math.max(1, Math.min(2, Number(window.devicePixelRatio || 1)));
      const pxW = Math.max(1, Math.floor(cssW * dpr));
      const pxH = Math.max(1, Math.floor(cssH * dpr));
      if (cvs.width !== pxW || cvs.height !== pxH) {
        cvs.width = pxW;
        cvs.height = pxH;
      }
      const ctx = cvs.getContext('2d');
      if (!ctx) return { ctx: null, w: cssW, h: cssH };
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      return { ctx, w: cssW, h: cssH };
    }

    function renderPopoutVisualizer(bins, snapshot, currentTimeSec) {
      const prep = preparePopVizCanvas();
      const ctx = prep.ctx;
      if (!ctx) return;
      const w = prep.w;
      const h = prep.h;
      ctx.clearRect(0, 0, w, h);
      const raw = Array.isArray(bins) && bins.length ? bins : [];
      const bars = Math.max(24, raw.length || 40);
      const prev = Array.isArray(state.popVizPrevBins) ? state.popVizPrevBins : [];
      const attack = 0.88; // react fast on rising energy
      const release = 0.52; // decay slower, but not sluggish
      const arr = new Array(bars).fill(0).map((_, i) => {
        const v = raw.length ? Number(raw[i % raw.length] || 0) : 0;
        const p = Number(prev[i] || 0);
        const smoothed = v >= p
          ? ((p * (1 - attack)) + (v * attack))
          : ((p * (1 - release)) + (v * release));
        const transient = Math.max(0, v - p) * 0.2;
        return Math.round(Math.max(0, Math.min(255, smoothed + transient)));
      });
      state.popVizPrevBins = arr;
      const mode = String(state.popVizMode || 'bars');
      const padX = 10;
      const innerW = Math.max(1, w - (padX * 2));
      const slotW = innerW / bars;
      const bg = ctx.createLinearGradient(0, 0, 0, h);
      bg.addColorStop(0, 'rgba(24,39,62,0.22)');
      bg.addColorStop(1, 'rgba(7,12,20,0.15)');
      ctx.fillStyle = bg;
      ctx.fillRect(0, 0, w, h);

      if (mode === 'mirror') {
        const mid = Math.floor(h / 2);
        for (let i = 0; i < bars; i += 1) {
          const x = Math.floor(padX + (i * slotW));
          const xNext = Math.floor(padX + ((i + 1) * slotW));
          const bw = Math.max(1, xNext - x - 1);
          const v = Math.max(0, arr[i] / 255);
          const bh = Math.max(2, Math.floor((h * 0.45) * v));
          const grad = ctx.createLinearGradient(0, mid - bh, 0, mid + bh);
          grad.addColorStop(0, '#b6dcff');
          grad.addColorStop(1, '#386298');
          ctx.fillStyle = grad;
          ctx.fillRect(x, mid - bh, bw, bh * 2);
        }
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      if (mode === 'ring') {
        const cx = Math.floor(w / 2);
        const cy = Math.floor(h / 2);
        const baseR = Math.min(w, h) * 0.22;
        const n = Math.min(96, bars);
        ctx.lineWidth = 2;
        for (let i = 0; i < n; i += 1) {
          const a = (Math.PI * 2 * i) / n;
          const v = Math.max(0, arr[i % bars] / 255);
          const ext = 22 + Math.floor(v * 70);
          const x1 = cx + Math.cos(a) * baseR;
          const y1 = cy + Math.sin(a) * baseR;
          const x2 = cx + Math.cos(a) * (baseR + ext);
          const y2 = cy + Math.sin(a) * (baseR + ext);
          ctx.strokeStyle = `rgba(${120 + Math.floor(v * 120)}, ${170 + Math.floor(v * 60)}, 255, 0.9)`;
          ctx.beginPath();
          ctx.moveTo(x1, y1);
          ctx.lineTo(x2, y2);
          ctx.stroke();
        }
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      if (mode === 'wave') {
        ctx.lineWidth = 2;
        const mid = Math.floor(h * 0.5);
        ctx.strokeStyle = 'rgba(158,205,255,0.95)';
        ctx.beginPath();
        for (let x = 0; x < w; x += 4) {
          const i = Math.floor((x / w) * bars) % bars;
          const v = arr[i] / 255;
          const y = mid + Math.sin((x / 52) + (Date.now() / 380)) * (22 + v * 64);
          if (x === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      if (mode === 'pulse') {
        const bass = arr.slice(0, Math.max(4, Math.floor(arr.length * 0.12))).reduce((a, b) => a + b, 0) / Math.max(1, Math.floor(arr.length * 0.12));
        const v = bass / 255;
        const cx = Math.floor(w / 2);
        const cy = Math.floor(h * 0.46);
        const r = Math.floor(Math.min(w, h) * (0.14 + v * 0.2));
        const rg = ctx.createRadialGradient(cx, cy, Math.max(6, r * 0.2), cx, cy, r * 1.35);
        rg.addColorStop(0, 'rgba(176,220,255,0.95)');
        rg.addColorStop(1, 'rgba(32,72,122,0.06)');
        ctx.fillStyle = rg;
        ctx.beginPath();
        ctx.arc(cx, cy, r * 1.35, 0, Math.PI * 2);
        ctx.fill();
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      if (mode === 'skyline') {
        for (let i = 0; i < bars; i += 1) {
          const x = Math.floor(padX + (i * slotW));
          const xNext = Math.floor(padX + ((i + 1) * slotW));
          const bw = Math.max(1, xNext - x);
          const v = Math.max(0, arr[i] / 255);
          const bh = Math.max(2, Math.floor((h - 18) * v));
          const y = h - 8 - bh;
          ctx.fillStyle = `rgba(${80 + Math.floor(v * 140)}, ${120 + Math.floor(v * 110)}, 230, 0.88)`;
          ctx.fillRect(x, y, bw, bh);
          ctx.fillStyle = 'rgba(180,220,255,0.35)';
          ctx.fillRect(x, y, bw, 2);
        }
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      if (mode === 'scope') {
        const mid = Math.floor(h * 0.44);
        ctx.lineWidth = 2;
        ctx.strokeStyle = 'rgba(166,216,255,0.94)';
        ctx.beginPath();
        for (let x = 0; x < w; x += 3) {
          const i = Math.floor((x / w) * bars) % bars;
          const j = (i + 7) % bars;
          const a = (arr[i] - 128) / 128;
          const b = (arr[j] - 128) / 128;
          const y = mid + (a * 40) + (b * 22);
          if (x === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        }
        ctx.stroke();
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      if (mode === 'orb') {
        const cx = Math.floor(w / 2);
        const cy = Math.floor(h * 0.48);
        const base = Math.max(38, Math.floor(Math.min(w, h) * 0.12));
        const n = Math.min(84, bars);
        ctx.lineWidth = 1.5;
        for (let i = 0; i < n; i += 1) {
          const v = Math.max(0, arr[i % bars] / 255);
          const ang = (Math.PI * 2 * i) / n;
          const r = base + Math.floor(v * 58);
          const x = cx + Math.cos(ang) * r;
          const y = cy + Math.sin(ang) * r;
          ctx.strokeStyle = `rgba(${92 + Math.floor(v * 140)}, ${160 + Math.floor(v * 86)}, 255, 0.88)`;
          ctx.beginPath();
          ctx.moveTo(cx, cy);
          ctx.lineTo(x, y);
          ctx.stroke();
        }
        drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
        return;
      }
      for (let i = 0; i < bars; i += 1) {
        const x = Math.floor(padX + (i * slotW));
        const xNext = Math.floor(padX + ((i + 1) * slotW));
        const bw = Math.max(1, xNext - x - 1);
        const v = Math.max(0, arr[i] / 255);
        const bh = Math.max(4, Math.floor((h - 18) * v));
        const y = h - 8 - bh;
        const grad = ctx.createLinearGradient(0, y, 0, y + bh);
        grad.addColorStop(0, '#93c4ff');
        grad.addColorStop(1, '#2f4f7e');
        ctx.fillStyle = grad;
        ctx.fillRect(x, y, bw, bh);
      }
      drawCanvasLyrics(ctx, w, h, snapshot || state.popoutSnapshot || {}, currentTimeSec || 0);
    }

    function renderPopoutQueue(snapshot) {
      const q = Array.isArray(snapshot.queue) ? snapshot.queue : [];
      const idx = Number(snapshot.queue_index || -1);
      const needle = String(state.popQueueFilter || '').trim().toLowerCase();
      const indexed = q.map((item, queueIndex) => ({ item, queueIndex }));
      const rows = !needle ? indexed : indexed.filter(({ item }) => {
        const hay = `${item.title || ''} ${item.artist || ''} ${item.album || ''}`.toLowerCase();
        return hay.includes(needle);
      });
      $('popQueueMeta').textContent = needle ? `${rows.length}/${q.length} tracks` : `${q.length} tracks`;
      const root = $('popQueue');
      root.innerHTML = '';
      if (!rows.length) {
        root.innerHTML = '<div class="muted" style="padding:10px;">Queue empty.</div>';
        return;
      }
      rows.forEach(({ item, queueIndex }) => {
        const row = document.createElement('div');
        row.className = `pop-q-item ${queueIndex === idx ? 'active' : ''}`;
        row.innerHTML = `
          <div>
            <div class="pop-q-title">${escapeHtml(item.title || '-')}</div>
            <div class="pop-q-meta">${escapeHtml(item.artist || '-')} · ${escapeHtml(item.album || '-')}</div>
          </div>
        `;
        row.onclick = () => {
          if (!state.playerBridge) return;
          state.playerBridge.postMessage({ type: 'player_control', action: 'jump_queue_index', value: queueIndex });
        };
        root.appendChild(row);
      });
      if (state.popQueueFollowCurrent && idx >= 0) {
        const active = root.querySelector('.pop-q-item.active');
        if (active) active.scrollIntoView({ block: 'nearest', behavior: 'auto' });
      }
    }

    function renderPopoutSnapshot(snapshot) {
      if (!state.popoutMode) return;
      state.popoutSnapshot = snapshot || {};
      const t = snapshot && snapshot.track ? snapshot.track : null;
      if (t && state.popoutSyncTimer) {
        window.clearInterval(state.popoutSyncTimer);
        state.popoutSyncTimer = null;
      }
      $('popTitle').textContent = t ? (t.title || 'Untitled') : 'Nothing playing';
      $('popSub').textContent = t ? `${t.artist || '-'} · ${t.album || '-'} · by ${t.uploader_name || '-'}` : 'Waiting for player data...';
      if (snapshot && snapshot.cover_url) {
        $('popCover').style.backgroundImage = `linear-gradient(180deg, rgba(0,0,0,.1), rgba(0,0,0,.45)), url('${snapshot.cover_url}')`;
      } else {
        $('popCover').style.backgroundImage = 'linear-gradient(135deg, #2d3e5a, #5c79a6)';
      }
      $('popElapsed').textContent = fmt(Number(snapshot.current_time || 0));
      $('popDuration').textContent = fmt(Number(snapshot.duration || 0));
      const d = Number(snapshot.duration || 0);
      const p = d > 0 ? Number(snapshot.current_time || 0) / d : 0;
      $('popSeek').value = String(Math.max(0, Math.min(1000, Math.round(p * 1000))));
      $('popVolume').value = String(Math.max(0, Math.min(100, Number(snapshot.volume || 0))));
      $('popEqPreset').value = snapshot.eq || 'flat';
      $('popPlayPause').textContent = snapshot.paused ? ICON_PLAY : ICON_PAUSE;
      $('popMute').textContent = snapshot.muted ? ICON_MUTE : ICON_UNMUTE;
      $('popShuffle').classList.toggle('active', !!snapshot.shuffle);
      $('popShuffle').title = snapshot.shuffle ? 'Shuffle on' : 'Shuffle off';
      const popRepeat = $('popRepeat');
      const repeatMode = normalizeRepeatMode(snapshot.repeat_mode);
      popRepeat.dataset.mode = repeatMode;
      popRepeat.textContent = repeatMode === 'one' ? '↻1' : '↻';
      popRepeat.classList.toggle('active', repeatMode !== 'off');
      popRepeat.title = `Repeat ${repeatMode}`;
      renderPopoutVisualizer(snapshot.bins || [], snapshot || {}, Number(snapshot.current_time || 0));
      renderPopoutQueue(snapshot);
    }

    function applyPopoutTick(tick) {
      if (!state.popoutMode || !state.popoutSnapshot) return;
      const t = tick || {};
      state.popoutSnapshot.current_time = Number(t.current_time || 0);
      state.popoutSnapshot.duration = Number(t.duration || 0);
      state.popoutSnapshot.paused = !!t.paused;
      state.popoutSnapshot.muted = !!t.muted;
      state.popoutSnapshot.volume = Math.max(0, Math.min(100, Number(t.volume || 0)));
      $('popElapsed').textContent = fmt(state.popoutSnapshot.current_time);
      $('popDuration').textContent = fmt(state.popoutSnapshot.duration);
      const d = Number(state.popoutSnapshot.duration || 0);
      const p = d > 0 ? state.popoutSnapshot.current_time / d : 0;
      $('popSeek').value = String(Math.max(0, Math.min(1000, Math.round(p * 1000))));
      $('popVolume').value = String(state.popoutSnapshot.volume);
      $('popPlayPause').textContent = state.popoutSnapshot.paused ? ICON_PLAY : ICON_PAUSE;
      $('popMute').textContent = state.popoutSnapshot.muted ? ICON_MUTE : ICON_UNMUTE;
      state.popoutSnapshot.shuffle = !!t.shuffle;
      state.popoutSnapshot.repeat_mode = normalizeRepeatMode(t.repeat_mode);
      $('popShuffle').classList.toggle('active', !!state.popoutSnapshot.shuffle);
      $('popShuffle').title = state.popoutSnapshot.shuffle ? 'Shuffle on' : 'Shuffle off';
      const popRepeat = $('popRepeat');
      popRepeat.dataset.mode = state.popoutSnapshot.repeat_mode;
      popRepeat.textContent = state.popoutSnapshot.repeat_mode === 'one' ? '↻1' : '↻';
      popRepeat.classList.toggle('active', state.popoutSnapshot.repeat_mode !== 'off');
      popRepeat.title = `Repeat ${state.popoutSnapshot.repeat_mode}`;
      renderPopoutVisualizer(Array.isArray(t.bins) ? t.bins : [], state.popoutSnapshot || {}, state.popoutSnapshot.current_time);
    }

    function renderJukeboxPopoutQueue(snapshot) {
      const root = $('jukePopQueue');
      if (!root) return;
      const q = Array.isArray(snapshot?.queue) ? snapshot.queue : [];
      const idx = Number(snapshot?.queue_index || -1);
      root.innerHTML = '';
      const upcoming = q.filter((_, i) => i > idx);
      if (!upcoming.length) {
        root.innerHTML = '<div class="muted" style="padding:10px;">Queue empty.</div>';
        return;
      }
      upcoming.forEach((item) => {
        const row = document.createElement('div');
        row.className = 'pop-q-item';
        row.innerHTML = `
          <div>
            <div class="pop-q-title">${escapeHtml(item.title || '-')}</div>
            <div class="pop-q-meta">${escapeHtml(item.artist || '-')} · ${escapeHtml(item.album || '-')}</div>
          </div>
        `;
        root.appendChild(row);
      });
    }

    function renderJukeboxPopoutSnapshot(snapshot) {
      if (!state.jukeboxPopoutMode) return;
      state.jukeboxPopoutSnapshot = snapshot || {};
      const t = snapshot?.track || null;
      $('jukePopTitle').textContent = t ? (t.title || 'Untitled') : 'Nothing playing';
      $('jukePopSub').textContent = t ? `${t.artist || '-'} · ${t.album || '-'} · by ${t.uploader_name || '-'}` : 'Start a Jukebox session.';
      if (snapshot?.cover_url) {
        $('jukePopCover').style.backgroundImage = `linear-gradient(180deg, rgba(0,0,0,.1), rgba(0,0,0,.45)), url('${snapshot.cover_url}')`;
      } else {
        $('jukePopCover').style.backgroundImage = 'linear-gradient(135deg, #2d3e5a, #5c79a6)';
      }
      $('jukePopElapsed').textContent = fmt(Number(snapshot?.current_time || 0));
      $('jukePopDuration').textContent = fmt(Number(snapshot?.duration || 0));
      const d = Number(snapshot?.duration || 0);
      const p = d > 0 ? Number(snapshot?.current_time || 0) / d : 0;
      $('jukePopProgressFill').style.width = `${Math.max(0, Math.min(100, p * 100)).toFixed(2)}%`;
      renderJukeboxPopoutQueue(snapshot || {});
    }

    function applyJukeboxPopoutTick(tick) {
      if (!state.jukeboxPopoutMode || !state.jukeboxPopoutSnapshot) return;
      const t = tick || {};
      state.jukeboxPopoutSnapshot.current_time = Number(t.current_time || 0);
      state.jukeboxPopoutSnapshot.duration = Number(t.duration || 0);
      state.jukeboxPopoutSnapshot.paused = !!t.paused;
      state.jukeboxPopoutSnapshot.muted = !!t.muted;
      state.jukeboxPopoutSnapshot.volume = Math.max(0, Math.min(100, Number(t.volume || 0)));
      $('jukePopElapsed').textContent = fmt(state.jukeboxPopoutSnapshot.current_time);
      $('jukePopDuration').textContent = fmt(state.jukeboxPopoutSnapshot.duration);
      const d = Number(state.jukeboxPopoutSnapshot.duration || 0);
      const p = d > 0 ? state.jukeboxPopoutSnapshot.current_time / d : 0;
      $('jukePopProgressFill').style.width = `${Math.max(0, Math.min(100, p * 100)).toFixed(2)}%`;
    }

    function requestPopoutStateSync() {
      if (!state.popoutMode || !state.playerBridge) return;
      const parsed = readCachedPlayerSnapshot();
      if (parsed && parsed.track && parsed.track.id) {
        renderPopoutSnapshot(parsed);
      }
      state.playerBridge.postMessage({ type: 'player_ready' });
    }

    function readCachedJukeboxSnapshot() {
      try {
        const cached = localStorage.getItem(LAST_JUKEBOX_SNAPSHOT_KEY);
        if (!cached) return null;
        const parsed = JSON.parse(cached);
        return parsed && typeof parsed === 'object' ? parsed : null;
      } catch (_) {
        return null;
      }
    }

    function requestJukeboxPopoutStateSync() {
      const parsed = readCachedJukeboxSnapshot();
      if (parsed) renderJukeboxPopoutSnapshot(parsed);
      if (state.jukeboxBridge) state.jukeboxBridge.postMessage({ type: 'jukebox_ready' });
    }

    function readCachedPlayerSnapshot() {
      try {
        const cached = localStorage.getItem(LAST_PLAYER_SNAPSHOT_KEY);
        if (!cached) return null;
        const parsed = JSON.parse(cached);
        return parsed && typeof parsed === 'object' ? parsed : null;
      } catch (_) {
        return null;
      }
    }

    function startPopoutSnapshotPolling() {
      if (!state.popoutMode) return;
      const syncFromCache = () => {
        const parsed = readCachedPlayerSnapshot();
        if (!parsed) return;
        renderPopoutSnapshot(parsed);
      };
      syncFromCache();
      window.addEventListener('storage', (ev) => {
        if (ev.key !== LAST_PLAYER_SNAPSHOT_KEY) return;
        syncFromCache();
      });
      if (!state.popoutSyncTimer) {
        state.popoutSyncTimer = window.setInterval(() => {
          syncFromCache();
          if (state.playerBridge) state.playerBridge.postMessage({ type: 'player_ready' });
        }, 1000);
      }
    }

    function initPlayerBridge() {
      if (state.playerBridge) return;
      if (typeof BroadcastChannel !== 'function') {
        if (state.popoutMode) startPopoutSnapshotPolling();
        return;
      }
      state.playerBridge = new BroadcastChannel(PLAYER_BRIDGE_CHANNEL);
      state.playerBridge.onmessage = async (ev) => {
        const msg = ev && ev.data ? ev.data : {};
        const type = String(msg.type || '');
        if (state.popoutMode) {
          if (type === 'player_state') {
            renderPopoutSnapshot(msg.payload || {});
          }
          if (type === 'player_tick') {
            applyPopoutTick(msg.payload || {});
          }
          if (type === 'player_ping') {
            state.playerBridge.postMessage({ type: 'player_ready' });
          }
          return;
        }
        if (type === 'player_ready') {
          emitPlayerState(true);
          return;
        }
        if (type === 'player_control') {
          await applyPlayerControl(msg.action, msg.value);
        }
      };
      if (state.popoutMode) {
        const parsed = readCachedPlayerSnapshot();
        if (parsed) renderPopoutSnapshot(parsed);
        requestPopoutStateSync();
        startPopoutSnapshotPolling();
      } else {
        state.playerBridge.postMessage({ type: 'player_ping' });
      }
    }

    function initJukeboxBridge() {
      if (state.jukeboxBridge) return;
      if (typeof BroadcastChannel !== 'function') return;
      state.jukeboxBridge = new BroadcastChannel(JUKEBOX_BRIDGE_CHANNEL);
      state.jukeboxBridge.onmessage = async (ev) => {
        const msg = ev?.data || {};
        const type = String(msg.type || '');
        if (state.jukeboxPopoutMode) {
          if (type === 'jukebox_state') renderJukeboxPopoutSnapshot(msg.payload || {});
          if (type === 'jukebox_tick') applyJukeboxPopoutTick(msg.payload || {});
          return;
        }
        if (type === 'jukebox_ready') {
          emitJukeboxState(true);
          return;
        }
        if (type === 'jukebox_control' && typeof ns.applyJukeboxControl === 'function') {
          await ns.applyJukeboxControl(msg.action, msg.value);
        }
      };
      if (state.jukeboxPopoutMode) {
        const parsed = readCachedJukeboxSnapshot();
        if (parsed) renderJukeboxPopoutSnapshot(parsed);
        requestJukeboxPopoutStateSync();
        if (!state.jukeboxPopoutSyncTimer) {
          state.jukeboxPopoutSyncTimer = window.setInterval(() => {
            requestJukeboxPopoutStateSync();
          }, 1000);
        }
      }
    }

    function bindPopoutEvents() {
      $('popClose').onclick = () => window.close();
      $('popShuffle').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'toggle_shuffle' });
      $('popPlayPause').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'play_pause' });
      $('popPrev').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'prev' });
      $('popNext').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'next' });
      $('popRepeat').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'cycle_repeat' });
      $('popMute').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'mute_toggle' });
      $('popSeek').oninput = (e) => {
        if (!state.playerBridge) return;
        state.playerBridge.postMessage({ type: 'player_control', action: 'seek', value: Number(e.target.value) / 1000 });
      };
      $('popVolume').oninput = (e) => {
        if (!state.playerBridge) return;
        state.playerBridge.postMessage({ type: 'player_control', action: 'volume', value: Number(e.target.value) });
      };
      $('popEqPreset').onchange = (e) => {
        if (!state.playerBridge) return;
        state.playerBridge.postMessage({ type: 'player_control', action: 'set_eq', value: String(e.target.value || 'flat') });
      };
      $('popVizMode').onchange = (e) => {
        state.popVizMode = String(e.target.value || 'bars');
        localStorage.setItem('hex_pop_viz_mode', state.popVizMode);
      };
      $('popLyricAutoScroll').onchange = (e) => {
        state.popLyricAutoScroll = !!e.target.checked;
        localStorage.setItem('hex_pop_lyric_autoscroll', state.popLyricAutoScroll ? '1' : '0');
      };
      $('popLyricFontDown').onclick = () => {
        state.popLyricFontPx = Math.max(11, Number(state.popLyricFontPx || 13) - 1);
        localStorage.setItem('hex_pop_lyric_font_px', String(state.popLyricFontPx));
        renderPopoutVisualizer((state.popoutSnapshot && state.popoutSnapshot.bins) || [], state.popoutSnapshot || {}, Number((state.popoutSnapshot && state.popoutSnapshot.current_time) || 0));
      };
      $('popLyricFontUp').onclick = () => {
        state.popLyricFontPx = Math.min(22, Number(state.popLyricFontPx || 13) + 1);
        localStorage.setItem('hex_pop_lyric_font_px', String(state.popLyricFontPx));
        renderPopoutVisualizer((state.popoutSnapshot && state.popoutSnapshot.bins) || [], state.popoutSnapshot || {}, Number((state.popoutSnapshot && state.popoutSnapshot.current_time) || 0));
      };
      $('popQueueFilter').oninput = (e) => {
        state.popQueueFilter = String(e.target.value || '');
        renderPopoutQueue(state.popoutSnapshot || { queue: [], queue_index: -1 });
      };
      $('popQueueFollowCurrent').onchange = (e) => {
        state.popQueueFollowCurrent = !!e.target.checked;
        localStorage.setItem('hex_pop_queue_follow', state.popQueueFollowCurrent ? '1' : '0');
      };
      window.addEventListener('resize', () => {
        renderPopoutVisualizer((state.popoutSnapshot && state.popoutSnapshot.bins) || [], state.popoutSnapshot || {}, Number((state.popoutSnapshot && state.popoutSnapshot.current_time) || 0));
      });
      window.addEventListener('keydown', (e) => {
        const tag = (e.target && e.target.tagName) ? e.target.tagName.toLowerCase() : '';
        if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
        const key = String(e.key || '').toLowerCase();
        if (key === ' ') {
          e.preventDefault();
          $('popPlayPause').click();
        } else if (key === 'arrowright') {
          e.preventDefault();
          $('popNext').click();
        } else if (key === 'arrowleft') {
          e.preventDefault();
          $('popPrev').click();
        } else if (key === 'm') {
          e.preventDefault();
          $('popMute').click();
        } else if (key === 'f') {
          e.preventDefault();
          $('popQueueFilter').focus();
        }
      });
      $('popVizMode').value = state.popVizMode;
      $('popLyricAutoScroll').checked = !!state.popLyricAutoScroll;
      $('popQueueFollowCurrent').checked = !!state.popQueueFollowCurrent;
      state.popLyricFontPx = Math.max(11, Math.min(22, Number(state.popLyricFontPx || 13)));
    }

    function bindJukeboxPopoutEvents() {
      $('jukePopClose').onclick = () => window.close();
    }

    function syncRoleUI() {
      const loggedIn = !!state.me;
      const creatorNavVisible = canUpload();
      const adminNavVisible = canAdmin();
      $('btnOpenLogin').classList.toggle('hidden', loggedIn);
      $('btnLogout').classList.toggle('hidden', !loggedIn);
      $('btnNotifications').classList.toggle('hidden', !loggedIn);
      $('btnNotifications').textContent = state.notificationsUnread > 0 ? `Inbox (${state.notificationsUnread})` : 'Inbox';
      $('btnNotifications').classList.toggle('has-alert', loggedIn && state.notificationsUnread > 0);
      $('navGroupCreator').classList.toggle('hidden', !creatorNavVisible);
      $('navGroupAdmin').classList.toggle('hidden', !adminNavVisible);
      $('navPlaylists').classList.toggle('hidden', !loggedIn);
      $('navFavorites').classList.toggle('hidden', !loggedIn);
      $('navProfile').classList.toggle('hidden', !loggedIn);
      $('navAdminUsers').classList.toggle('hidden', !adminNavVisible);
      $('navAdminSystem').classList.toggle('hidden', !adminNavVisible);
      $('navAdminLogs').classList.toggle('hidden', !adminNavVisible);
      syncPublicUI();

      $('navUpload').classList.toggle('hidden', !creatorNavVisible);
      $('navCreatorStats').classList.toggle('hidden', !creatorNavVisible);
      $('navJobs').classList.toggle('hidden', !adminNavVisible);

      if (!canUpload() && !$('viewUpload').classList.contains('hidden')) {
        switchView('albums');
      }
      if (!canManage() && (!$('viewTrackManage').classList.contains('hidden') || !$('viewAlbumManage').classList.contains('hidden'))) {
        switchView('albums');
      }
      if (!canUpload() && !$('viewCreatorStats').classList.contains('hidden')) {
        switchView('albums');
      }
      if (!canAdmin() && !$('viewJobs').classList.contains('hidden')) {
        switchView('albums');
      }
      if (!loggedIn && !$('viewPlaylists').classList.contains('hidden')) {
        switchView('albums');
      }
      if (!loggedIn && !$('viewFavorites').classList.contains('hidden')) {
        switchView('albums');
      }
      if (!loggedIn && !$('viewProfile').classList.contains('hidden')) {
        switchView('albums');
      }
      if (!canAdmin() && (!$('viewAdminUsers').classList.contains('hidden') || !$('viewAdminSystem').classList.contains('hidden') || !$('viewAdminLogs').classList.contains('hidden'))) {
        switchView('albums');
      }
    }

    function closePanels() {
      $('authDrawer').classList.add('hidden');
      $('signupDrawer').classList.add('hidden');
    }

    function showPlayer() {
      $('playerBar').classList.remove('hidden');
    }

    function hidePlayer() {
      $('playerBar').classList.remove('hidden');
    }

    async function openAlbumFromPlayerThumb() {
      const track = currentTrackFromQueue();
      if (!track) return;
      const album = findAlbumForTrack(track);
      if (!album || !album.id) return;
      await selectAlbum(album.id);
    }

    function updatePlayerButtons() {
      const a = activeAudio();
      const volumePct = Math.round(((a.volume || 0) * 100));
      $('btnPlayPause').textContent = a.paused ? ICON_PLAY : ICON_PAUSE;
      $('btnMute').textContent = a.muted ? ICON_MUTE : ICON_UNMUTE;
      $('btnMute').classList.toggle('is-muted', !!a.muted);
      $('btnMute').title = a.muted ? 'Unmute audio' : 'Mute audio';
      $('btnMute').setAttribute('aria-label', a.muted ? 'Unmute audio' : 'Mute audio');
      $('volumeValueLabel').textContent = a.muted ? `Muted ${volumePct}%` : `${volumePct}%`;
      const shuffleBtn = $('btnShuffle');
      if (shuffleBtn) {
        shuffleBtn.classList.toggle('active', !!state.mainPlayer.shuffle);
        shuffleBtn.title = state.mainPlayer.shuffle ? 'Shuffle on' : 'Shuffle off';
      }
      const repeatBtn = $('btnRepeat');
      if (repeatBtn) {
        const repeatMode = normalizeRepeatMode(state.mainPlayer.repeatMode);
        repeatBtn.dataset.mode = repeatMode;
        repeatBtn.textContent = repeatMode === 'one' ? '↻1' : '↻';
        repeatBtn.classList.toggle('active', repeatMode !== 'off');
        repeatBtn.title = `Repeat ${repeatMode}`;
      }
    }

    function setPlayerThumb(url = '') {
      const thumb = $('playerThumb');
      if (!thumb) return;
      if (url) {
        thumb.style.backgroundImage = `linear-gradient(180deg, rgba(0,0,0,0.1), rgba(0,0,0,0.38)), url('${url}')`;
      } else {
        thumb.style.backgroundImage = 'linear-gradient(135deg, #25344d, #5672a3)';
      }
    }


    function findAlbumForTrack(track) {
      if (!track) return null;
      const directAlbumID = Number(track.album_id || track.albumID || 0);
      if (directAlbumID > 0) {
        const direct = state.albums.find((a) => Number(a.id || 0) === directAlbumID);
        if (direct) return direct;
      }
      return state.albums.find((a) => {
        if (normText(a.title) !== normText(track.album)) return false;
        if (!track.artist || !a.artist) return true;
        return normText(a.artist) === normText(track.artist);
      }) || state.albums.find((a) => normText(a.title) === normText(track.album)) || null;
    }

    function coverURLForTrack(track) {
      const album = findAlbumForTrack(track);
      if (!album) return '';
      return state.albumCoverURLs[album.id] || '';
    }

    async function ensureCoverURLForAlbum(album) {
      if (!album) return '';
      const existing = state.albumCoverURLs[album.id];
      if (existing) return existing;
      if (!album.cover_path) return '';

      if (album.visibility === 'public') {
        const url = `/api/v1/albums/${album.id}/cover?v=${Date.now()}`;
        state.albumCoverURLs[album.id] = url;
        return url;
      }

      if (!state.token) return '';
      const signRes = await apiFetch(`/api/v1/albums/${album.id}/cover-sign`, {
        method: 'POST',
        headers: headers()
      });
      if (!signRes.ok) return '';
      const sj = await signRes.json();
      if (!sj || !sj.url) return '';
      const url = `${sj.url}&v=${Date.now()}`;
      state.albumCoverURLs[album.id] = url;
      return url;
    }

    async function resolveTrackCoverURL(track) {
      const album = findAlbumForTrack(track);
      let url = coverURLForTrack(track);
      if (!url) {
        url = await ensureCoverURLForAlbum(album);
      }
      if (!url) {
        const albumID = Number(track.album_id || track.albumID || 0);
        if (albumID > 0) {
          if (state.token) {
            const signRes = await apiFetch(`/api/v1/albums/${albumID}/cover-sign`, {
              method: 'POST',
              headers: headers()
            });
            if (signRes.ok) {
              const sj = await signRes.json();
              if (sj && sj.url) url = `${sj.url}&v=${Date.now()}`;
            }
          }
          if (!url) {
            url = `/api/v1/albums/${albumID}/cover?v=${Date.now()}`;
          }
        }
      }
      if (!url && track && track.id) {
        try {
          const detailRes = await apiFetch(`/api/v1/tracks/${encodeURIComponent(track.id)}`, { headers: headers() }, false);
          if (detailRes.ok) {
            const detail = await detailRes.json();
            if (detail && typeof detail === 'object') {
              if (!track.album && detail.album) track.album = detail.album;
              if (!track.artist && detail.artist) track.artist = detail.artist;
              if (!track.uploader_name && detail.uploader_name) track.uploader_name = detail.uploader_name;
            }
            const detailAlbumID = Number(detail.album_id || 0);
            if (detailAlbumID > 0) {
              track.album_id = detailAlbumID;
            }
            if (!url) {
              const retryAlbum = findAlbumForTrack(track);
              if (retryAlbum) {
                url = state.albumCoverURLs[retryAlbum.id] || '';
                if (!url) {
                  url = await ensureCoverURLForAlbum(retryAlbum);
                }
              }
            }
            if (!url && detailAlbumID > 0) {
              if (state.token) {
                const signRes = await apiFetch(`/api/v1/albums/${detailAlbumID}/cover-sign`, {
                  method: 'POST',
                  headers: headers()
                });
                if (signRes.ok) {
                  const sj = await signRes.json();
                  if (sj && sj.url) url = `${sj.url}&v=${Date.now()}`;
                }
              }
              if (!url) {
                url = `/api/v1/albums/${detailAlbumID}/cover?v=${Date.now()}`;
              }
            }
          }
        } catch (_) {}
      }
      return url || '';
    }

    async function updateNowPlayingVisuals(track) {
      if (!track) {
        $('npTitle').textContent = 'Nothing playing';
        $('npSub').textContent = 'Select a track and hit play';
        state.currentTrackMeta = null;
        state.currentCoverURL = '';
        setPlayerThumb('');
        const engine = engineState(state.activeEngine || 'main');
        engine.currentLyrics = { track_id: '', plain: '', srt: '', cues: [] };
        syncLegacyPlayerMirror(state.activeEngine || 'main');
        return;
      }
      state.currentTrackMeta = {
        id: track.id,
        title: track.title || 'Untitled',
        artist: track.artist || '-',
        album: track.album || '-',
        album_id: Number(track.album_id || track.albumID || 0),
        uploader_name: track.uploader_name || track.owner_sub || '-'
      };
      $('npTitle').textContent = track.title || 'Untitled';
      $('npSub').textContent = `${track.artist || '-'} · ${track.album || '-'}`;
      const url = await resolveTrackCoverURL(track);
      state.currentCoverURL = url || '';
      setPlayerThumb(url);
      cachePlayerSnapshot();
    }

    async function loadTrackLyrics(trackID) {
      const engine = engineState(state.activeEngine || 'main');
      if (!trackID) {
        engine.currentLyrics = { track_id: '', plain: '', srt: '', cues: [] };
        syncLegacyPlayerMirror(state.activeEngine || 'main');
        return;
      }
      const res = await apiFetch(`/api/v1/tracks/${encodeURIComponent(trackID)}`, { headers: headers() }, false);
      if (!res.ok) {
        engine.currentLyrics = { track_id: trackID, plain: '', srt: '', cues: [] };
        syncLegacyPlayerMirror(state.activeEngine || 'main');
        return;
      }
      const json = await res.json();
      const srt = String(json.lyrics_srt || '').trim();
      engine.currentLyrics = {
        track_id: trackID,
        plain: String(json.lyrics_txt || ''),
        srt,
        cues: parseSRTLyrics(srt)
      };
      syncLegacyPlayerMirror(state.activeEngine || 'main');
    }

    function ensureAudioFX() {
      if (state.audioFX) return state.audioFX;
      const audioMain = $('audio');
      const audioA = $('audioJukeboxA');
      const audioB = $('audioJukeboxB');
      const Ctx = window.AudioContext || window.webkitAudioContext;
      if (!Ctx) return null;
      const ctx = new Ctx();
      const sourceMain = ctx.createMediaElementSource(audioMain);
      const sourceA = ctx.createMediaElementSource(audioA);
      const sourceB = ctx.createMediaElementSource(audioB);
      const gainMain = ctx.createGain();
      const gainA = ctx.createGain();
      const gainB = ctx.createGain();
      gainMain.gain.value = 1;
      gainA.gain.value = 1;
      gainB.gain.value = 0;
      const freqs = [60, 170, 350, 1000, 3500, 10000];
      const filters = freqs.map((f, idx) => {
        const biquad = ctx.createBiquadFilter();
        biquad.frequency.value = f;
        biquad.gain.value = 0;
        biquad.Q.value = 1.0;
        if (idx === 0) biquad.type = 'lowshelf';
        else if (idx === freqs.length - 1) biquad.type = 'highshelf';
        else biquad.type = 'peaking';
        return biquad;
      });
      sourceMain.connect(gainMain);
      sourceA.connect(gainA);
      sourceB.connect(gainB);
      gainMain.connect(filters[0]);
      gainA.connect(filters[0]);
      gainB.connect(filters[0]);
      for (let i = 0; i < filters.length - 1; i++) {
        filters[i].connect(filters[i + 1]);
      }
      const analyser = ctx.createAnalyser();
      analyser.fftSize = 256;
      analyser.smoothingTimeConstant = 0.82;
      filters[filters.length - 1].connect(analyser);
      analyser.connect(ctx.destination);
      state.audioFX = { ctx, sourceMain, sourceA, sourceB, gainMain, gainA, gainB, filters, analyser };
      return state.audioFX;
    }

    async function resumeAudioContextIfNeeded() {
      const fx = ensureAudioFX();
      if (!fx) return;
      if (fx.ctx.state === 'suspended') {
        try {
          await fx.ctx.resume();
        } catch (_) {}
      }
    }

    function eqPresetGains(name) {
      const presets = {
        flat: [0, 0, 0, 0, 0, 0],
        rock: [4, 2, -1, -1, 2, 4],
        pop: [-1, 2, 4, 3, 1, -1],
        jazz: [3, 1, -1, 1, 2, 3],
        classical: [3, 2, 0, -1, 2, 3],
        electronic: [4, 2, 0, -2, 2, 4],
        bass_boost: [6, 4, 2, 0, -1, -2],
        vocal: [-2, -1, 2, 4, 4, 2]
      };
      return presets[name] || presets.flat;
    }

    function applyEQPreset(name) {
      const fx = ensureAudioFX();
      if (!fx) return;
      const gains = eqPresetGains(name);
      fx.filters.forEach((f, idx) => {
        f.gain.value = gains[idx] || 0;
      });
    }


    async function signStream(trackId, format = 'mp3') {
      const res = await apiFetch('/api/v1/streams/sign', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ track_id: trackId, format })
      });
      if (!res.ok) return null;
      return await res.json();
    }

    async function ensureSignedStream(trackId, format = 'mp3') {
      const key = `${trackId}:${format}`;
      const cached = state.streamSignCache[key];
      const now = Math.floor(Date.now() / 1000);
      if (cached && Number(cached.expires_unix || 0) > (now + 8)) {
        return cached;
      }
      const signed = await signStream(trackId, format);
      if (signed) state.streamSignCache[key] = signed;
      return signed;
    }

    async function primeNextSignedStream(queue, index) {
      const next = Array.isArray(queue) ? queue[index + 1] : null;
      if (!next || !next.id) return;
      await ensureSignedStream(next.id, 'mp3');
    }

    async function maybeScheduleJukeboxCrossfade() {
      if (!isJukeboxPlayback()) return;
      if (state.jukeboxPlayer.crossfading) return;
      const audio = activeAudio();
      if (!audio || !audio.src || audio.paused) return;
      const idx = Number(state.jukeboxPlayer.queueIndex || 0);
      const nextIndex = idx + 1;
      const nextTrack = state.jukeboxPlayer.queue[nextIndex];
      if (!nextTrack || !nextTrack.id) return;
      const duration = Number(audio.duration || 0);
      const currentTime = Number(audio.currentTime || 0);
      if (!Number.isFinite(duration) || duration <= 0) return;
      const remaining = duration - currentTime;
      const crossfadeSeconds = Math.max(2, Number(state.jukeboxPlayer.crossfadeSeconds || 5));
      if (remaining > crossfadeSeconds) return;
      const trackKey = `${state.jukeboxPlayer.nowPlayingTrackId}:${nextTrack.id}`;
      if (state.jukeboxPlayer.crossfadeTrackId === trackKey) return;
      state.jukeboxPlayer.crossfadeTrackId = trackKey;
      state.jukeboxPlayer.crossfadeTargetIndex = nextIndex;
      await startJukeboxCrossfade(nextIndex);
    }

    async function startJukeboxCrossfade(nextIndex) {
      if (state.jukeboxPlayer.crossfading) return;
      const current = jukeboxActiveAudio();
      const standby = jukeboxStandbyAudio();
      const nextTrack = state.jukeboxPlayer.queue[nextIndex];
      if (!current || !standby || !nextTrack) return;
      const signed = await ensureSignedStream(nextTrack.id, 'mp3');
      if (!signed) return;
      state.jukeboxPlayer.crossfading = true;
      standby.pause();
      standby.src = `/api/v1/stream/${nextTrack.id}?format=mp3&token=${encodeURIComponent(signed.token)}&expires=${signed.expires_unix}`;
      standby.preload = 'auto';
      standby.currentTime = 0;
      standby.volume = current.volume;
      standby.muted = current.muted;
      await resumeAudioContextIfNeeded();
      const fx = ensureAudioFX();
      if (fx && fx.gainA && fx.gainB) {
        const activeGain = state.jukeboxPlayer.activeAudioId === 'audioJukeboxA' ? fx.gainA : fx.gainB;
        const standbyGain = state.jukeboxPlayer.activeAudioId === 'audioJukeboxA' ? fx.gainB : fx.gainA;
        activeGain.gain.value = current.muted ? 0 : 1;
        standbyGain.gain.value = 0;
      }
      try {
        await standby.play();
      } catch (_) {
        stopJukeboxCrossfade();
        return;
      }
      const fadeMs = Math.max(2000, Number(state.jukeboxPlayer.crossfadeSeconds || 5) * 1000);
      const startedAt = Date.now();
      const previousTrack = state.jukeboxPlayer.queue[state.jukeboxPlayer.queueIndex] || null;
      const sourceContext = 'jukebox';
      state.jukeboxPlayer.crossfadeTimer = window.setInterval(async () => {
        const elapsed = Date.now() - startedAt;
        const ratio = Math.max(0, Math.min(1, elapsed / fadeMs));
        if (fx && fx.gainA && fx.gainB) {
          const activeGain = state.jukeboxPlayer.activeAudioId === 'audioJukeboxA' ? fx.gainA : fx.gainB;
          const standbyGain = state.jukeboxPlayer.activeAudioId === 'audioJukeboxA' ? fx.gainB : fx.gainA;
          activeGain.gain.value = current.muted ? 0 : (1 - ratio);
          standbyGain.gain.value = standby.muted ? 0 : ratio;
        } else {
          current.volume = Math.max(0, (1 - ratio) * (standby.volume || 0.7));
          standby.volume = Math.max(0, ratio * (standby.volume || 0.7));
        }
        if (ratio < 1) return;
        stopJukeboxCrossfade();
        current.pause();
        current.src = '';
        swapJukeboxDecks();
        syncJukeboxDeckVolumes();
        state.jukeboxPlayer.queueIndex = nextIndex;
        state.jukeboxPlayer.nowPlayingTrackId = normTrackID(nextTrack.id);
        syncLegacyPlayerMirror('jukebox');
        if (state.jukebox && state.jukebox.session_id) {
          state.jukebox.current_position = Math.max(0, nextIndex);
        }
        if (previousTrack && state.listeningSession && state.listeningSession.track_id === previousTrack.id) {
          finalizeListeningSession('natural_end');
        }
        beginListeningSession(nextTrack, sourceContext);
        await loadTrackLyrics(nextTrack.id);
        state.jukeboxCurrentCoverURL = await resolveTrackCoverURL(nextTrack);
        if (state.currentView === 'jukebox' && typeof ns.renderJukeboxNowPlaying === 'function') {
          ns.renderJukeboxNowPlaying();
          if (typeof ns.renderJukeboxQueue === 'function') ns.renderJukeboxQueue();
        }
        primeNextSignedStream(state.jukeboxPlayer.queue, state.jukeboxPlayer.queueIndex).catch(() => {});
        cacheJukeboxSnapshot();
        emitJukeboxState(true);
        if ((state.jukeboxPlayer.queue.length - nextIndex) <= 3 && typeof ns.refreshJukeboxQueue === 'function') {
          ns.refreshJukeboxQueue(nextIndex).catch(() => {});
        }
      }, 100);
    }

    async function startTrackById(trackId, sourceList = state.filteredTracks, sourceContext = '') {
      const targetID = normTrackID(trackId);
      const t = sourceList.find((x) => normTrackID(x?.id) === targetID);
      if (!t) return;
      const isJukeboxQueue = !!(
        state.jukebox &&
        state.jukebox.session_id &&
        Array.isArray(state.jukebox.queue) &&
        state.jukebox.queue.length === sourceList.length &&
        state.jukebox.queue.every((item, idx) => normTrackID(item?.id) === normTrackID(sourceList[idx]?.id))
      );
      if (!sourceContext && isJukeboxQueue) {
        sourceContext = `jukebox:${state.jukebox.mode || 'for_you'}`;
      }
      try {
        sourceContextForPlayback(sourceContext);
      } catch (_) {}
      stopJukeboxPlayback();
      stopMainPlayback();
      if (state.listeningSession && state.listeningSession.track_id !== trackId) {
        finalizeListeningSession('switch');
      }
      const signed = await ensureSignedStream(trackId, 'mp3');
      if (!signed) {
        alert('Play requires login and permission for this track.');
        return;
      }

      state.mainPlayer.queue = sourceList.slice();
      state.mainPlayer.queueIndex = sourceList.findIndex((x) => normTrackID(x?.id) === targetID);
      if (state.mainPlayer.queueIndex < 0 && state.selectedAlbum) {
        const albumQueue = getSelectedAlbumTracks();
        const albumIdx = albumQueue.findIndex((x) => normTrackID(x?.id) === targetID);
        if (albumIdx >= 0) {
          state.mainPlayer.queue = albumQueue;
          state.mainPlayer.queueIndex = albumIdx;
        }
      }
      state.mainPlayer.nowPlayingTrackId = targetID;
      state.currentTrackMeta = {
        id: t.id,
        title: t.title || 'Untitled',
        artist: t.artist || '-',
        album: t.album || '-',
        album_id: Number(t.album_id || t.albumID || 0),
        uploader_name: t.uploader_name || t.owner_sub || '-'
      };
      syncLegacyPlayerMirror('main');
      state.activeEngine = 'main';
      cachePlayerSnapshot();
      emitPlayerState(true);
      const audio = $('audio');
      audio.src = `/api/v1/stream/${targetID}?format=mp3&token=${encodeURIComponent(signed.token)}&expires=${signed.expires_unix}`;
      audio.preload = 'auto';
      audio.currentTime = 0;
      audio.muted = false;
      audio.volume = Number($('volumeRange')?.value || 70) / 100;
      try { audio.load(); } catch (_) {}
      await resumeAudioContextIfNeeded();
      try {
        await audio.play();
      } catch (err) {
        console.warn('manual main player audio.play() failed', err);
        alert('Manual playback could not be started.');
        return;
      }
      beginListeningSession(t, sourceContext);
      try {
        await loadTrackLyrics(targetID);
      } catch (err) {
        console.warn('loadTrackLyrics failed for main player track', err);
      }

      updatePlayerButtons();
      try {
        await updateNowPlayingVisuals(t);
      } catch (err) {
        console.warn('updateNowPlayingVisuals failed for main player track', err);
      }
      cachePlayerSnapshot();
      emitPlayerState(true);
      try {
        if (state.selectedPlaylistTracks.length) {
          renderPlaylistTracks();
        } else {
          renderPlaylistDock();
        }
      } catch (err) {
        console.warn('playlist render failed after main player start', err);
      }
      showPlayer();
      primeNextSignedStream(state.mainPlayer.queue, state.mainPlayer.queueIndex).catch(() => {});
      closePanels();
    }

    async function startJukeboxTrackById(trackId, sourceList = [], sourceContext = 'jukebox') {
      const targetID = normTrackID(trackId);
      const t = sourceList.find((x) => normTrackID(x?.id) === targetID);
      if (!t) return;
      stopMainPlayback();
      if (state.listeningSession && state.listeningSession.track_id !== trackId) {
        finalizeListeningSession('switch');
      }
      const signed = await ensureSignedStream(targetID, 'mp3');
      if (!signed) {
        alert('Play requires login and permission for this track.');
        return;
      }
      state.jukeboxPlayer.queue = sourceList.slice();
      state.jukeboxPlayer.queueIndex = sourceList.findIndex((x) => normTrackID(x?.id) === targetID);
      state.jukeboxPlayer.nowPlayingTrackId = targetID;
      syncLegacyPlayerMirror('jukebox');
      const audio = jukeboxActiveAudio();
      const standby = jukeboxStandbyAudio();
      standby.pause();
      standby.src = '';
      audio.src = `/api/v1/stream/${targetID}?format=mp3&token=${encodeURIComponent(signed.token)}&expires=${signed.expires_unix}`;
      audio.preload = 'auto';
      audio.volume = Number($('volumeRange')?.value || 70) / 100;
      await resumeAudioContextIfNeeded();
      await audio.play();
      beginListeningSession(t, sourceContext);
      try {
        await loadTrackLyrics(targetID);
      } catch (err) {
        console.warn('loadTrackLyrics failed for jukebox track', err);
      }
      try {
        state.jukeboxCurrentCoverURL = await resolveTrackCoverURL(t);
      } catch (err) {
        console.warn('resolveTrackCoverURL failed for jukebox track', err);
      }
      if (state.currentView === 'jukebox' && typeof ns.renderJukeboxNowPlaying === 'function') {
        ns.renderJukeboxNowPlaying();
        if (typeof ns.renderJukeboxQueue === 'function') ns.renderJukeboxQueue();
      }
      primeNextSignedStream(state.jukeboxPlayer.queue, state.jukeboxPlayer.queueIndex).catch(() => {});
      cacheJukeboxSnapshot();
      emitJukeboxState(true);
    }

    async function playAlbum(shuffle = false) {
      let tracks = [];
      if (state.selectedAlbum) {
        tracks = state.tracks.filter((t) => {
          if (normText(t.album) !== normText(state.selectedAlbum.title)) return false;
          if (!state.selectedAlbum.artist || !t.artist) return true;
          return normText(t.artist) === normText(state.selectedAlbum.artist);
        });
        tracks.sort((a, b) => {
          const an = Number(a.track_number || 0);
          const bn = Number(b.track_number || 0);
          if (an > 0 && bn > 0 && an !== bn) return an - bn;
          if (an > 0 && bn <= 0) return -1;
          if (bn > 0 && an <= 0) return 1;
          return String(a.title || '').localeCompare(String(b.title || ''));
        });
      } else {
        tracks = state.filteredTracks.slice();
      }
      if (!tracks.length) return;
      if (shuffle) tracks = tracks.sort(() => Math.random() - 0.5);
      await startTrackById(tracks[0].id, tracks);
    }

    function setManualQueueOpen(open) {
      state.manualQueueOpen = !!open;
      localStorage.setItem('hex_manual_queue_open', state.manualQueueOpen ? '1' : '0');
      const dock = $('manualQueueDock');
      const toggle = $('btnManualQueueToggle');
      const inline = $('btnManualQueue');
      if (!dock || !toggle) return;
      dock.classList.toggle('open', state.manualQueueOpen);
      dock.classList.toggle('hidden', !state.manualQueueOpen);
      toggle.classList.toggle('hidden', !state.mainPlayer.queue.length);
      toggle.classList.toggle('active', state.manualQueueOpen);
      if (inline) inline.classList.toggle('active', state.manualQueueOpen);
      syncPlayerSideDocks();
    }

    function renderManualQueueDock() {
      const body = $('manualQueueBody');
      const meta = $('manualQueueMeta');
      const toggle = $('btnManualQueueToggle');
      const inline = $('btnManualQueue');
      if (!body || !meta || !toggle) return;
      const queue = Array.isArray(state.mainPlayer.queue) ? state.mainPlayer.queue : [];
      const idx = Number(state.mainPlayer.queueIndex || -1);
      const hasQueue = queue.length > 0;
      toggle.classList.toggle('hidden', !hasQueue);
      if (inline) inline.disabled = !hasQueue;
      meta.textContent = hasQueue ? `${queue.length} queued tracks · current ${idx >= 0 ? idx + 1 : '-'}/${queue.length}` : 'No manual queue active.';
      body.innerHTML = '';
      if (!hasQueue) {
        body.innerHTML = '<div class="muted">Start a manual track or album to build a queue.</div>';
        setManualQueueOpen(false);
        return;
      }
      queue.forEach((track, queueIndex) => {
        const item = document.createElement('div');
        item.className = `manual-queue-item ${queueIndex === idx ? 'active' : ''}`.trim();
        item.innerHTML = `
          <div class="manual-queue-index">${queueIndex === idx ? '&gt;' : queueIndex + 1}</div>
          <div class="manual-queue-meta">
            <div class="manual-queue-title">${escapeHtml(track.title || '-')}</div>
            <div class="manual-queue-sub">${escapeHtml(track.artist || '-')} · ${escapeHtml(track.album || '-')}</div>
          </div>
          <div class="manual-queue-actions">
            <button class="btn slim" data-main-queue-jump="${queueIndex}">Play</button>
            <button class="btn slim danger" data-main-queue-remove="${queueIndex}">Remove</button>
          </div>
        `;
        const jumpBtn = item.querySelector('[data-main-queue-jump]');
        if (jumpBtn) jumpBtn.onclick = async () => applyPlayerControl('jump_queue_index', queueIndex);
        const removeBtn = item.querySelector('[data-main-queue-remove]');
        if (removeBtn) removeBtn.onclick = () => removeMainQueueIndex(queueIndex);
        body.appendChild(item);
      });
      syncPlayerSideDocks();
    }

    function syncPlayerSideDocks() {
      const queueDock = $('manualQueueDock');
      const playlistDock = $('playlistDock');
      if (!queueDock || !playlistDock) return;
      const queueOpen = !!state.manualQueueOpen && !queueDock.classList.contains('hidden');
      const playlistOpen = !!state.playlistDockOpen && !playlistDock.classList.contains('hidden');
      queueDock.classList.toggle('is-paired-left', queueOpen && playlistOpen);
      playlistDock.classList.toggle('is-paired-right', queueOpen && playlistOpen);
    }

    function removeMainQueueIndex(index) {
      const engine = state.mainPlayer;
      const idx = Number(index);
      if (!Number.isFinite(idx) || idx < 0 || idx >= engine.queue.length) return;
      engine.queue.splice(idx, 1);
      if (!engine.queue.length) {
        engine.queueIndex = -1;
        engine.nowPlayingTrackId = '';
        syncLegacyPlayerMirror('main');
        emitPlayerState(true);
        return;
      }
      if (idx < engine.queueIndex) {
        engine.queueIndex -= 1;
      } else if (idx === engine.queueIndex) {
        engine.queueIndex = Math.min(engine.queueIndex, engine.queue.length - 1);
      }
      syncLegacyPlayerMirror('main');
      emitPlayerState(true);
    }

    function clearMainQueue() {
      state.mainPlayer.queue = [];
      state.mainPlayer.queueIndex = -1;
      state.mainPlayer.nowPlayingTrackId = '';
      syncLegacyPlayerMirror('main');
      emitPlayerState(true);
    }

    function insertQueueAfterCurrent(tracks) {
      if (!tracks.length) return;
      const engine = state.mainPlayer;
      if (!engine.queue.length || engine.queueIndex < 0) {
        engine.queue = tracks.slice();
        engine.queueIndex = -1;
        if ((state.activeEngine || 'main') === 'main') syncLegacyPlayerMirror('main');
        emitPlayerState(true);
        return;
      }
      const left = engine.queue.slice(0, engine.queueIndex + 1);
      const right = engine.queue.slice(engine.queueIndex + 1);
      engine.queue = left.concat(tracks).concat(right);
      if ((state.activeEngine || 'main') === 'main') syncLegacyPlayerMirror('main');
      emitPlayerState(true);
    }

    function appendToQueue(tracks) {
      if (!tracks.length) return;
      const engine = state.mainPlayer;
      if (!engine.queue.length) {
        engine.queue = tracks.slice();
        engine.queueIndex = -1;
        if ((state.activeEngine || 'main') === 'main') syncLegacyPlayerMirror('main');
        emitPlayerState(true);
        return;
      }
      engine.queue = engine.queue.concat(tracks);
      if ((state.activeEngine || 'main') === 'main') syncLegacyPlayerMirror('main');
      emitPlayerState(true);
    }

Object.assign(window.HexSonic, {
      isPopoutPlayerMode,
      isJukeboxPopoutMode,
      applyPopoutPlayerMode,
      parseSRTTimeToMs,
      parseSRTLyrics,
      currentTrackFromQueue,
      normTrackID,
      currentQueueTrackID,
      isActiveTrack,
      visualizerBins,
      buildPlayerSnapshot,
      emitPlayerState,
      buildPlayerTick,
      emitPlayerTick,
      startBridgeTicker,
      applyPlayerControl,
      openPopoutPlayer,
      lyricActiveIndex,
      drawCanvasLyrics,
      preparePopVizCanvas,
      renderPopoutVisualizer,
      renderPopoutQueue,
      renderPopoutSnapshot,
      applyPopoutTick,
      initPlayerBridge,
      initJukeboxBridge,
      bindPopoutEvents,
      bindJukeboxPopoutEvents,
      closePanels,
      showPlayer,
      hidePlayer,
      openAlbumFromPlayerThumb,
      updatePlayerButtons,
      setPlayerThumb,
      findAlbumForTrack,
      coverURLForTrack,
      ensureCoverURLForAlbum,
      updateNowPlayingVisuals,
      loadTrackLyrics,
      ensureAudioFX,
      resumeAudioContextIfNeeded,
      eqPresetGains,
      applyEQPreset,
      resolveTrackCoverURL,
      signStream,
      ensureSignedStream,
      primeNextSignedStream,
      activeAudio,
      standbyAudio,
      pickMainAdjacentIndex,
      jukeboxActiveAudio,
      emitJukeboxState,
      emitJukeboxTick,
      openJukeboxPopout,
      maybeScheduleJukeboxCrossfade,
      startTrackById,
      startJukeboxTrackById,
      playAlbum,
      setManualQueueOpen,
      renderManualQueueDock,
      syncPlayerSideDocks,
      removeMainQueueIndex,
      clearMainQueue,
      insertQueueAfterCurrent,
      appendToQueue
});
})(window.HexSonic = window.HexSonic || {});
