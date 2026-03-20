(function(ns) {
const { state, $, escapeHtml, headers, apiFetch, fmt, ICON_PLAYLIST } = ns;
const canCreatePlaylists = (...args) => ns.canCreatePlaylists(...args);
const startTrackById = (...args) => ns.startTrackById(...args);
const openTrackDetail = (...args) => ns.openTrackDetail(...args);
const addTrackToPlaylist = (...args) => ns.addTrackToPlaylist(...args);
const selectAlbum = (...args) => ns.selectAlbum(...args);
const canUpload = (...args) => ns.canUpload(...args);

    function renderDiscoveryTrackList(rows, sourceLabel) {
      const items = Array.isArray(rows) ? rows : [];
      if (!items.length) return '<div class="muted">No tracks yet.</div>';
      return `<div class="discovery-track-list">${items.slice(0, 8).map((t) => `
        <div class="discovery-track-item">
          <div class="discovery-track-top">
            <div>
              <div class="discovery-track-title">${escapeHtml(t.title || '-')}</div>
              <div class="discovery-track-meta">${escapeHtml(t.artist || '-')} · ${escapeHtml(t.album || '-')} · ${escapeHtml(t.genre || '-')}</div>
            </div>
            <span class="pill">${Number(t.score || 0).toFixed(0)}</span>
          </div>
          <div class="discovery-track-actions">
            <button class="btn slim" data-discovery-play="${escapeHtml(t.id || '')}" data-discovery-source="${escapeHtml(sourceLabel)}">Play</button>
            <button class="btn slim" data-discovery-detail="${escapeHtml(t.id || '')}">Details</button>
            ${canCreatePlaylists() ? `<button class="btn slim" data-discovery-playlist="${escapeHtml(t.id || '')}">${ICON_PLAYLIST}</button>` : ''}
          </div>
        </div>
      `).join('')}</div>`;
    }

    function renderDiscoveryAlbumList(rows) {
      const items = Array.isArray(rows) ? rows : [];
      if (!items.length) return '<div class="muted">No albums yet.</div>';
      return `<div class="discovery-track-list">${items.slice(0, 6).map((a) => `
        <div class="discovery-track-item">
          <div class="discovery-track-top">
            <div>
              <div class="discovery-track-title">${escapeHtml(a.title || '-')}</div>
              <div class="discovery-track-meta">${escapeHtml(a.artist || '-')} · ${escapeHtml(a.genre || '-')} · ${Number(a.track_count || 0)} tracks</div>
            </div>
            <span class="pill">${Number(a.score || 0).toFixed(0)}</span>
          </div>
          <div class="discovery-track-actions">
            <button class="btn slim" data-discovery-album="${Number(a.id || 0)}">Open album</button>
          </div>
        </div>
      `).join('')}</div>`;
    }

    function bindDiscoveryActions() {
      $('discoveryGrid').querySelectorAll('[data-discovery-tab]').forEach((btn) => {
        btn.onclick = () => {
          const tab = btn.getAttribute('data-discovery-tab') || 'top';
          if (tab !== 'top' && tab !== 'personal') return;
          state.discoveryTab = tab;
          renderDiscovery();
        };
      });
      $('discoveryGrid').querySelectorAll('[data-discovery-play]').forEach((btn) => {
        btn.onclick = async () => {
          const id = btn.getAttribute('data-discovery-play');
          const source = btn.getAttribute('data-discovery-source') || 'discovery';
          if (!id) return;
          const all = state.tracks.slice();
          await startTrackById(id, all, source);
        };
      });
      $('discoveryGrid').querySelectorAll('[data-discovery-detail]').forEach((btn) => {
        btn.onclick = async () => {
          const id = btn.getAttribute('data-discovery-detail');
          if (!id) return;
          await openTrackDetail(id);
        };
      });
      $('discoveryGrid').querySelectorAll('[data-discovery-playlist]').forEach((btn) => {
        btn.onclick = async () => {
          const id = btn.getAttribute('data-discovery-playlist');
          if (!id) return;
          await addTrackToPlaylist(id);
        };
      });
      $('discoveryGrid').querySelectorAll('[data-discovery-album]').forEach((btn) => {
        btn.onclick = async () => {
          const id = Number(btn.getAttribute('data-discovery-album') || '0');
          if (!id) return;
          await selectAlbum(id);
        };
      });
    }

    function renderDiscovery() {
      const root = $('discoveryGrid');
      const data = state.discovery || {};
      const global = data.global || {};
      const personal = data.personal || {};
      const favoriteGenres = Array.isArray(personal.favorite_genres) ? personal.favorite_genres : [];
      const hasPersonal = !!personal.enabled;
      if (!hasPersonal && state.discoveryTab === 'personal') {
        state.discoveryTab = 'top';
      }
      const songsTab = state.discoveryTab === 'personal' && hasPersonal ? 'personal' : 'top';
      const songsTitle = songsTab === 'personal' ? 'For You' : 'Top Songs';
      const songsRows = songsTab === 'personal' ? (personal.recommended_tracks || []) : (global.top_songs || []);
      const songsSource = songsTab === 'personal' ? 'discovery_personal' : 'discovery_top';
      const songsBadge = songsTab === 'personal'
        ? `<span class="pill ok">Personal</span>`
        : `<span class="pill">${Number((global.top_songs || []).length || 0)} picks</span>`;
      $('discoverySummary').textContent = personal.enabled
        ? (personal.summary || 'Personal and global discovery active.')
        : 'Global discovery is active. Login enables personal recommendations.';
      root.innerHTML = `
        <div class="discovery-column">
          <div class="discovery-column-head">
            <div class="discovery-column-title">${songsTitle}</div>
            <div class="discovery-tabs">
              ${hasPersonal ? `<button class="discovery-tab ${songsTab === 'top' ? 'active' : ''}" data-discovery-tab="top">Top</button><button class="discovery-tab ${songsTab === 'personal' ? 'active' : ''}" data-discovery-tab="personal">For You</button>` : ''}
              ${songsBadge}
            </div>
          </div>
          <div class="discovery-column-intro">
            ${songsTab === 'personal'
              ? (favoriteGenres.length ? `<div class="discovery-chip-row">${favoriteGenres.map((g) => `<span class="discovery-chip">${escapeHtml(g)}</span>`).join('')}</div>` : '<div class="muted">No personal genre profile yet.</div>')
              : '<div class="muted">Most relevant public tracks across the platform.</div>'}
          </div>
          <div class="discovery-column-body">
            ${renderDiscoveryTrackList(songsRows, songsSource)}
          </div>
        </div>
        <div class="discovery-column">
          <div class="discovery-column-head"><div class="discovery-column-title">Trending</div><span class="pill">Live</span></div>
          <div class="discovery-column-intro"><div class="muted">Movement based on recent listening activity.</div></div>
          <div class="discovery-column-body">
            ${renderDiscoveryTrackList(global.trending_tracks || [], 'discovery_trending')}
          </div>
        </div>
        <div class="discovery-column">
          <div class="discovery-column-head"><div class="discovery-column-title">Albums</div><span class="pill">${Number((global.top_albums || []).length || 0)} picks</span></div>
          <div class="discovery-column-intro"><div class="muted">Albums that currently pull the strongest listening signals.</div></div>
          <div class="discovery-column-body">
            ${renderDiscoveryAlbumList(global.top_albums || [])}
          </div>
        </div>
      `;
      bindDiscoveryActions();
    }

    async function loadDiscovery() {
      const res = await apiFetch('/api/v1/discovery', { headers: headers() }, false);
      if (!res.ok) {
        state.discovery = null;
        $('discoverySummary').textContent = 'Discovery is temporarily unavailable.';
        $('discoveryGrid').innerHTML = '';
        return;
      }
      state.discovery = await res.json();
      renderDiscovery();
    }

    function renderCreatorStats() {
      const data = state.creatorStats || {};
      const ov = data.overview || {};
      $('creatorStatsSummary').textContent = `Window: ${data.window || '-'} · Updated: ${(data.server_time || '').replace('T', ' ').replace('Z', ' UTC') || '-'}`;
      $('creatorStatsMetrics').innerHTML = `
        <div class="metric-card"><div class="metric-label">Tracks</div><div class="metric-value">${Number(ov.tracks_total || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Albums</div><div class="metric-value">${Number(ov.albums_total || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Total Plays</div><div class="metric-value">${Number(ov.plays_total || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Unique Listeners</div><div class="metric-value">${Number(ov.unique_listeners || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Qualified Listens</div><div class="metric-value">${Number(ov.qualified_listens || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Completed</div><div class="metric-value">${Number(ov.completed_listens || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Playlist Adds</div><div class="metric-value">${Number(ov.playlist_adds || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Avg Rating</div><div class="metric-value">${Number(ov.avg_rating || 0).toFixed(1)}</div></div>
      `;
      const renderRows = (rows, kind) => {
        const list = Array.isArray(rows) ? rows : [];
        if (!list.length) return '<tr><td colspan="4" class="muted">No data in this window.</td></tr>';
        return list.map((row, idx) => `
          <tr>
            <td>${idx + 1}. ${escapeHtml(row.title || '-')}</td>
            <td>${escapeHtml(row.artist || '-')}</td>
            <td>${escapeHtml(row.genre || '-')}</td>
            <td style="text-align:right;">${Number(row.score || 0).toFixed(0)}</td>
          </tr>
        `).join('');
      };
      const renderSourceRows = (rows) => {
        const list = Array.isArray(rows) ? rows : [];
        if (!list.length) return '<tr><td colspan="2" class="muted">No source data.</td></tr>';
        return list.map((row) => `
          <tr>
            <td>${escapeHtml(row.source_context || 'unknown')}</td>
            <td style="text-align:right;">${Number(row.events || 0)}</td>
          </tr>
        `).join('');
      };
      $('creatorStatsGrid').innerHTML = `
        <div class="service-card">
          <div class="service-head"><b>Top Tracks</b><span class="pill">${Number((data.top_tracks || []).length || 0)} rows</span></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Track</th><th>Artist</th><th>Genre</th><th style="text-align:right;">Score</th></tr></thead>
              <tbody>${renderRows(data.top_tracks || [], 'track')}</tbody>
            </table>
          </div>
        </div>
        <div class="service-card">
          <div class="service-head"><b>Top Albums</b><span class="pill">${Number((data.top_albums || []).length || 0)} rows</span></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Album</th><th>Artist</th><th>Genre</th><th style="text-align:right;">Score</th></tr></thead>
              <tbody>${renderRows(data.top_albums || [], 'album')}</tbody>
            </table>
          </div>
        </div>
        <div class="service-card">
          <div class="service-head"><b>Traffic Sources</b><span class="pill">Discovery</span></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Source</th><th style="text-align:right;">Events</th></tr></thead>
              <tbody>${renderSourceRows(data.sources || [])}</tbody>
            </table>
          </div>
        </div>
      `;
    }

    async function loadCreatorStats() {
      if (!canUpload()) {
        $('creatorStatsSummary').textContent = 'Creator badge or admin role required.';
        $('creatorStatsMetrics').innerHTML = '';
        $('creatorStatsGrid').innerHTML = '';
        return;
      }
      const windowValue = $('creatorStatsWindow').value || '7d';
      const res = await apiFetch(`/api/v1/creator/stats?window=${encodeURIComponent(windowValue)}`, { headers: headers() });
      if (!res.ok) {
        $('creatorStatsSummary').textContent = `Creator stats unavailable (${res.status}).`;
        $('creatorStatsMetrics').innerHTML = '';
        $('creatorStatsGrid').innerHTML = '';
        return;
      }
      state.creatorStats = await res.json();
      renderCreatorStats();
    }

Object.assign(window.HexSonic, {
      renderDiscoveryTrackList,
      renderDiscoveryAlbumList,
      bindDiscoveryActions,
      renderDiscovery,
      loadDiscovery,
      renderCreatorStats,
      loadCreatorStats
});
})(window.HexSonic = window.HexSonic || {});
