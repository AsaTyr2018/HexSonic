(function(ns) {
  const { state, $, escapeHtml, headers, apiFetch } = ns;
  const loadData = (...args) => ns.loadData(...args);
  const loadManageData = (...args) => ns.loadManageData(...args);
  const setAdminTarget = (...args) => ns.setAdminTarget(...args);
  const setAlbumTarget = (...args) => ns.setAlbumTarget(...args);
  const loadTrackForEditor = (...args) => ns.loadTrackForEditor(...args);
  const saveAlbumMetadata = (...args) => ns.saveAlbumMetadata(...args);
  const adminDeleteTrack = (...args) => ns.adminDeleteTrack(...args);
  const switchView = (...args) => ns.switchView(...args);
  const canUpload = (...args) => ns.canUpload(...args);
  const canAdmin = (...args) => ns.canAdmin(...args);

  function switchCreatorCenterTab(tab) {
    const next = String(tab || 'overview').trim() || 'overview';
    state.creatorCenterTab = next;
    document.querySelectorAll('#creatorCenterTabs [data-creator-tab]').forEach((btn) => {
      btn.classList.toggle('active', btn.getAttribute('data-creator-tab') === next);
    });
    ['overview', 'uploads', 'albums', 'tracks'].forEach((key) => {
      const el = $(`creatorCenterTab${key.charAt(0).toUpperCase()}${key.slice(1)}`);
      if (el) el.classList.toggle('hidden', key !== next);
    });
  }

  function metricCard(label, value, note = '') {
    return `
      <div class="metric-card">
        <div class="metric-label">${escapeHtml(label)}</div>
        <div class="metric-value">${escapeHtml(String(value))}</div>
        <div class="metric-note">${escapeHtml(note)}</div>
      </div>
    `;
  }

  function renderCreatorCenterSummary() {
    const tracks = Array.isArray(state.manageTracks) ? state.manageTracks : [];
    const albums = Array.isArray(state.manageAlbums) ? state.manageAlbums : [];
    const publicTracks = tracks.filter((t) => t.visibility === 'public').length;
    const privateTracks = tracks.filter((t) => t.visibility !== 'public').length;
    const missingGenre = tracks.filter((t) => !String(t.genre || '').trim()).length;
    const missingLyrics = tracks.filter((t) => !t.has_lyrics_txt && !t.has_lyrics_srt).length;
    const albumsNoCover = albums.filter((a) => !String(a.cover_path || '').trim()).length;
    $('creatorCenterSummary').innerHTML = [
      metricCard('Albums', albums.length, 'Owned releases'),
      metricCard('Tracks', tracks.length, 'Owned songs'),
      metricCard('Public', publicTracks, 'Tracks visible to all'),
      metricCard('Private', privateTracks, 'Tracks still hidden'),
      metricCard('No Genre', missingGenre, 'Tracks needing genre cleanup'),
      metricCard('No Lyrics', missingLyrics, 'Tracks missing lyrics')
    ].join('');
    $('creatorCenterHealth').innerHTML = [
      { label: 'Albums without cover', value: albumsNoCover },
      { label: 'Tracks without genre', value: missingGenre },
      { label: 'Tracks without lyrics', value: missingLyrics },
      { label: 'Tracks with synced lyrics', value: tracks.filter((t) => t.has_lyrics_srt).length }
    ].map((row) => `
      <div class="creator-health-row">
        <strong>${escapeHtml(row.label)}</strong>
        <span class="muted">${escapeHtml(String(row.value))}</span>
      </div>
    `).join('');
    $('creatorCenterMeta').textContent = `${albums.length} albums · ${tracks.length} tracks · ${publicTracks} public tracks`;
  }

  function renderCreatorAlbums() {
    const body = $('creatorAlbumsBody');
    if (!body) return;
    const rows = Array.isArray(state.manageAlbums) ? state.manageAlbums : [];
    body.innerHTML = '';
    rows.forEach((album, idx) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td>${idx + 1}</td>
        <td>${escapeHtml(album.title || '-')}</td>
        <td>${escapeHtml(album.artist || '-')}</td>
        <td>${escapeHtml(album.genre || '-')}</td>
        <td>${Number(album.track_count || 0)}</td>
        <td>${escapeHtml(album.visibility || '-')}</td>
        <td>
          <div class="creator-row-actions">
            <button class="btn slim creator-select-btn" data-select-album="${album.id}">Select</button>
            <button class="btn slim danger" data-delete-album="${album.id}">Delete</button>
          </div>
        </td>
      `;
      const selectBtn = tr.querySelector('[data-select-album]');
      if (selectBtn) selectBtn.onclick = () => {
        setAlbumTarget(album);
        switchCreatorCenterTab('albums');
      };
      const deleteBtn = tr.querySelector('[data-delete-album]');
      if (deleteBtn) deleteBtn.onclick = async () => {
        setAlbumTarget(album);
        if (typeof ns.deleteAlbumMetadata === 'function') await ns.deleteAlbumMetadata();
      };
      body.appendChild(tr);
    });
    if (!rows.length) {
      body.innerHTML = '<tr><td colspan="7" class="muted">No albums yet.</td></tr>';
    }
  }

  function renderCreatorTracks() {
    const body = $('creatorTracksBody');
    if (!body) return;
    const rows = Array.isArray(state.manageTracks) ? state.manageTracks : [];
    body.innerHTML = '';
    rows.forEach((track) => {
      const tr = document.createElement('tr');
      const lyricsLabel = track.has_lyrics_srt && track.has_lyrics_txt ? 'T · S'
        : track.has_lyrics_srt ? 'S'
        : track.has_lyrics_txt ? 'T'
        : '-';
      tr.innerHTML = `
        <td>${escapeHtml(track.title || '-')}<div class="uploader-inline">${escapeHtml(track.artist || '-')}</div></td>
        <td>${escapeHtml(track.album || '-')}</td>
        <td>${escapeHtml(track.genre || '-')}</td>
        <td>${escapeHtml(lyricsLabel)}</td>
        <td>${escapeHtml(track.visibility || '-')}</td>
        <td>
          <div class="creator-row-actions">
            <button class="btn slim creator-select-btn" data-select-track="${track.id}">Edit</button>
            <button class="btn slim danger" data-delete-track="${track.id}">Delete</button>
          </div>
        </td>
      `;
      const selectBtn = tr.querySelector('[data-select-track]');
      if (selectBtn) selectBtn.onclick = async () => {
        await openCreatorCenterTrack(track.id);
      };
      const deleteBtn = tr.querySelector('[data-delete-track]');
      if (deleteBtn) deleteBtn.onclick = async () => {
        await openCreatorCenterTrack(track.id);
        await adminDeleteTrack();
      };
      body.appendChild(tr);
    });
    if (!rows.length) {
      body.innerHTML = '<tr><td colspan="6" class="muted">No tracks available.</td></tr>';
    }
  }

  function syncCreatorCenterSelection() {
    document.querySelectorAll('#creatorTracksBody tr').forEach((row) => row.classList.remove('selected'));
    document.querySelectorAll('#creatorAlbumsBody tr').forEach((row) => row.classList.remove('selected'));
  }

  async function createCreatorAlbum() {
    if (!canUpload()) {
      alert('Creator badge or admin role required.');
      return;
    }
    const payload = {
      title: $('creatorCreateAlbumTitle').value.trim(),
      artist: $('creatorCreateAlbumArtist').value.trim(),
      genre: $('creatorCreateAlbumGenre').value.trim(),
      visibility: $('creatorCreateAlbumVisibility').value
    };
    const out = $('creatorCreateAlbumStatus');
    out.className = 'muted';
    out.textContent = 'Creating album...';
    const res = await apiFetch('/api/v1/manage/albums-create', {
      method: 'POST',
      headers: headers({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(payload)
    });
    if (!res.ok) {
      const text = (await res.text().catch(() => '')).trim();
      out.className = 'bad';
      out.textContent = text || `Album creation failed (${res.status}).`;
      return;
    }
    out.className = 'ok';
    out.textContent = 'Album created.';
    $('creatorCreateAlbumTitle').value = '';
    $('creatorCreateAlbumArtist').value = '';
    $('creatorCreateAlbumGenre').value = '';
    $('creatorCreateAlbumVisibility').value = 'private';
    await loadData();
    switchCreatorCenterTab('albums');
  }

  async function openCreatorCenterTrack(trackID) {
    switchView('creator_center');
    switchCreatorCenterTab('tracks');
    const ok = await loadTrackForEditor(trackID, false);
    if (!ok) {
      alert('Track cannot be loaded.');
      setAdminTarget(null);
    }
  }

  function openCreatorCenterAlbum(albumID) {
    switchView('creator_center');
    switchCreatorCenterTab('albums');
    const album = (state.manageAlbums || []).find((row) => Number(row.id) === Number(albumID));
    if (album) setAlbumTarget(album);
  }

  function renderCreatorCenter() {
    if (!$('viewCreatorCenter')) return;
    renderCreatorCenterSummary();
    renderCreatorAlbums();
    renderCreatorTracks();
    switchCreatorCenterTab(state.creatorCenterTab || 'overview');
  }

  function bindCreatorCenterEvents() {
    document.querySelectorAll('#creatorCenterTabs [data-creator-tab]').forEach((btn) => {
      btn.onclick = () => switchCreatorCenterTab(btn.getAttribute('data-creator-tab'));
    });
    if ($('btnCreatorCenterRefresh')) $('btnCreatorCenterRefresh').onclick = async () => {
      await loadManageData();
      renderCreatorCenter();
    };
    if ($('btnCreatorJumpUpload')) $('btnCreatorJumpUpload').onclick = () => switchCreatorCenterTab('uploads');
    if ($('btnCreatorJumpAlbums')) $('btnCreatorJumpAlbums').onclick = () => switchCreatorCenterTab('albums');
    if ($('btnCreatorJumpTracks')) $('btnCreatorJumpTracks').onclick = () => switchCreatorCenterTab('tracks');
    if ($('btnCreatorCreateAlbum')) $('btnCreatorCreateAlbum').onclick = createCreatorAlbum;
  }

  Object.assign(ns, {
    switchCreatorCenterTab,
    renderCreatorCenter,
    syncCreatorCenterSelection,
    bindCreatorCenterEvents,
    openCreatorCenterTrack,
    openCreatorCenterAlbum,
    createCreatorAlbum
  });
})(window.HexSonic = window.HexSonic || {});
