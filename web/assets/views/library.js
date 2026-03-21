(function(ns) {
const { state, $, escapeHtml, headers, apiFetch, fmt, normText, saveSelectedAlbumID, loadSelectedAlbumID, ICON_PLAY, ICON_DETAIL, ICON_PLAYLIST } = ns;
const canUpload = (...args) => ns.canUpload(...args);
const canAdmin = (...args) => ns.canAdmin(...args);
const canManageTrack = (...args) => ns.canManageTrack(...args);
const openUserProfile = (...args) => ns.openUserProfile(...args);
const addTrackToPlaylist = (...args) => ns.addTrackToPlaylist(...args);
const loadTrackForEditor = (...args) => ns.loadTrackForEditor(...args);
const setAdminTarget = (...args) => ns.setAdminTarget(...args);
const switchView = (...args) => ns.switchView(...args);
const renderAdminUsers = (...args) => ns.renderAdminUsers(...args);
const loadJobs = (...args) => ns.loadJobs(...args);
const updateNowPlayingVisuals = (...args) => ns.updateNowPlayingVisuals(...args);
const emitPlayerState = (...args) => ns.emitPlayerState(...args);
const loadDiscovery = (...args) => ns.loadDiscovery(...args);
const loadManageData = (...args) => ns.loadManageData(...args);
const loadPlaylists = (...args) => ns.loadPlaylists(...args);
const loadFavorites = (...args) => ns.loadFavorites(...args);
const renderFavorites = (...args) => ns.renderFavorites(...args);
const loadNotifications = (...args) => ns.loadNotifications(...args);
const loadMyProfile = (...args) => ns.loadMyProfile(...args);
const loadCreatorHighscore = (...args) => ns.loadCreatorHighscore(...args);
const loadCreatorStats = (...args) => ns.loadCreatorStats(...args);
const loadAdminUsers = (...args) => ns.loadAdminUsers(...args);
const loadAdminSystemOverview = (...args) => ns.loadAdminSystemOverview(...args);
const loadAdminDebugToggle = (...args) => ns.loadAdminDebugToggle(...args);
const loadAdminAuditLogs = (...args) => ns.loadAdminAuditLogs(...args);
const updateProfileDirtyState = (...args) => ns.updateProfileDirtyState(...args);
const renderAlbumDetail = (...args) => ns.renderAlbumDetail(...args);
const isFavorite = (...args) => ns.isFavorite(...args);
const toggleFavorite = (...args) => ns.toggleFavorite(...args);
const canCreatePlaylists = (...args) => ns.canCreatePlaylists(...args);
const startTrackById = (...args) => ns.startTrackById(...args);

    function clearAlbumCoverURLs() {
      state.albumCoverURLs = {};
    }

    async function preloadAlbumCovers() {
      const candidates = state.albums.filter((a) => a.cover_path);
      await Promise.all(candidates.map(async (a) => {
        if (a.visibility === 'public') {
          state.albumCoverURLs[a.id] = `/api/v1/albums/${a.id}/cover?v=${Date.now()}`;
          return;
        }
        if (!state.token) return;
        const signRes = await apiFetch(`/api/v1/albums/${a.id}/cover-sign`, {
          method: 'POST',
          headers: headers()
        });
        if (!signRes.ok) return;
        const sj = await signRes.json();
        if (sj && sj.url) {
          state.albumCoverURLs[a.id] = `${sj.url}&v=${Date.now()}`;
        }
      }));
    }

    function normalizeTrack(t) {
      return {
        id: t.id,
        title: t.title || 'Untitled',
        artist: t.artist || '-',
        album: t.album || '-',
        album_id: Number(t.album_id || 0),
        genre: t.genre || '',
        track_number: Number(t.track_number || 0),
        rating: Number(t.rating || t.average_rating || 0),
        visibility: t.visibility || '-',
        duration: Math.round(t.duration_seconds || 0),
        owner_sub: t.owner_sub || '',
        uploader_name: t.uploader_name || t.owner_sub || '',
        has_lyrics_txt: !!t.has_lyrics_txt,
        has_lyrics_srt: !!t.has_lyrics_srt
      };
    }

    function applyFilters() {
      const q = $('searchInput').value.trim().toLowerCase();
      const vis = $('filterVisibility').value;
      const albumGenreFilter = state.albumGenreFilter || 'all';
      const trackGenreFilter = state.trackGenreFilter || 'all';

      state.filteredTracks = state.tracks.filter((t) => {
        const okText = !q || `${t.title} ${t.artist} ${t.album} ${t.genre || ''} ${t.uploader_name || ''}`.toLowerCase().includes(q);
        const okVis = vis === 'all' || t.visibility === vis;
        const okGenre = trackGenreFilter === 'all' || (t.genre || '') === trackGenreFilter;
        return okText && okVis && okGenre;
      });

      state.filteredAlbums = state.albums.filter((a) => {
        const okText = !q || `${a.title} ${a.artist} ${a.genre || ''} ${a.uploader_name || ''}`.toLowerCase().includes(q);
        const okVis = vis === 'all' || a.visibility === vis;
        const okGenre = albumGenreFilter === 'all' || (a.genre || '') === albumGenreFilter;
        return okText && okVis && okGenre;
      });
    }

    function genreOptionsForView(view) {
      if (view === 'albums') {
        return Array.from(new Set((state.albums || []).map((a) => (a.genre || '').trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
      }
      if (view === 'tracks') {
        const pool = state.currentView === 'tracks' && state.selectedAlbum && Number(state.selectedAlbum.id) === Number(state.tracksAlbumContextID)
          ? (state.tracks || []).filter((t) => {
              if (normText(t.album) !== normText(state.selectedAlbum.title)) return false;
              if (!state.selectedAlbum.artist || !t.artist) return true;
              return normText(t.artist) === normText(state.selectedAlbum.artist);
            })
          : (state.tracks || []);
        return Array.from(new Set(pool.map((t) => (t.genre || '').trim()).filter(Boolean))).sort((a, b) => a.localeCompare(b));
      }
      return [];
    }

    function syncGenreFilterControl(view = state.currentView) {
      const control = $('filterGenre');
      const activeView = view === 'albums' || view === 'tracks' ? view : state.currentView;
      const show = activeView === 'albums' || activeView === 'tracks';
      control.classList.toggle('hidden', !show);
      if (!show) return;

      const options = genreOptionsForView(activeView);
      const currentValue = activeView === 'albums' ? (state.albumGenreFilter || 'all') : (state.trackGenreFilter || 'all');
      const fallbackLabel = activeView === 'albums' ? 'All album genres' : 'All track genres';
      control.innerHTML = `<option value="all">${fallbackLabel}</option>${options.map((genre) => `<option value="${escapeHtml(genre)}">${escapeHtml(genre)}</option>`).join('')}`;
      control.value = options.includes(currentValue) ? currentValue : 'all';
      if (activeView === 'albums') state.albumGenreFilter = control.value;
      if (activeView === 'tracks') state.trackGenreFilter = control.value;
    }

    function sourceContextForPlayback(explicit = '') {
      if (explicit) return explicit;
      if (state.currentView === 'tracks' && state.selectedAlbum && Number(state.selectedAlbum.id) === Number(state.tracksAlbumContextID)) {
        return 'album_tracks';
      }
      switch (state.currentView) {
        case 'albums': return 'albums';
        case 'tracks': return 'tracks';
        case 'playlists': return 'playlists';
        case 'track_detail': return 'track_detail';
        case 'user_profile': return 'user_profile';
        case 'creator_stats': return 'creator_stats';
        default: return state.currentView || 'unknown';
      }
    }

    async function sendListeningEvent(eventType, trackId, extra = {}) {
      if (!trackId) return;
      try {
        const res = await apiFetch('/api/v1/listening-events', {
          method: 'POST',
          headers: headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({
            track_id: trackId,
            event_type: eventType,
            source_context: extra.source_context || sourceContextForPlayback(),
            session_id: extra.session_id || '',
            playback_seconds: Number(extra.playback_seconds || 0),
            duration_seconds: Number(extra.duration_seconds || 0)
          })
        }, false);
        if (!res.ok) return;
      } catch (_) {}
    }

    function beginListeningSession(track, sourceContext = '') {
      if (!track || !track.id) return;
      state.listeningSession = {
        track_id: track.id,
        source_context: sourceContextForPlayback(sourceContext),
        session_id: `sess_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`,
        duration_seconds: Number(track.duration || 0),
        max_playback_seconds: 0,
        sent30: false,
        sent50: false,
        sentComplete: false
      };
      sendListeningEvent('play_start', track.id, {
        session_id: state.listeningSession.session_id,
        source_context: state.listeningSession.source_context,
        duration_seconds: state.listeningSession.duration_seconds
      });
    }

    function finalizeListeningSession(reason = 'switch') {
      const s = state.listeningSession;
      if (!s || !s.track_id) return;
      const duration = Number(s.duration_seconds || 0);
      const played = Number(s.max_playback_seconds || 0);
      if (!s.sentComplete) {
        const completed = duration > 0 ? played >= Math.max(30, duration * 0.92) : played >= 180;
        if (completed) {
          s.sentComplete = true;
          sendListeningEvent('play_complete', s.track_id, {
            session_id: s.session_id,
            source_context: s.source_context,
            playback_seconds: played,
            duration_seconds: duration
          });
        } else if (reason !== 'natural_end' && played > 0 && played < Math.min(30, duration > 0 ? duration * 0.25 : 30)) {
          sendListeningEvent('skip_early', s.track_id, {
            session_id: s.session_id,
            source_context: s.source_context,
            playback_seconds: played,
            duration_seconds: duration
          });
        }
      }
      state.listeningSession = null;
    }

    function updateListeningSession(currentTime, duration) {
      const s = state.listeningSession;
      if (!s) return;
      const cur = Number(currentTime || 0);
      const dur = Number(duration || s.duration_seconds || 0);
      if (cur > s.max_playback_seconds) s.max_playback_seconds = cur;
      if (dur > 0) s.duration_seconds = dur;
      if (!s.sent30 && cur >= 30) {
        s.sent30 = true;
        sendListeningEvent('play_30s', s.track_id, {
          session_id: s.session_id,
          source_context: s.source_context,
          playback_seconds: cur,
          duration_seconds: dur
        });
      }
      if (!s.sent50 && dur > 0 && (cur / dur) >= 0.5) {
        s.sent50 = true;
        sendListeningEvent('play_50_percent', s.track_id, {
          session_id: s.session_id,
          source_context: s.source_context,
          playback_seconds: cur,
          duration_seconds: dur
        });
      }
      if (!s.sentComplete && dur > 0 && (cur / dur) >= 0.95) {
        s.sentComplete = true;
        sendListeningEvent('play_complete', s.track_id, {
          session_id: s.session_id,
          source_context: s.source_context,
          playback_seconds: cur,
          duration_seconds: dur
        });
      }
    }


    function renderAlbums() {
      const root = $('albumsGrid');
      root.innerHTML = '';
      for (const album of state.filteredAlbums) {
        const card = document.createElement('div');
        card.className = `album-card ${state.selectedAlbum && state.selectedAlbum.id === album.id ? 'active' : ''}`;
        const albumTracks = state.filteredTracks.filter((t) => normText(t.album) === normText(album.title) && (!album.artist || normText(t.artist) === normText(album.artist)));
        const albumAvg = calcAlbumAvgRating(albumTracks);
        const coverURL = state.albumCoverURLs[album.id] || '';
        const coverHTML = coverURL
          ? `<div class="album-cover" style="background-image: linear-gradient(180deg, rgba(0,0,0,0.02), rgba(0,0,0,0.5)), url('${coverURL}'); background-size: cover; background-position: center;">${album.visibility.toUpperCase()}</div>`
          : `<div class="album-cover">${album.visibility.toUpperCase()}</div>`;
        card.innerHTML = `
          ${coverHTML}
          <div class="album-name" title="${album.title}">${album.title}</div>
          <div class="album-meta-stack">
            <div class="album-meta-line">
              <span><strong>Artist:</strong> ${album.artist || '-'}</span>
              <span class="album-meta-tag"><strong>Genre</strong> ${album.genre || '-'}</span>
              ${state.me ? `<button class="btn slim" data-fav-album="${album.id}">${isFavorite('album', album.id) ? '★' : '☆'}</button>` : ''}
            </div>
            <div class="album-meta-line">
              <span><strong>By</strong> <a class="uploader-link" data-user-sub="${album.owner_sub || ''}">${album.uploader_name || album.owner_sub || '-'}</a></span>
              <span><strong>Rating</strong> ${albumAvg > 0 ? albumAvg.toFixed(1) : '-'}★</span>
              <span><strong>Visibility</strong> ${album.visibility || '-'}</span>
            </div>
          </div>
        `;
        card.onclick = () => selectAlbum(album.id);
        const up = card.querySelector('[data-user-sub]');
        if (up) {
          up.onclick = (e) => {
            e.stopPropagation();
            const sub = up.getAttribute('data-user-sub');
            if (sub) openUserProfile(sub);
          };
        }
        const fav = card.querySelector('[data-fav-album]');
        if (fav) fav.onclick = async (e) => {
          e.stopPropagation();
          await toggleFavorite('album', album.id);
        };
        root.appendChild(card);
      }
      if (!state.filteredAlbums.length) {
        root.innerHTML = '<div class="muted" style="padding:10px;">No albums in current view.</div>';
      }
    }


    function getSelectedAlbumTracks() {
      if (!state.selectedAlbum) return [];
      const rows = state.filteredTracks.filter((t) => {
        if (normText(t.album) !== normText(state.selectedAlbum.title)) return false;
        if (!state.selectedAlbum.artist || !t.artist) return true;
        return normText(t.artist) === normText(state.selectedAlbum.artist);
      });
      rows.sort((a, b) => {
        const an = Number(a.track_number || 0);
        const bn = Number(b.track_number || 0);
        if (an > 0 && bn > 0 && an !== bn) return an - bn;
        if (an > 0 && bn <= 0) return -1;
        if (bn > 0 && an <= 0) return 1;
        const byTitle = String(a.title || '').localeCompare(String(b.title || ''));
        if (byTitle !== 0) return byTitle;
        return String(a.id || '').localeCompare(String(b.id || ''));
      });
      return rows;
    }

    function renderTracksAlbumHeader(rows, hasAlbum) {
      const activeAlbumContext = hasAlbum && state.currentView === 'tracks';
      $('tracksAlbumHero').classList.toggle('hidden', !activeAlbumContext);
      $('tracksActionbar').classList.toggle('hidden', !activeAlbumContext);
      $('albumSocialPanel').classList.toggle('hidden', !activeAlbumContext);
      const canPlaylist = canCreatePlaylists();
      $('btnTracksPlayNext').classList.toggle('hidden', !canPlaylist);
      $('btnTracksPlayLater').classList.toggle('hidden', !canPlaylist);
      $('btnTracksAddPlaylist').classList.toggle('hidden', !canPlaylist);
      if (!activeAlbumContext) return;

      const total = rows.reduce((sum, t) => sum + (t.duration || 0), 0);
      const avgRating = calcAlbumAvgRating(rows);
      const album = state.selectedAlbum;
      const coverURL = state.albumCoverURLs[album.id] || '';
      $('tracksAlbumTitle').textContent = album.title || 'Album';
      $('tracksAlbumMeta').textContent = `${album.artist || '-'} · by ${album.uploader_name || album.owner_sub || '-'} · ${album.genre || '-'} · ${rows.length} songs · ${fmt(total)} · rating ${avgRating > 0 ? avgRating.toFixed(1) : '-'} · ${album.visibility || '-'}`;
      const cover = $('tracksAlbumCover');
      if (coverURL) {
        cover.style.backgroundImage = `linear-gradient(180deg, rgba(0,0,0,0.04), rgba(0,0,0,0.5)), url('${coverURL}')`;
      } else {
        cover.style.backgroundImage = 'linear-gradient(135deg, #24334f, #5f7188)';
      }
      renderAlbumComments();
    }

    function renderTracks() {
      const body = $('tracksBody');
      body.innerHTML = '';
      const hasAlbumContext = !!(state.selectedAlbum && Number(state.selectedAlbum.id) === Number(state.tracksAlbumContextID));
      const rows = hasAlbumContext ? getSelectedAlbumTracks() : state.filteredTracks;
      renderTracksAlbumHeader(rows, hasAlbumContext);

      rows.forEach((t, idx) => {
        const tr = document.createElement('tr');
        const uploader = t.uploader_name || t.owner_sub || '-';
        tr.innerHTML = `
          <td>${idx + 1}</td>
          <td>
            <div class="track-main-title">${t.title}</div>
            <div class="track-meta-line">
              <span><strong>Artist</strong> ${t.artist || '-'}</span>
              <span class="sep">·</span>
              <span><strong>Album</strong> ${t.album || '-'}</span>
              <span class="sep">·</span>
              <span><strong>Genre</strong> ${t.genre || '-'}</span>
              <span class="sep">·</span>
              <span><strong>By</strong> <a class="uploader-link" data-uploader="${t.owner_sub || ''}">${uploader}</a></span>
            </div>
          </td>
          <td>${t.artist}</td>
          <td>${t.album}</td>
          <td>${t.genre || '-'}</td>
          <td>${fmt(t.duration)}</td>
          <td>${renderLyricIndicators(t)}</td>
          <td>${state.me ? renderRateableStars(t.id, t.rating) : `<span class="stars">${renderStars(t.rating)}<span class="stars-meta">${renderTrackRating(t.rating)}</span></span>`}</td>
          <td>${shortVisibility(t.visibility)}</td>
          <td>
            <span class="track-actions">
              <button class="btn" data-play="${t.id}" title="Play" aria-label="Play">${ICON_PLAY}</button>
              <button class="btn" data-detail="${t.id}" title="Details" aria-label="Details">${ICON_DETAIL}</button>
              ${state.me ? `<button class="btn" data-fav-track="${t.id}" title="Favorite" aria-label="Favorite">${isFavorite('track', t.id) ? '★' : '☆'}</button>` : ''}
              ${canCreatePlaylists() ? `<button class="btn" data-addpl="${t.id}" title="Add to Playlist" aria-label="Add to Playlist">${ICON_PLAYLIST}</button>` : ''}
              ${canManageTrack(t) ? `<button class="btn" data-manage="${t.id}" title="Manage" aria-label="Manage">M</button>` : ''}
            </span>
          </td>
        `;
        const playBtn = tr.querySelector('[data-play]');
        if (playBtn) playBtn.onclick = () => startTrackById(t.id, rows);
        const detailBtn = tr.querySelector('[data-detail]');
        if (detailBtn) detailBtn.onclick = () => openTrackDetail(t.id);
        const favBtn = tr.querySelector('[data-fav-track]');
        if (favBtn) favBtn.onclick = async () => toggleFavorite('track', t.id);
        const uploaderLink = tr.querySelector('[data-uploader]');
        if (uploaderLink) {
          uploaderLink.onclick = (e) => {
            e.stopPropagation();
            const sub = uploaderLink.getAttribute('data-uploader');
            if (sub) openUserProfile(sub);
          };
        }
        tr.querySelectorAll('[data-rate-track]').forEach((btn) => {
          btn.onclick = async () => {
            await rateTrackByID(btn.getAttribute('data-rate-track'), btn.getAttribute('data-rate-value'));
          };
        });
        const addPLBtn = tr.querySelector('[data-addpl]');
        if (addPLBtn) addPLBtn.onclick = () => addTrackToPlaylist(t.id);

        const manageBtn = tr.querySelector('[data-manage]');
        if (manageBtn) {
          manageBtn.onclick = async () => {
            const targetView = canAdmin() ? 'admin_track_manage' : 'user_track_manage';
            switchView(targetView);
            const loaded = await loadTrackForEditor(t.id, false);
            if (!loaded) {
              alert('Track kann nicht bearbeitet werden (kein Zugriff oder nicht gefunden).');
              setAdminTarget(null);
            }
          };
        }
        body.appendChild(tr);
      });

      if (!rows.length) {
        body.innerHTML = '<tr><td colspan="10" class="muted">No tracks in current view.</td></tr>';
      }
    }

    function renderLyricIndicators(track) {
      const hasPlain = !!track && !!track.has_lyrics_txt;
      const hasSync = !!track && !!track.has_lyrics_srt;
      if (!hasPlain && !hasSync) return '<span class="muted">-</span>';
      const chips = [];
      if (hasPlain) {
        chips.push('<span class="lyric-indicator plain" title="Plain lyrics" aria-label="Plain lyrics">T</span>');
      }
      if (hasSync) {
        chips.push('<span class="lyric-indicator sync" title="Synced lyrics" aria-label="Synced lyrics">S</span>');
      }
      return `<span class="lyric-indicators">${chips.join('')}</span>`;
    }

    function renderTrackRating(v) {
      const n = Number(v || 0);
      if (!Number.isFinite(n) || n <= 0) return '-';
      return `${Math.min(5, Math.max(0, n)).toFixed(1)}/5`;
    }

    function renderStars(v, size = 5) {
      const n = Number(v || 0);
      const rounded = Number.isFinite(n) && n > 0 ? Math.min(size, Math.max(0, Math.round(n))) : 0;
      let out = '';
      for (let i = 1; i <= size; i += 1) {
        out += `<span class="star ${i <= rounded ? 'on' : ''}">&#9733;</span>`;
      }
      return out;
    }

    function renderRateableStars(trackID, value) {
      const n = Number(value || 0);
      const rounded = Number.isFinite(n) && n > 0 ? Math.min(5, Math.max(0, Math.round(n))) : 0;
      let out = '<span class="stars interactive">';
      for (let i = 1; i <= 5; i += 1) {
        out += `<button class="star ${i <= rounded ? 'on' : ''}" data-rate-track="${trackID}" data-rate-value="${i}" title="Rate ${i}/5">&#9733;</button>`;
      }
      out += '</span>';
      return out;
    }

    function shortVisibility(v) {
      const x = String(v || '').toLowerCase();
      if (x === 'followers_only') return 'follow';
      if (x === 'unlisted') return 'unlist';
      if (x === 'private') return 'priv';
      if (x === 'public') return 'pub';
      return x || '-';
    }

    function calcAlbumAvgRating(tracks) {
      const vals = tracks
        .map((t) => Number(t.rating || 0))
        .filter((n) => Number.isFinite(n) && n > 0);
      if (!vals.length) return 0;
      return vals.reduce((a, b) => a + b, 0) / vals.length;
    }

    function renderAlbumComments() {
      const hasAlbum = !!state.selectedAlbum;
      $('albumSocialPanel').classList.toggle('hidden', !hasAlbum);
      if (!hasAlbum) return;

      const tracks = getSelectedAlbumTracks();
      const avg = calcAlbumAvgRating(tracks);
      $('albumSocialRating').textContent = `Rating: ${avg > 0 ? `${avg.toFixed(1)}/5` : '-'}`;

      const loggedIn = !!state.me;
      $('albumCommentForm').classList.toggle('hidden', !loggedIn);
      $('albumCommentGuestHint').classList.toggle('hidden', loggedIn);

      const root = $('albumCommentsList');
      root.innerHTML = '';
      const comments = state.selectedAlbumComments || [];
      comments.forEach((c) => {
        const item = document.createElement('div');
        item.className = 'album-comment-item';
        const head = document.createElement('div');
        head.className = 'album-comment-head';
        const author = document.createElement('a');
        author.className = 'uploader-link';
        author.textContent = c.author_name || c.author_sub || 'user';
        author.href = '#';
        author.onclick = async (e) => {
          e.preventDefault();
          if (c.author_sub) await openUserProfile(c.author_sub);
        };
        const created = document.createElement('span');
        created.textContent = (c.created_at || '').replace('T', ' ').slice(0, 19);
        head.appendChild(author);
        head.appendChild(created);
        const body = document.createElement('div');
        body.className = 'album-comment-body';
        body.textContent = c.content || '';
        item.appendChild(head);
        item.appendChild(body);
        root.appendChild(item);
      });
      if (!comments.length) {
        root.innerHTML = '<div class="muted">No comments yet.</div>';
      }
    }

    async function loadAlbumComments(albumID) {
      if (!albumID) {
        state.selectedAlbumComments = [];
        renderAlbumComments();
        return;
      }
      const res = await apiFetch(`/api/v1/albums/${albumID}/comments`, { headers: headers() }, false);
      if (!res.ok) {
        state.selectedAlbumComments = [];
        renderAlbumComments();
        return;
      }
      const json = await res.json();
      state.selectedAlbumComments = json.comments || [];
      renderAlbumComments();
    }

    async function createAlbumComment() {
      if (!state.selectedAlbum || !state.selectedAlbum.id) return;
      if (!state.me) {
        alert('Login required.');
        return;
      }
      const content = $('albumCommentInput').value.trim();
      if (!content) return;
      const res = await apiFetch(`/api/v1/albums/${state.selectedAlbum.id}/comments`, {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ content })
      });
      if (!res.ok) {
        alert(`Comment failed (${res.status}).`);
        return;
      }
      $('albumCommentInput').value = '';
      await loadAlbumComments(state.selectedAlbum.id);
    }

    async function rateTrackByID(trackID, rating) {
      if (!state.me) {
        alert('Login required.');
        return false;
      }
      const n = Number(rating);
      if (!Number.isFinite(n) || n < 1 || n > 5) {
        alert('Rating must be 1..5.');
        return false;
      }
      const res = await apiFetch(`/api/v1/tracks/${encodeURIComponent(trackID)}/rating`, {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ rating: Math.round(n) })
      });
      if (!res.ok) {
        alert(`Rating failed (${res.status}).`);
        return false;
      }
      await loadData();
      if (state.selectedDetailTrackId === trackID) {
        await openTrackDetail(trackID);
      }
      return true;
    }


    function renderTrackDetail() {
      const t = state.selectedDetailTrackData;
      if (!t) {
        $('detailTrackTitle').textContent = 'Select a track';
        $('detailTrackMeta').textContent = '-';
        $('detailTrackLyrics').textContent = 'No lyrics available.';
        $('detailTrackRateStars').innerHTML = '';
        $('detailTrackRatingMeta').textContent = 'Track rating: -';
        return;
      }
      $('detailTrackTitle').textContent = t.title || 'Untitled';
      const up = t.uploader_name || t.owner_sub || '-';
      $('detailTrackMeta').textContent = `${t.artist || '-'} · ${t.album || '-'} · by ${up} · ${t.genre || '-'} · ${t.visibility || '-'}`;
      $('detailTrackLyrics').textContent = (t.lyrics_txt && t.lyrics_txt.trim()) ? t.lyrics_txt : 'No lyrics available.';
      const avg = Number(t.rating || 0);
      $('detailTrackRatingMeta').textContent = `Track rating: ${avg > 0 ? `${avg.toFixed(1)}/5` : '-'}`;
      if (state.me) {
        $('detailTrackRateStars').innerHTML = renderRateableStars(t.id, avg);
      } else {
        $('detailTrackRateStars').innerHTML = `<span class="stars">${renderStars(avg)}<span class="stars-meta">Login to rate</span></span>`;
      }
      $('detailTrackRateStars').querySelectorAll('[data-rate-track]').forEach((btn) => {
        btn.onclick = async () => {
          await rateTrackByID(btn.getAttribute('data-rate-track'), btn.getAttribute('data-rate-value'));
        };
      });
    }

    async function openTrackDetail(trackID) {
      const res = await apiFetch(`/api/v1/tracks/${encodeURIComponent(trackID)}`, { headers: headers() }, false);
      if (!res.ok) {
        // Retry with auth headers if guest call was forbidden for private tracks.
        const authed = await apiFetch(`/api/v1/tracks/${encodeURIComponent(trackID)}`, { headers: headers() });
        if (!authed.ok) {
          alert('Track details not available.');
          return;
        }
        state.selectedDetailTrackData = await authed.json();
      } else {
        state.selectedDetailTrackData = await res.json();
      }
      state.selectedDetailTrackId = trackID;
      renderTrackDetail();
      switchView('track_detail');
    }

    async function loadData() {
      const [tracksRes, albumsRes] = await Promise.all([
        apiFetch('/api/v1/tracks', { headers: headers() }),
        apiFetch('/api/v1/albums', { headers: headers() })
      ]);
      const tracksJson = await tracksRes.json();
      const albumsJson = await albumsRes.json();

      state.tracks = (tracksJson.tracks || []).map(normalizeTrack);
      clearAlbumCoverURLs();
      state.albums = (albumsJson.albums || []).map((a) => ({
        id: a.id,
        title: a.title,
        artist: a.artist,
        genre: a.genre || '',
        visibility: a.visibility,
        cover_path: a.cover_path || '',
        owner_sub: a.owner_sub || '',
        uploader_name: a.uploader_name || a.owner_sub || ''
      }));
      await preloadAlbumCovers();

      if (state.selectedAlbum) {
        state.selectedAlbum = state.albums.find((a) => a.id === state.selectedAlbum.id) || null;
      } else {
        const persistedAlbumID = loadSelectedAlbumID();
        if (persistedAlbumID > 0) {
          state.selectedAlbum = state.albums.find((a) => a.id === persistedAlbumID) || null;
        }
      }
      if (state.selectedAlbum) {
        saveSelectedAlbumID(state.selectedAlbum.id);
      } else {
        saveSelectedAlbumID(0);
      }

      syncGenreFilterControl(state.currentView);
      applyFilters();
      await loadDiscovery();
      renderAlbums();
      renderTracks();
      renderAlbumDetail();
      if (state.selectedAlbum && state.selectedAlbum.id) {
        await loadAlbumComments(state.selectedAlbum.id);
      } else {
        state.selectedAlbumComments = [];
        renderAlbumComments();
      }
      await loadManageData();
      await loadPlaylists();
      await loadFavorites();
      renderFavorites();
      await loadNotifications();
      await loadMyProfile();
      await loadCreatorHighscore();
      if (canUpload() && !$('viewCreatorStats').classList.contains('hidden')) {
        await loadCreatorStats();
      }
      if (canAdmin()) {
        await loadAdminUsers();
        if (!$('viewAdminSystem').classList.contains('hidden')) {
          await loadAdminSystemOverview();
        }
        if (!$('viewAdminLogs').classList.contains('hidden')) {
          await loadAdminDebugToggle();
          await loadAdminAuditLogs();
        }
      } else {
        state.adminUsers = [];
        renderAdminUsers();
      }
      if (state.queueIndex >= 0 && state.queue[state.queueIndex]) {
        await updateNowPlayingVisuals(state.queue[state.queueIndex]);
        emitPlayerState(true);
      }
      if (canAdmin() && !$('viewJobs').classList.contains('hidden')) {
        await loadJobs();
      }
    }

    async function selectAlbum(id) {
      state.selectedAlbum = state.albums.find((a) => a.id === id) || null;
      state.tracksAlbumContextID = state.selectedAlbum ? Number(state.selectedAlbum.id) : 0;
      saveSelectedAlbumID(state.selectedAlbum ? state.selectedAlbum.id : 0);
      renderAlbums();
      renderTracks();
      renderAlbumDetail();
      if (state.selectedAlbum && state.selectedAlbum.id) {
        await loadAlbumComments(state.selectedAlbum.id);
      } else {
        state.selectedAlbumComments = [];
        renderAlbumComments();
      }
      switchView('tracks');
    }

Object.assign(window.HexSonic, {
      clearAlbumCoverURLs,
      preloadAlbumCovers,
      normalizeTrack,
      applyFilters,
      genreOptionsForView,
      syncGenreFilterControl,
      sourceContextForPlayback,
      sendListeningEvent,
      beginListeningSession,
      finalizeListeningSession,
      updateListeningSession,
      renderAlbums,
      getSelectedAlbumTracks,
      renderTracksAlbumHeader,
      renderTracks,
      renderLyricIndicators,
      renderTrackRating,
      renderStars,
      renderRateableStars,
      shortVisibility,
      calcAlbumAvgRating,
      renderAlbumComments,
      loadAlbumComments,
      createAlbumComment,
      rateTrackByID,
      renderTrackDetail,
      openTrackDetail,
      loadData,
      selectAlbum
});
})(window.HexSonic = window.HexSonic || {});
