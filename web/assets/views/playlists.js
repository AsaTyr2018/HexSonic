(function(ns) {
const { state, $, headers, apiFetch, escapeHtml } = ns;
const canAdmin = (...args) => ns.canAdmin(...args);
const isFavorite = (...args) => ns.isFavorite(...args);
const startTrackById = (...args) => ns.startTrackById(...args);
const openTrackDetail = (...args) => ns.openTrackDetail(...args);
const openUserProfile = (...args) => ns.openUserProfile(...args);
const renderLyricIndicators = (...args) => ns.renderLyricIndicators(...args);
const renderRateableStars = (...args) => ns.renderRateableStars(...args);
const shortVisibility = (...args) => ns.shortVisibility(...args);

    function canCreatePlaylists() {
      return !!state.me;
    }

    function setPlaylistDockOpen(open) {
      state.playlistDockOpen = !!open;
      localStorage.setItem('hex_playlist_dock_open', state.playlistDockOpen ? '1' : '0');
      const dock = $('playlistDock');
      const toggle = $('btnPlaylistDockToggle');
      dock.classList.toggle('open', state.playlistDockOpen);
      toggle.classList.toggle('active', state.playlistDockOpen);
      toggle.textContent = state.playlistDockOpen ? 'Playlists <<' : 'Playlists';
    }

    function ownPlaylists() {
      if (!state.me) return [];
      return state.playlists.filter((p) => p.owner_sub === state.me.subject || canAdmin());
    }

    function canManageSelectedPlaylist() {
      if (!state.me || !state.selectedPlaylistId) return false;
      if (canAdmin()) return true;
      const p = state.playlists.find((x) => Number(x.id) === Number(state.selectedPlaylistId));
      return !!(p && p.owner_sub === state.me.subject);
    }

    async function loadPlaylists() {
      const res = await apiFetch('/api/v1/playlists', { headers: headers() }, false);
      if (!res.ok) {
        state.playlists = [];
        renderPlaylists();
        if (!$('playlistPickerModalOverlay').classList.contains('hidden')) {
          renderPlaylistPickerModal();
        }
        return;
      }
      const json = await res.json();
      state.playlists = json.playlists || [];
      renderPlaylists();
      if (!$('playlistPickerModalOverlay').classList.contains('hidden')) {
        renderPlaylistPickerModal();
      }
      if (state.selectedPlaylistId) {
        const still = state.playlists.find((p) => Number(p.id) === Number(state.selectedPlaylistId));
        if (still) await selectPlaylist(still.id);
        else clearSelectedPlaylist();
      }
    }

    function clearSelectedPlaylist() {
      state.selectedPlaylistId = 0;
      state.selectedPlaylistTracks = [];
      $('playlistTitle').textContent = 'No playlist selected';
      $('playlistMeta').textContent = '-';
      $('btnPlaylistDelete').disabled = true;
      $('playlistTracksBody').innerHTML = '<tr><td colspan="6" class="muted">Select a playlist.</td></tr>';
      renderPlaylists();
      renderPlaylistDock();
    }

    function renderPlaylists() {
      $('btnCreatePlaylist').disabled = !canCreatePlaylists();
      $('playlistName').disabled = !canCreatePlaylists();
      $('playlistVisibility').disabled = !canCreatePlaylists();

      const root = $('playlistList');
      root.innerHTML = '';
      for (const p of state.playlists) {
        const div = document.createElement('div');
        div.className = `playlist-item ${Number(p.id) === Number(state.selectedPlaylistId) ? 'active' : ''}`;
        div.innerHTML = `
          <div style="display:flex; justify-content:space-between; gap:8px; align-items:center;">
            <div style="font-weight:600;">${p.name}</div>
            ${state.me ? `<button class="btn slim" data-fav-playlist="${p.id}">${isFavorite('playlist', p.id) ? '★' : '☆'}</button>` : ''}
          </div>
          <div class="muted" style="font-size:12px; margin-top:2px;">${p.visibility} · tracks ${p.track_count || 0}</div>
        `;
        div.onclick = () => selectPlaylist(p.id);
        const favBtn = div.querySelector('[data-fav-playlist]');
        if (favBtn) favBtn.onclick = async (e) => {
          e.stopPropagation();
          await toggleFavorite('playlist', p.id);
        };
        root.appendChild(div);
      }
      if (!state.playlists.length) {
        root.innerHTML = '<div class="muted">No playlists available.</div>';
      }
      renderPlaylistDock();
    }

    async function selectPlaylist(playlistID) {
      const metaRes = await apiFetch(`/api/v1/playlists/${playlistID}`, { headers: headers() }, false);
      if (!metaRes.ok) {
        clearSelectedPlaylist();
        return;
      }
      const meta = await metaRes.json();
      const tracksRes = await apiFetch(`/api/v1/playlists/${playlistID}/tracks`, { headers: headers() }, false);
      if (!tracksRes.ok) {
        clearSelectedPlaylist();
        return;
      }
      const tj = await tracksRes.json();
      state.selectedPlaylistId = Number(playlistID);
      state.selectedPlaylistTracks = tj.tracks || [];
      $('playlistTitle').textContent = meta.name || 'Playlist';
      $('playlistMeta').textContent = `${meta.visibility || '-'} · tracks ${meta.track_count || state.selectedPlaylistTracks.length || 0}`;
      $('btnPlaylistDelete').disabled = !canManageSelectedPlaylist();
      renderPlaylistTracks();
      renderPlaylists();
      renderPlaylistDock();
    }

    function renderPlaylistTracks() {
      const body = $('playlistTracksBody');
      body.innerHTML = '';
      const canManage = canManageSelectedPlaylist();
      state.selectedPlaylistTracks.forEach((t, idx) => {
        const active = isActiveTrack(t.id);
        const tr = document.createElement('tr');
        tr.className = `clickable-row ${active ? 'active' : ''}`.trim();
        tr.innerHTML = `
          <td>${active ? '&gt; ' : ''}${idx + 1}</td>
          <td>${t.title}</td>
          <td>${t.artist}<div class="uploader-inline">by <a class="uploader-link" data-uploader="${t.owner_sub || ''}">${t.uploader_name || t.owner_sub || '-'}</a></div></td>
          <td>${t.album}</td>
          <td>${fmt(Math.round(t.duration_seconds || 0))}</td>
          <td>
            ${canManage ? `<button class="btn" data-remove="${t.id}">Remove</button>` : ''}
          </td>
        `;
        tr.onclick = (ev) => {
          const target = ev.target instanceof Element ? ev.target : null;
          if (target && target.closest('button, a, input, select, textarea')) return;
          startTrackById(t.id, state.selectedPlaylistTracks.map((x) => ({
            id: x.id, title: x.title, artist: x.artist, album: x.album, duration: Math.round(x.duration_seconds || 0), visibility: x.visibility || 'private', genre: x.genre || ''
          })));
        };
        const rmBtn = tr.querySelector('[data-remove]');
        if (rmBtn) rmBtn.onclick = () => removeTrackFromPlaylist(t.id);
        body.appendChild(tr);
      });
      if (!state.selectedPlaylistTracks.length) {
        body.innerHTML = '<tr><td colspan="6" class="muted">No tracks in playlist.</td></tr>';
      }
      renderPlaylistDock();
    }

    function renderPlaylistDock() {
      const hasAny = state.playlists.length > 0;
      const show = hasAny || canCreatePlaylists();
      $('btnPlaylistDockToggle').classList.toggle('hidden', !show);
      $('playlistDock').classList.toggle('hidden', !show);
      if (!show) return;
      setPlaylistDockOpen(state.playlistDockOpen);

      const select = $('playlistDockSelect');
      select.innerHTML = '';
      if (!hasAny) {
        select.innerHTML = '<option value="">No playlists</option>';
        select.disabled = true;
        $('playlistDockMeta').textContent = 'No playlist selected.';
        $('playlistDockBody').innerHTML = '<div class="muted">No playlists available.</div>';
        return;
      }

      select.disabled = false;
      if (!state.selectedPlaylistId) {
        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = 'Select playlist...';
        placeholder.selected = true;
        select.appendChild(placeholder);
      }
      state.playlists.forEach((p) => {
        const opt = document.createElement('option');
        opt.value = String(p.id);
        opt.textContent = `${p.name} (${p.track_count || 0})`;
        select.appendChild(opt);
      });
      if (state.selectedPlaylistId) {
        select.value = String(state.selectedPlaylistId);
      }

      const active = state.playlists.find((p) => Number(p.id) === Number(state.selectedPlaylistId));
      if (!active) {
        $('playlistDockMeta').textContent = 'Select a playlist.';
        $('playlistDockBody').innerHTML = '<div class="muted">Select a playlist to preview tracks.</div>';
        return;
      }

      $('playlistDockMeta').textContent = `${active.visibility || '-'} · tracks ${active.track_count || state.selectedPlaylistTracks.length || 0}`;
      const body = $('playlistDockBody');
      body.innerHTML = '';
      const canManage = canManageSelectedPlaylist();
      const rows = state.selectedPlaylistTracks.slice(0, 60);
      rows.forEach((t, idx) => {
        const active = isActiveTrack(t.id);
        const item = document.createElement('div');
        item.className = `playlist-dock-track ${active ? 'active' : ''}`.trim();
        item.innerHTML = `
          <div>
            <div class="playlist-dock-track-title">${active ? '&gt; ' : ''}${idx + 1}. ${t.title || '-'}</div>
            <div class="playlist-dock-track-meta">${t.artist || '-'} · ${t.album || '-'} · ${fmt(Math.round(t.duration_seconds || 0))}</div>
          </div>
          <div class="playlist-dock-track-actions">
            ${canManage ? `<button class="btn slim danger" data-remove="${t.id}">X</button>` : ''}
          </div>
        `;
        item.onclick = (ev) => {
          const target = ev.target instanceof Element ? ev.target : null;
          if (target && target.closest('button, a, input, select, textarea')) return;
          startTrackById(t.id, state.selectedPlaylistTracks.map((x) => ({
            id: x.id, title: x.title, artist: x.artist, album: x.album, duration: Math.round(x.duration_seconds || 0), visibility: x.visibility || 'private', genre: x.genre || ''
          })));
        };
        const rmBtn = item.querySelector('[data-remove]');
        if (rmBtn) rmBtn.onclick = (ev) => {
          ev.stopPropagation();
          removeTrackFromPlaylist(t.id);
        };
        body.appendChild(item);
      });
      if (!rows.length) {
        body.innerHTML = '<div class="muted">No tracks in playlist.</div>';
      }
    }

    async function createPlaylist() {
      if (!canCreatePlaylists()) {
        alert('Login required.');
        return;
      }
      const name = $('playlistName').value.trim();
      if (!name) {
        alert('Playlist name required.');
        return;
      }
      const res = await apiFetch('/api/v1/playlists', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ name, visibility: $('playlistVisibility').value })
      });
      if (!res.ok) {
        alert(`Create playlist failed (${res.status}).`);
        return;
      }
      $('playlistName').value = '';
      await loadPlaylists();
      switchView('playlists');
    }

    function closePlaylistPickerModal() {
      state.playlistPickerTrackIDs = [];
      $('playlistPickerModalOverlay').classList.add('hidden');
      $('playlistPickerMeta').textContent = 'Select a playlist for the pending tracks.';
      $('playlistPickerName').value = '';
      $('playlistPickerVisibility').value = 'private';
      $('playlistPickerList').innerHTML = '';
    }

    function renderPlaylistPickerModal() {
      const listRoot = $('playlistPickerList');
      const mine = ownPlaylists();
      listRoot.innerHTML = '';
      if (!mine.length) {
        listRoot.innerHTML = '<div class="muted">No playlists yet. Create one on the left.</div>';
        return;
      }
      mine.forEach((p) => {
        const item = document.createElement('div');
        item.className = 'playlist-picker-item';
        item.innerHTML = `
          <div>
            <div class="playlist-picker-item-title">${escapeHtml(p.name || 'Playlist')}</div>
            <div class="playlist-picker-item-meta">${escapeHtml(p.visibility || 'private')} · ${Number(p.track_count || 0)} tracks</div>
          </div>
          <button class="btn" data-pick-playlist="${p.id}">Add here</button>
        `;
        const pickBtn = item.querySelector('[data-pick-playlist]');
        if (pickBtn) {
          pickBtn.onclick = async () => {
            await addTrackIDsToPlaylist(state.playlistPickerTrackIDs.slice(), Number(p.id));
          };
        }
        listRoot.appendChild(item);
      });
    }

    async function openPlaylistPickerModal(trackIDs) {
      const ids = (trackIDs || []).map((id) => String(id || '').trim()).filter(Boolean);
      if (!ids.length) return;
      if (!canCreatePlaylists()) {
        alert('Login required.');
        return;
      }
      state.playlistPickerTrackIDs = ids;
      $('playlistPickerMeta').textContent = `${ids.length} track${ids.length === 1 ? '' : 's'} ready to add.`;
      $('playlistPickerModalOverlay').classList.remove('hidden');
      await loadPlaylists();
      renderPlaylistPickerModal();
    }

    async function addTrackIDsToPlaylist(trackIDs, playlistID) {
      const ids = (trackIDs || []).map((id) => String(id || '').trim()).filter(Boolean);
      if (!ids.length) return;
      if (!Number.isFinite(playlistID) || playlistID <= 0) {
        alert('Invalid playlist id.');
        return;
      }
      for (const trackID of ids) {
        const res = await apiFetch(`/api/v1/playlists/${playlistID}/tracks`, {
          method: 'POST',
          headers: headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ track_id: trackID })
        });
        if (!res.ok) {
          alert(`Add track failed (${res.status}).`);
          return;
        }
      }
      await loadPlaylists();
      if (Number(state.selectedPlaylistId) === Number(playlistID)) {
        await selectPlaylist(playlistID);
      } else {
        renderPlaylistPickerModal();
      }
      closePlaylistPickerModal();
    }

    async function createPlaylistFromPicker() {
      if (!canCreatePlaylists()) {
        alert('Login required.');
        return;
      }
      const ids = state.playlistPickerTrackIDs.slice();
      if (!ids.length) return;
      const name = $('playlistPickerName').value.trim();
      if (!name) {
        alert('Playlist name required.');
        return;
      }
      const res = await apiFetch('/api/v1/playlists', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ name, visibility: $('playlistPickerVisibility').value })
      });
      if (!res.ok) {
        alert(`Create playlist failed (${res.status}).`);
        return;
      }
      const json = await res.json();
      const playlistID = Number(json.id || json.playlist_id || 0);
      if (!Number.isFinite(playlistID) || playlistID <= 0) {
        await loadPlaylists();
        const created = ownPlaylists().find((p) => String(p.name || '').trim() === name);
        if (!created) {
          alert('Playlist created, but the new playlist id could not be resolved.');
          return;
        }
        await addTrackIDsToPlaylist(ids, Number(created.id));
        return;
      }
      await addTrackIDsToPlaylist(ids, playlistID);
    }

    async function deleteSelectedPlaylist() {
      if (!state.selectedPlaylistId) return;
      const ok = confirm('Delete selected playlist?');
      if (!ok) return;
      const res = await apiFetch(`/api/v1/playlists/${state.selectedPlaylistId}`, {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Delete playlist failed (${res.status}).`);
        return;
      }
      clearSelectedPlaylist();
      await loadPlaylists();
    }

    async function addTrackToPlaylist(trackID) {
      await openPlaylistPickerModal([trackID]);
    }

    async function addTracksToPlaylist(trackIDs) {
      await openPlaylistPickerModal(trackIDs);
    }

    async function removeTrackFromPlaylist(trackID) {
      if (!state.selectedPlaylistId) return;
      const res = await apiFetch(`/api/v1/playlists/${state.selectedPlaylistId}/tracks/${encodeURIComponent(trackID)}`, {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Remove failed (${res.status}).`);
        return;
      }
      await selectPlaylist(state.selectedPlaylistId);
      await loadPlaylists();
    }

Object.assign(window.HexSonic, {
      canCreatePlaylists,
      setPlaylistDockOpen,
      ownPlaylists,
      canManageSelectedPlaylist,
      loadPlaylists,
      clearSelectedPlaylist,
      renderPlaylists,
      selectPlaylist,
      renderPlaylistTracks,
      renderPlaylistDock,
      createPlaylist,
      closePlaylistPickerModal,
      renderPlaylistPickerModal,
      openPlaylistPickerModal,
      addTrackIDsToPlaylist,
      createPlaylistFromPicker,
      deleteSelectedPlaylist,
      addTrackToPlaylist,
      addTracksToPlaylist,
      removeTrackFromPlaylist
});
})(window.HexSonic = window.HexSonic || {});
