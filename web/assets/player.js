(function(ns) {
const { state, $, ICON_PLAY, ICON_PAUSE, ICON_MUTE, ICON_UNMUTE, ICON_DETAIL, ICON_PLAYLIST, PLAYER_BRIDGE_CHANNEL, PLAYER_TICK_MS, headers, apiFetch, fmt } = ns;
const finalizeListeningSession = (...args) => ns.finalizeListeningSession(...args);
const beginListeningSession = (...args) => ns.beginListeningSession(...args);
const renderPlaylistTracks = (...args) => ns.renderPlaylistTracks(...args);
const renderPlaylistDock = (...args) => ns.renderPlaylistDock(...args);
const selectAlbum = (...args) => ns.selectAlbum(...args);
const sourceContextForPlayback = (...args) => ns.sourceContextForPlayback(...args);

    function isPopoutPlayerMode() {
      const u = new URL(window.location.href);
      return (u.searchParams.get('popout_player') || '') === '1' || String(window.location.hash || '').replace(/^#/, '') === 'player_popout';
    }

    function applyPopoutPlayerMode() {
      document.body.classList.toggle('popout-player', !!state.popoutMode);
      $('popoutPlayer').classList.toggle('hidden', !state.popoutMode);
      if (state.popoutMode) {
        document.title = 'HEXSONIC Popout Player';
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
      if (state.queueIndex < 0 || !state.queue[state.queueIndex]) return null;
      return state.queue[state.queueIndex];
    }

    function normTrackID(v) {
      if (v === null || v === undefined) return '';
      return String(v).trim();
    }

    function currentQueueTrackID() {
      const track = currentTrackFromQueue();
      return track ? normTrackID(track.id) : '';
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
      const audio = $('audio');
      const track = currentTrackFromQueue();
      return {
        at: Date.now(),
        track: track ? {
          id: track.id,
          title: track.title || 'Untitled',
          artist: track.artist || '-',
          album: track.album || '-',
          uploader_name: track.uploader_name || track.owner_sub || '-'
        } : null,
        queue: state.queue.map((q) => ({
          id: q.id,
          title: q.title || 'Untitled',
          artist: q.artist || '-',
          album: q.album || '-'
        })),
        queue_index: state.queueIndex,
        paused: audio.paused,
        muted: audio.muted,
        volume: Math.round((audio.volume || 0) * 100),
        current_time: Number(audio.currentTime || 0),
        duration: Number(audio.duration || 0),
        cover_url: track ? (coverURLForTrack(track) || '') : '',
        eq: $('eqPreset').value || 'flat',
        lyrics: {
          track_id: state.currentLyrics.track_id || '',
          plain: state.currentLyrics.plain || '',
          cues: Array.isArray(state.currentLyrics.cues) ? state.currentLyrics.cues : []
        },
        bins: visualizerBins()
      };
    }

    function emitPlayerState(force = false) {
      if (!state.playerBridge) return;
      const now = Date.now();
      if (!force && now - state.lastBridgeStateAt < 120) return;
      state.lastBridgeStateAt = now;
      state.playerBridge.postMessage({ type: 'player_state', payload: buildPlayerSnapshot() });
    }

    function buildPlayerTick() {
      const audio = $('audio');
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

    function emitPlayerTick() {
      if (!state.playerBridge || state.popoutMode) return;
      state.playerBridge.postMessage({ type: 'player_tick', payload: buildPlayerTick() });
    }

    function startBridgeTicker() {
      if (state.bridgeTickTimer || state.popoutMode) return;
      state.bridgeTickTimer = window.setInterval(() => {
        const a = $('audio');
        if (!a || !a.src) return;
        emitPlayerTick();
      }, PLAYER_TICK_MS);
    }

    async function applyPlayerControl(action, value) {
      const audio = $('audio');
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
        if (!state.queue.length) return;
        state.queueIndex = (state.queueIndex - 1 + state.queue.length) % state.queue.length;
        await startTrackById(state.queue[state.queueIndex].id, state.queue);
        return;
      }
      if (action === 'next') {
        if (!state.queue.length) return;
        state.queueIndex = (state.queueIndex + 1) % state.queue.length;
        await startTrackById(state.queue[state.queueIndex].id, state.queue);
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
        if (!Number.isFinite(idx) || idx < 0 || idx >= state.queue.length) return;
        state.queueIndex = idx;
        await startTrackById(state.queue[state.queueIndex].id, state.queue);
      }
    }

    function openPopoutPlayer() {
      const u = new URL(window.location.href);
      u.searchParams.set('popout_player', '1');
      u.hash = 'player_popout';
      const w = window.open(u.toString(), 'hexsonic_popout_player', 'width=1280,height=880,resizable=yes,scrollbars=no');
      if (!w) {
        alert('Popup blocked. Please allow popups for this site.');
        return;
      }
      state.popoutWindow = w;
      emitPlayerState(true);
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
            <div class="pop-q-title">${esc(item.title || '-')}</div>
            <div class="pop-q-meta">${esc(item.artist || '-')} · ${esc(item.album || '-')}</div>
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
      renderPopoutVisualizer(Array.isArray(t.bins) ? t.bins : [], state.popoutSnapshot || {}, state.popoutSnapshot.current_time);
    }

    function initPlayerBridge() {
      if (state.playerBridge) return;
      if (typeof BroadcastChannel !== 'function') return;
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
        state.playerBridge.postMessage({ type: 'player_ready' });
      } else {
        state.playerBridge.postMessage({ type: 'player_ping' });
      }
    }

    function bindPopoutEvents() {
      $('popClose').onclick = () => window.close();
      $('popPlayPause').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'play_pause' });
      $('popPrev').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'prev' });
      $('popNext').onclick = () => state.playerBridge && state.playerBridge.postMessage({ type: 'player_control', action: 'next' });
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
      const a = $('audio');
      const volumePct = Math.round(((a.volume || 0) * 100));
      $('btnPlayPause').textContent = a.paused ? ICON_PLAY : ICON_PAUSE;
      $('btnMute').textContent = a.muted ? ICON_MUTE : ICON_UNMUTE;
      $('btnMute').classList.toggle('is-muted', !!a.muted);
      $('btnMute').title = a.muted ? 'Unmute audio' : 'Mute audio';
      $('btnMute').setAttribute('aria-label', a.muted ? 'Unmute audio' : 'Mute audio');
      $('volumeValueLabel').textContent = a.muted ? `Muted ${volumePct}%` : `${volumePct}%`;
    }

    function setPlayerThumb(url = '') {
      const thumb = document.querySelector('.thumb');
      if (!thumb) return;
      if (url) {
        thumb.style.backgroundImage = `linear-gradient(180deg, rgba(0,0,0,0.1), rgba(0,0,0,0.38)), url('${url}')`;
      } else {
        thumb.style.backgroundImage = 'linear-gradient(135deg, #25344d, #5672a3)';
      }
    }


    function findAlbumForTrack(track) {
      if (!track) return null;
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

    async function updateNowPlayingVisuals(track) {
      if (!track) {
        $('npTitle').textContent = 'Nothing playing';
        $('npSub').textContent = 'Select a track and hit play';
        setPlayerThumb('');
        state.currentLyrics = { track_id: '', plain: '', srt: '', cues: [] };
        return;
      }
      $('npTitle').textContent = track.title || 'Untitled';
      $('npSub').textContent = `${track.artist || '-'} · ${track.album || '-'}`;
      const album = findAlbumForTrack(track);
      let url = coverURLForTrack(track);
      if (!url) {
        url = await ensureCoverURLForAlbum(album);
      }
      setPlayerThumb(url);
    }

    async function loadTrackLyrics(trackID) {
      if (!trackID) {
        state.currentLyrics = { track_id: '', plain: '', srt: '', cues: [] };
        return;
      }
      const res = await apiFetch(`/api/v1/tracks/${encodeURIComponent(trackID)}`, { headers: headers() }, false);
      if (!res.ok) {
        state.currentLyrics = { track_id: trackID, plain: '', srt: '', cues: [] };
        return;
      }
      const json = await res.json();
      const srt = String(json.lyrics_srt || '').trim();
      state.currentLyrics = {
        track_id: trackID,
        plain: String(json.lyrics_txt || ''),
        srt,
        cues: parseSRTLyrics(srt)
      };
    }

    function ensureAudioFX() {
      if (state.audioFX) return state.audioFX;
      const audio = $('audio');
      const Ctx = window.AudioContext || window.webkitAudioContext;
      if (!Ctx) return null;
      const ctx = new Ctx();
      const source = ctx.createMediaElementSource(audio);
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
      source.connect(filters[0]);
      for (let i = 0; i < filters.length - 1; i++) {
        filters[i].connect(filters[i + 1]);
      }
      const analyser = ctx.createAnalyser();
      analyser.fftSize = 256;
      analyser.smoothingTimeConstant = 0.82;
      filters[filters.length - 1].connect(analyser);
      analyser.connect(ctx.destination);
      state.audioFX = { ctx, source, filters, analyser };
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

    async function startTrackById(trackId, sourceList = state.filteredTracks, sourceContext = '') {
      const t = sourceList.find((x) => x.id === trackId);
      if (!t) return;
      if (state.listeningSession && state.listeningSession.track_id !== trackId) {
        finalizeListeningSession('switch');
      }
      const signed = await signStream(trackId, 'mp3');
      if (!signed) {
        alert('Play requires login and permission for this track.');
        return;
      }

      state.queue = sourceList;
      state.queueIndex = sourceList.findIndex((x) => x.id === trackId);
      state.nowPlayingTrackId = normTrackID(trackId);

      const audio = $('audio');
      audio.src = `/api/v1/stream/${trackId}?format=mp3&token=${encodeURIComponent(signed.token)}&expires=${signed.expires_unix}`;
      await resumeAudioContextIfNeeded();
      await audio.play();
      beginListeningSession(t, sourceContext);
      await loadTrackLyrics(trackId);

      updatePlayerButtons();
      await updateNowPlayingVisuals(t);
      if (state.selectedPlaylistTracks.length) {
        renderPlaylistTracks();
      } else {
        renderPlaylistDock();
      }
      showPlayer();
      closePanels();
      emitPlayerState(true);
    }

    async function playAlbum(shuffle = false) {
      let tracks = state.selectedAlbum ? getSelectedAlbumTracks() : state.filteredTracks.slice();
      if (!tracks.length) return;
      if (shuffle) tracks = tracks.sort(() => Math.random() - 0.5);
      await startTrackById(tracks[0].id, tracks);
    }

    function insertQueueAfterCurrent(tracks) {
      if (!tracks.length) return;
      if (!state.queue.length || state.queueIndex < 0) {
        state.queue = tracks.slice();
        state.queueIndex = -1;
        emitPlayerState(true);
        return;
      }
      const left = state.queue.slice(0, state.queueIndex + 1);
      const right = state.queue.slice(state.queueIndex + 1);
      state.queue = left.concat(tracks).concat(right);
      emitPlayerState(true);
    }

    function appendToQueue(tracks) {
      if (!tracks.length) return;
      if (!state.queue.length) {
        state.queue = tracks.slice();
        state.queueIndex = -1;
        emitPlayerState(true);
        return;
      }
      state.queue = state.queue.concat(tracks);
      emitPlayerState(true);
    }

Object.assign(window.HexSonic, {
      isPopoutPlayerMode,
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
      bindPopoutEvents,
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
      signStream,
      startTrackById,
      playAlbum,
      insertQueueAfterCurrent,
      appendToQueue
});
})(window.HexSonic = window.HexSonic || {});
