(function(ns) {
const { state, $, headers, apiFetch, escapeHtml } = ns;
const canAdmin = (...args) => ns.canAdmin(...args);
const canManage = (...args) => ns.canManage(...args);
const loadData = (...args) => ns.loadData(...args);
const syncPublicUI = (...args) => ns.syncPublicUI(...args);

    function setAdminTarget(track) {
      state.selectedAdminTrackId = track ? track.id : '';
      if (!track) {
        $('adminTargetLabel').textContent = 'No track selected.';
        $('trackEditTitle').value = '';
        $('trackEditArtist').value = '';
        $('trackEditAlbum').value = '';
        $('trackEditGenre').value = '';
        $('trackEditLyrics').value = '';
        $('trackEditLyricsPlain').value = '';
        return;
      }
      $('adminTargetLabel').textContent = `Selected: ${track.title} by ${track.artist} (${track.id})`;
      $('adminVisibility').value = track.visibility || 'public';
      $('trackEditTitle').value = track.title || '';
      $('trackEditArtist').value = track.artist || '';
      $('trackEditAlbum').value = track.album || '';
      $('trackEditGenre').value = track.genre || '';
      $('trackEditLyricsPlain').value = '';
      loadTrackDetails(track.id);
      if (typeof ns.syncCreatorCenterSelection === 'function') ns.syncCreatorCenterSelection();
    }

    function applyTrackManageMode(view) {
      const isAdminMode = view === 'admin_track_manage' && canAdmin();
      $('trackManageTitle').textContent = isAdminMode ? 'Admin Track Management' : 'User Track Management';
      $('trackManageHint').textContent = isAdminMode
        ? 'Admin: Edit any track. Use the Songs view (M) or load directly via Track ID.'
        : 'User: Edit only your own tracks. Open from Songs view via button M.';
      $('trackManageAdminTools').classList.toggle('hidden', !isAdminMode);
    }

    async function loadTrackForEditor(trackID, showAlert = true) {
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(trackID)}`, { headers: headers() });
      if (!res.ok) {
        if (showAlert) alert(`Track load failed (${res.status}).`);
        return false;
      }
      const json = await res.json();
      setAdminTarget({
        id: json.id,
        title: json.title || '',
        artist: json.artist || '',
        album: json.album || '',
        genre: json.genre || '',
        visibility: json.visibility || 'private'
      });
      return true;
    }

    function setAlbumTarget(album) {
      state.selectedManageAlbumId = album ? Number(album.id) : 0;
      if (!album) {
        $('albumTargetLabel').textContent = 'No album selected.';
        $('albumEditTitle').value = '';
        $('albumEditArtist').value = '';
        $('albumEditGenre').value = '';
        $('albumEditVisibility').value = 'private';
        return;
      }
      $('albumTargetLabel').textContent = `Selected album: ${album.title} (${album.id})`;
      $('albumEditTitle').value = album.title || '';
      $('albumEditArtist').value = album.artist || '';
      $('albumEditGenre').value = album.genre || '';
      $('albumEditVisibility').value = album.visibility || 'private';
      if (typeof ns.syncCreatorCenterSelection === 'function') ns.syncCreatorCenterSelection();
    }


    function renderManageAlbums() {
      const body = $('manageAlbumsBody');
      if (!body) return;
      body.innerHTML = '';
      if (!canManage()) {
        body.innerHTML = '<tr><td colspan="7" class="muted">Login required.</td></tr>';
        return;
      }
      state.manageAlbums.forEach((a, idx) => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
          <td>${idx + 1}</td>
          <td>${a.title}</td>
          <td>${a.artist}</td>
          <td>${a.genre || '-'}</td>
          <td>${a.track_count || 0}</td>
          <td>${a.visibility}</td>
          <td><button class="btn" data-album="${a.id}">Select</button></td>
        `;
        tr.querySelector('button').onclick = () => setAlbumTarget(a);
        body.appendChild(tr);
      });
      if (!state.manageAlbums.length) {
        body.innerHTML = '<tr><td colspan="7" class="muted">No albums available.</td></tr>';
      }
    }

    async function loadTrackDetails(trackID) {
      if (!canManage()) return;
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(trackID)}`, { headers: headers() });
      if (!res.ok) return;
      const json = await res.json();
      $('trackEditLyrics').value = json.lyrics_srt || '';
      $('trackEditLyricsPlain').value = json.lyrics_txt || '';
      $('trackEditGenre').value = json.genre || '';
    }

    async function loadManageData() {
      if (!canManage()) {
        state.manageTracks = [];
        state.manageAlbums = [];
        renderManageAlbums();
        setAdminTarget(null);
        setAlbumTarget(null);
        if (typeof ns.renderCreatorCenter === 'function') ns.renderCreatorCenter();
        return;
      }
      const tRes = await apiFetch('/api/v1/manage/tracks', { headers: headers() });
      if (tRes.ok) {
        const tj = await tRes.json();
        state.manageTracks = tj.tracks || [];
      } else {
        state.manageTracks = [];
      }
      const aRes = await apiFetch('/api/v1/manage/albums', { headers: headers() });
      if (aRes.ok) {
        const aj = await aRes.json();
        state.manageAlbums = aj.albums || [];
      } else {
        state.manageAlbums = [];
      }
      renderManageAlbums();
      if (state.selectedAdminTrackId) {
        await loadTrackForEditor(state.selectedAdminTrackId, false);
      }
      if (state.selectedManageAlbumId) {
        const a = state.manageAlbums.find((x) => Number(x.id) === Number(state.selectedManageAlbumId));
        if (a) setAlbumTarget(a);
      }
      if (typeof ns.renderCreatorCenter === 'function') ns.renderCreatorCenter();
    }

    function renderAdminUsers() {
      const body = $('adminUsersBody');
      body.innerHTML = '';
      if (!canAdmin()) {
        body.innerHTML = '<tr><td colspan="5" class="muted">Admin role required.</td></tr>';
        return;
      }
      state.adminUsers.forEach((u) => {
        const tr = document.createElement('tr');
        const roles = Array.isArray(u.roles) ? u.roles.join(', ') : '';
        const top = topRoleFromRoles(u.roles || []);
        tr.innerHTML = `
          <td>${u.username || '-'}</td>
          <td>${u.email || '-'}</td>
          <td>
            <select data-top-role="${u.id}">
              <option value="user" ${top === 'user' ? 'selected' : ''}>User</option>
              <option value="creator" ${top === 'creator' ? 'selected' : ''}>Creator</option>
              <option value="moderator" ${top === 'moderator' ? 'selected' : ''}>Moderator</option>
              <option value="admin" ${top === 'admin' ? 'selected' : ''}>Admin</option>
            </select>
            <label class="muted" style="display:block; margin-top:4px; font-size:11px;">
              <input type="checkbox" data-creator-badge="${u.id}" ${u.creator_badge ? 'checked' : ''} />
              Creator badge (upload permission)
            </label>
            <div class="muted" style="font-size:11px; margin-top:3px;">${roles || '-'}</div>
          </td>
          <td>${u.enabled ? 'yes' : 'no'}</td>
          <td>
            <button class="btn" data-toggle="${u.id}">${u.enabled ? 'Disable' : 'Enable'}</button>
            <button class="btn" data-rolesave="${u.id}">Set Role</button>
            <button class="btn danger" data-delete="${u.id}">Delete</button>
          </td>
        `;
        const toggleBtn = tr.querySelector('[data-toggle]');
        if (toggleBtn) toggleBtn.onclick = () => adminToggleUserEnabled(u);
        const roleSaveBtn = tr.querySelector('[data-rolesave]');
        if (roleSaveBtn) roleSaveBtn.onclick = () => adminSaveUserRoles(u, tr);
        const deleteBtn = tr.querySelector('[data-delete]');
        if (deleteBtn) deleteBtn.onclick = () => adminDeleteUserByID(u);
        body.appendChild(tr);
      });
      if (!state.adminUsers.length) {
        body.innerHTML = '<tr><td colspan="5" class="muted">No users found.</td></tr>';
      }
    }

    async function loadAdminUsers() {
      if (!canAdmin()) {
        state.adminUsers = [];
        renderAdminUsers();
        return;
      }
      const q = $('adminUserSearch').value.trim();
      const suffix = q ? `?search=${encodeURIComponent(q)}` : '';
      const res = await apiFetch(`/api/v1/admin/users${suffix}`, { headers: headers() });
      if (!res.ok) {
        state.adminUsers = [];
        renderAdminUsers();
        return;
      }
      const json = await res.json();
      state.adminUsers = json.users || [];
      renderAdminUsers();
    }

    async function adminToggleUserEnabled(user) {
      const res = await apiFetch(`/api/v1/admin/users/${encodeURIComponent(user.id)}`, {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ enabled: !user.enabled })
      });
      if (!res.ok) {
        alert(`User update failed (${res.status}).`);
        return;
      }
      await loadAdminUsers();
    }

    async function adminSaveUserRoles(user, rowEl) {
      const input = rowEl.querySelector(`[data-top-role="${user.id}"]`);
      if (!input) return;
      const topRole = String(input.value || '').trim().toLowerCase();
      const creatorBadgeInput = rowEl.querySelector(`[data-creator-badge="${user.id}"]`);
      const creatorBadge = !!(creatorBadgeInput && creatorBadgeInput.checked);
      const res = await apiFetch(`/api/v1/admin/users/${encodeURIComponent(user.id)}`, {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ top_role: topRole, creator_badge: creatorBadge })
      });
      if (!res.ok) {
        alert(`Role update failed (${res.status}).`);
        return;
      }
      await loadAdminUsers();
    }

    function topRoleFromRoles(roles) {
      const set = new Set((roles || []).map((r) => String(r).toLowerCase()));
      if (set.has('admin')) return 'admin';
      if (set.has('moderator')) return 'moderator';
      if (set.has('member')) return 'creator';
      if (set.has('creator')) return 'creator';
      return 'user';
    }

    async function adminDeleteUserByID(user) {
      const ok = confirm(`Delete user ${user.username || user.id}?`);
      if (!ok) return;
      const res = await apiFetch(`/api/v1/admin/users/${encodeURIComponent(user.id)}`, {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Delete user failed (${res.status}).`);
        return;
      }
      await loadAdminUsers();
    }

    async function adminDeleteFilteredUsers() {
      if (!canAdmin()) return;
      if (!state.adminUsers.length) return;
      const names = state.adminUsers.map((u) => u.username).slice(0, 8).join(', ');
      const ok = confirm(`Delete ${state.adminUsers.length} listed users? Sample: ${names}`);
      if (!ok) return;
      for (const u of state.adminUsers) {
        const res = await apiFetch(`/api/v1/admin/users/${encodeURIComponent(u.id)}`, {
          method: 'DELETE',
          headers: headers()
        });
        if (!res.ok) {
          alert(`Delete failed for ${u.username} (${res.status}).`);
          break;
        }
      }
      await loadAdminUsers();
    }

    async function adminCreateInvite() {
      if (!canAdmin()) return;
      const ttl = Number($('adminInviteTTL').value || '1440');
      const res = await apiFetch('/api/v1/admin/invites', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ ttl_minutes: ttl })
      });
      if (!res.ok) {
        alert(`Invite creation failed (${res.status}).`);
        return;
      }
      const json = await res.json();
      $('adminInviteLink').value = json.invite_link || '';
      if ($('adminInviteLink').value) {
        try { await navigator.clipboard.writeText($('adminInviteLink').value); } catch (_) {}
      }
    }

    async function loadAdminInvites() {
      if (!canAdmin()) return;
      const res = await apiFetch('/api/v1/admin/invites', { headers: headers() });
      if (!res.ok) {
        state.adminInvites = [];
        renderAdminInvites();
        return;
      }
      const json = await res.json();
      state.adminInvites = json.invites || [];
      renderAdminInvites();
    }

    function renderAdminInvites() {
      const body = $('inviteModalBody');
      body.innerHTML = '';
      state.adminInvites.forEach((inv) => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
          <td>${inv.id}</td>
          <td><input value="${inv.invite_link || ''}" readonly style="width:100%;" /></td>
          <td>${(inv.expires_at || '').replace('T', ' ').slice(0, 19)}</td>
          <td>${inv.used_by || '-'}</td>
          <td><button class="btn danger" data-del-invite="${inv.id}">Delete</button></td>
        `;
        const del = tr.querySelector('[data-del-invite]');
        if (del) del.onclick = () => adminDeleteInviteByID(inv.id);
        const linkInput = tr.querySelector('input');
        if (linkInput) {
          linkInput.onclick = async () => {
            linkInput.select();
            try { await navigator.clipboard.writeText(linkInput.value); } catch (_) {}
          };
        }
        body.appendChild(tr);
      });
      if (!state.adminInvites.length) {
        body.innerHTML = '<tr><td colspan="5" class="muted">No invites available.</td></tr>';
      }
    }

    async function adminDeleteInviteByID(id) {
      const ok = confirm(`Delete invite ${id}?`);
      if (!ok) return;
      const res = await apiFetch(`/api/v1/admin/invites/${encodeURIComponent(id)}`, {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Invite delete failed (${res.status}).`);
        return;
      }
      await loadAdminInvites();
    }

    async function signupWithInvitePage() {
      const username = $('inviteRegUser').value.trim();
      const email = $('inviteRegEmail').value.trim();
      const password = $('inviteRegPass').value;
      if (!state.inviteToken) {
        alert('Invite token missing.');
        return;
      }
      if (!username || !password) {
        alert('Username and password required.');
        return;
      }
      const res = await fetch('/api/v1/auth/signup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, email, password, invite_token: state.inviteToken })
      });
      if (!res.ok) {
        let msg = 'Registration failed.';
        try { msg = await res.text(); } catch (_) {}
        alert(msg || 'Registration failed.');
        return;
      }
      state.inviteToken = '';
      $('inviteRegPass').value = '';
      $('inviteRegEmail').value = '';
      $('inviteRegisterMeta').textContent = 'Registration successful. You can now log in.';
      state.invitePageMode = false;
      applyInvitePageMode();
      switchView('albums');
      $('authDrawer').classList.remove('hidden');
    }

    function esc(s) {
      return String(s ?? '').replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
    }

    function statusPill(ok) {
      return `<span class="pill ${ok ? 'ok' : 'bad'}">${ok ? 'Online' : 'Offline'}</span>`;
    }

    function adminProxyURL(base) {
      return String(base || '').trim();
    }

    async function openAdminMonitor(target, url) {
      if (!canAdmin()) {
        alert('Admin role required.');
        return;
      }
      const res = await apiFetch('/api/v1/admin/proxy-session', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ target })
      });
      if (!res.ok) {
        alert(`Proxy session failed (${res.status}).`);
        return;
      }
      const j = await res.json();
      const openURL = (j && j.url) ? j.url : (url || '/');
      window.open(openURL, '_blank', 'noopener');
    }

    async function refreshAdminProxySessions() {
      if (!canAdmin()) return;
      for (const target of ['prometheus', 'grafana']) {
        try {
          await apiFetch('/api/v1/admin/proxy-session', {
            method: 'POST',
            headers: headers({ 'Content-Type': 'application/json' }),
            body: JSON.stringify({ target })
          }, false);
        } catch (_) {}
      }
    }

    function renderAdminSystemOverview(data) {
      const stats = data && data.stats ? data.stats : {};
      const jobs = data && data.jobs_by_status ? data.jobs_by_status : {};
      const playback = data && data.playback ? data.playback : {};
      $('adminSystemOverviewStatus').textContent = `Updated: ${(data && data.server_time) ? data.server_time.replace('T', ' ').replace('Z', ' UTC') : '-'}`;
      $('adminMetricsGrid').innerHTML = `
        <div class="metric-card"><div class="metric-label">Users</div><div class="metric-value">${Number(data?.users || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Tracks</div><div class="metric-value">${Number(stats.tracks || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Albums</div><div class="metric-value">${Number(stats.albums || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Queued Jobs</div><div class="metric-value">${Number(jobs.queued || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Total Plays</div><div class="metric-value">${Number(playback.plays_total || 0)}</div></div>
        <div class="metric-card"><div class="metric-label">Unique Listeners</div><div class="metric-value">${Number(playback.unique_listeners || 0)}</div></div>
      `;

      const mon = data && data.monitoring ? data.monitoring : {};
      const valkey = mon.valkey || {};
      const prom = mon.prometheus || {};
      const grafana = mon.grafana || {};
      const keycloak = mon.keycloak || {};
      const keycloakOpen = String(keycloak.public_url || keycloak.issuer || '').trim();
      const btnOpenKeycloak = $('btnOpenKeycloak');
      if (keycloakOpen) {
        btnOpenKeycloak.href = keycloakOpen;
        btnOpenKeycloak.classList.remove('hidden');
      } else {
        btnOpenKeycloak.href = '#';
        btnOpenKeycloak.classList.add('hidden');
      }
      $('adminServiceGrid').innerHTML = `
        <div class="service-card">
          <div class="service-head"><b>Valkey</b>${statusPill(!!valkey.reachable)}</div>
          <div class="service-kv">
            <div>Address</div><div>${esc(valkey.addr || '-')}</div>
            <div>Version</div><div>${esc(valkey.version || '-')}</div>
            <div>Latency</div><div>${esc(valkey.latency_ms ?? '-')} ms</div>
            <div>Clients</div><div>${esc(valkey.connected_clients || '-')}</div>
            <div>Memory</div><div>${esc(valkey.used_memory_human || '-')}</div>
            <div>DB0</div><div>${esc(valkey.db0 || '-')}</div>
          </div>
          ${valkey.error ? `<div class="muted">Error: ${esc(valkey.error)}</div>` : ''}
        </div>
        <div class="service-card">
          <div class="service-head"><b>Prometheus</b>${statusPill(!!prom.reachable)}</div>
          <div class="service-kv">
            <div>Internal URL</div><div>${esc(prom.url || '-')}</div>
            <div>Latency</div><div>${esc(prom.latency_ms ?? '-')} ms</div>
            <div>Targets Up</div><div>${esc(prom.targets_up ?? '-')}</div>
            <div>Total Targets</div><div>${esc(prom.targets_total ?? '-')}</div>
          </div>
          <div class="service-links">
            ${prom.public_url ? `<a href="${esc(adminProxyURL(prom.public_url))}" data-monitor-target="prometheus">Open Prometheus</a>` : ''}
          </div>
          ${prom.error ? `<div class="muted">Error: ${esc(prom.error)}</div>` : ''}
        </div>
        <div class="service-card">
          <div class="service-head"><b>Grafana</b>${statusPill(!!grafana.reachable)}</div>
          <div class="service-kv">
            <div>Internal URL</div><div>${esc(grafana.url || '-')}</div>
            <div>Version</div><div>${esc(grafana.version || '-')}</div>
            <div>Database</div><div>${esc(grafana.database || '-')}</div>
            <div>Latency</div><div>${esc(grafana.latency_ms ?? '-')} ms</div>
          </div>
          <div class="service-links">
            ${grafana.public_url ? `<a href="${esc(adminProxyURL(grafana.public_url))}" data-monitor-target="grafana">Open Grafana</a>` : ''}
          </div>
          ${grafana.error ? `<div class="muted">Error: ${esc(grafana.error)}</div>` : ''}
        </div>
        <div class="service-card">
          <div class="service-head"><b>Keycloak</b>${statusPill(!!keycloak.reachable)}</div>
          <div class="service-kv">
            <div>Issuer</div><div>${esc(keycloak.issuer || '-')}</div>
            <div>Discovered</div><div>${esc(keycloak.discovered_issuer || '-')}</div>
            <div>Latency</div><div>${esc(keycloak.latency_ms ?? '-')} ms</div>
            <div>Admin API</div><div>${keycloak.admin_api ? 'OK' : 'Unavailable'}</div>
          </div>
          <div class="service-links">
            ${keycloak.public_url ? `<a href="${esc(keycloak.public_url)}" target="_blank" rel="noopener">Open Keycloak</a>` : ''}
          </div>
          ${keycloak.error ? `<div class="muted">Error: ${esc(keycloak.error)}</div>` : ''}
          ${keycloak.admin_error ? `<div class="muted">Admin: ${esc(keycloak.admin_error)}</div>` : ''}
        </div>
      `;
      const topTracks = Array.isArray(playback.top_tracks) ? playback.top_tracks : [];
      const topAlbums = Array.isArray(playback.top_albums) ? playback.top_albums : [];
      const renderRows = (rows, type) => {
        if (!rows.length) return '<tr><td colspan="3" class="muted">No play events yet.</td></tr>';
        return rows.slice(0, 10).map((r, i) => `
          <tr>
            <td>${i + 1}. ${esc(r.title || '-')}</td>
            <td class="muted">${esc(r.artist || '-')}</td>
            <td style="text-align:right;">${Number(r.plays || 0)}</td>
          </tr>
        `).join('');
      };
      $('adminPlaybackGrid').innerHTML = `
        <div class="service-card">
          <div class="service-head"><b>Top Tracks</b><span class="pill">${Number(playback.plays_total || 0)} plays</span></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Track</th><th>Artist</th><th style="text-align:right;">Plays</th></tr></thead>
              <tbody>${renderRows(topTracks, 'track')}</tbody>
            </table>
          </div>
        </div>
        <div class="service-card">
          <div class="service-head"><b>Top Albums</b><span class="pill">${Number(playback.guest_plays || 0)} guest plays</span></div>
          <div class="table-wrap">
            <table>
              <thead><tr><th>Album</th><th>Artist</th><th style="text-align:right;">Plays</th></tr></thead>
              <tbody>${renderRows(topAlbums, 'album')}</tbody>
            </table>
          </div>
        </div>
      `;
      $('adminSystemOverview').textContent = JSON.stringify(data || {}, null, 2);
      $('adminServiceGrid').querySelectorAll('[data-monitor-target]').forEach((a) => {
        a.onclick = async (e) => {
          e.preventDefault();
          const target = a.getAttribute('data-monitor-target') || '';
          const href = a.getAttribute('href') || '/';
          await openAdminMonitor(target, href);
        };
      });
    }

    async function loadAdminSystemOverview() {
      if (!canAdmin()) {
        $('adminSystemOverviewStatus').textContent = 'Admin role required.';
        $('adminMetricsGrid').innerHTML = '';
        $('adminServiceGrid').innerHTML = '';
        $('adminPlaybackGrid').innerHTML = '';
        $('btnOpenKeycloak').classList.add('hidden');
        $('adminSystemOverview').textContent = 'Admin role required.';
        return;
      }
      const [sRes, oRes] = await Promise.all([
        apiFetch('/api/v1/admin/settings', { headers: headers() }),
        apiFetch('/api/v1/admin/system/overview', { headers: headers() })
      ]);
      if (sRes.ok) {
        const sj = await sRes.json();
        state.publicSettings.registration_enabled = !!sj.registration_enabled;
        $('adminJukeboxTrackLimit').value = String(Number(sj.jukebox_max_track_plays_per_hour || 1));
        $('adminJukeboxCreatorLimit').value = String(Number(sj.jukebox_max_creator_tracks_per_hour || 12));
      }
      syncPublicUI();
      $('registrationState').textContent = `Registration: ${state.publicSettings.registration_enabled ? 'enabled' : 'disabled'}`;
      if (!oRes.ok) {
        $('adminSystemOverviewStatus').textContent = 'Failed to load system overview.';
        $('adminMetricsGrid').innerHTML = '';
        $('adminServiceGrid').innerHTML = '';
        $('adminPlaybackGrid').innerHTML = '';
        $('btnOpenKeycloak').classList.add('hidden');
        $('adminSystemOverview').textContent = 'Failed to load system overview.';
        return;
      }
      const json = await oRes.json();
      state.adminSystemOverview = json;
      renderAdminSystemOverview(json);
      refreshAdminProxySessions();
    }

    async function loadAdminAuditLogs() {
      if (!canAdmin()) {
        $('adminLogsBody').innerHTML = '';
        return;
      }
      const source = ($('adminLogSource').value || 'audit').trim().toLowerCase();
      const limit = Number(($('adminLogLimit').value || '200')) || 200;
      const action = encodeURIComponent(($('adminLogAction').value || '').trim());
      const targetType = encodeURIComponent(($('adminLogTargetType').value || '').trim());
      const endpoint = source === 'debug' ? '/api/v1/admin/debug-logs' : '/api/v1/admin/logs';
      const filterKey = source === 'debug' ? 'endpoint' : 'target_type';
      const res = await apiFetch(`${endpoint}?limit=${limit}&action=${action}&${filterKey}=${targetType}`, { headers: headers() });
      if (!res.ok) {
        $('adminLogsBody').innerHTML = `<tr><td colspan="5" class="muted">Failed to load logs (${res.status})</td></tr>`;
        return;
      }
      const json = await res.json();
      const rows = Array.isArray(json.logs) ? json.logs : [];
      if (!rows.length) {
        $('adminLogsBody').innerHTML = `<tr><td colspan="5" class="muted">No log entries.</td></tr>`;
        return;
      }
      $('adminLogsBody').innerHTML = rows.map((row) => {
        const details = row.details ? JSON.stringify(row.details) : '{}';
        const actor = row.actor_sub || '-';
        const target = `${row.target_type || '-'}:${row.target_id || '-'}`;
        return `
          <tr>
            <td class="mono">${escapeHtml((row.created_at || '').replace('T', ' ').replace('Z', ''))}</td>
            <td class="mono">${escapeHtml(row.action || '-')}</td>
            <td class="mono">${escapeHtml(actor)}</td>
            <td class="mono">${escapeHtml(target)}</td>
            <td class="mono" style="max-width:520px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;" title="${escapeHtml(details)}">${escapeHtml(details)}</td>
          </tr>
        `;
      }).join('');
    }

    async function loadAdminDebugToggle() {
      if (!canAdmin()) return;
      const res = await apiFetch('/api/v1/admin/settings', { headers: headers() });
      if (!res.ok) return;
      const json = await res.json();
      $('adminDebugToggle').checked = !!json.debug_logging_enabled;
    }

    async function saveAdminDebugToggle() {
      if (!canAdmin()) return;
      const enabled = !!$('adminDebugToggle').checked;
      const res = await apiFetch('/api/v1/admin/settings', {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ debug_logging_enabled: enabled })
      });
      if (!res.ok) {
        alert(`Debug toggle update failed (${res.status}).`);
        return;
      }
      await loadAdminDebugToggle();
      await loadAdminAuditLogs();
    }

    async function toggleRegistrationSetting() {
      if (!canAdmin()) return;
      const next = !state.publicSettings.registration_enabled;
      const res = await apiFetch('/api/v1/admin/settings', {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ registration_enabled: next })
      });
      if (!res.ok) {
        alert(`Settings update failed (${res.status}).`);
        return;
      }
      await loadPublicSettings();
      await loadAdminSystemOverview();
    }

    async function saveAdminJukeboxSettings() {
      if (!canAdmin()) return;
      const trackLimit = Number($('adminJukeboxTrackLimit').value || '1');
      const creatorLimit = Number($('adminJukeboxCreatorLimit').value || '12');
      const res = await apiFetch('/api/v1/admin/settings', {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          jukebox_max_track_plays_per_hour: trackLimit,
          jukebox_max_creator_tracks_per_hour: creatorLimit
        })
      });
      if (!res.ok) {
        alert(`Jukebox settings update failed (${res.status}).`);
        return;
      }
      await loadAdminSystemOverview();
    }

    function renderAlbumDetail() {
      // Deprecated: album bottom detail strip removed from Albums view.
    }


    async function loadJobs() {
      const body = $('jobsBody');
      body.innerHTML = '';
      if (!canAdmin()) {
        body.innerHTML = '<tr><td colspan="6" class="muted">Admin role required.</td></tr>';
        return;
      }
      const res = await apiFetch('/api/v1/admin/transcode-jobs', { headers: headers() });
      if (!res.ok) {
        body.innerHTML = '<tr><td colspan="6" class="muted">Loading failed.</td></tr>';
        return;
      }

      const data = await res.json();
      const jobs = data.jobs || [];
      jobs.slice(0, 120).forEach((j) => {
        const tr = document.createElement('tr');
        const track = j.track_title || j.track_id || '-';
        const album = j.album_title || '-';
        tr.innerHTML = `<td>${j.id}</td><td>${escapeHtml(track)}</td><td>${escapeHtml(album)}</td><td>${escapeHtml(j.status || '-')}</td><td>${Number(j.attempts || 0)}</td><td class="muted">${escapeHtml(j.error || '')}</td>`;
        body.appendChild(tr);
      });
      if (!jobs.length) {
        body.innerHTML = '<tr><td colspan="6" class="muted">No jobs found.</td></tr>';
      }
    }

    async function adminSetVisibility() {
      if (!canManage()) {
        alert('Login required.');
        return;
      }
      if (!state.selectedAdminTrackId) {
        alert('Select a track first in Track Management.');
        return;
      }
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(state.selectedAdminTrackId)}`, {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          title: $('trackEditTitle').value.trim(),
          artist: $('trackEditArtist').value.trim(),
          album: $('trackEditAlbum').value.trim(),
          genre: $('trackEditGenre').value.trim(),
          visibility: $('adminVisibility').value,
          lyrics_srt: $('trackEditLyrics').value,
          lyrics_txt: $('trackEditLyricsPlain').value
        })
      });
      if (!res.ok) {
        alert(`Update failed (${res.status}).`);
        return;
      }
      await loadData();
      if (state.selectedAdminTrackId) {
        await loadTrackForEditor(state.selectedAdminTrackId, false);
      }
    }

    async function adminDeleteTrack() {
      if (!canManage()) {
        alert('Login required.');
        return;
      }
      if (!state.selectedAdminTrackId) {
        alert('Select a track first in Track Management.');
        return;
      }
      const ok = confirm('Delete selected track permanently?');
      if (!ok) return;

      const target = state.selectedAdminTrackId;
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(target)}`, {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Delete failed (${res.status}).`);
        return;
      }
      setAdminTarget(null);
      await loadData();
    }

    async function uploadTrackCover() {
      if (!state.selectedAdminTrackId) {
        alert('Select a track first in Track Management.');
        return;
      }
      const f = $('trackCoverFile').files[0];
      if (!f) {
        alert('Select a cover image first.');
        return;
      }
      const form = new FormData();
      form.append('cover', f);
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(state.selectedAdminTrackId)}/cover`, {
        method: 'POST',
        headers: headers(),
        body: form
      });
      if (!res.ok) {
        alert(`Cover upload failed (${res.status}).`);
        return;
      }
      $('trackCoverFile').value = '';
      await loadData();
    }

    async function uploadTrackLyrics() {
      if (!state.selectedAdminTrackId) {
        alert('Select a track first in Track Management.');
        return;
      }
      const f = $('trackLyricsFile').files[0];
      if (!f) {
        alert('Select an .srt file first.');
        return;
      }
      const form = new FormData();
      form.append('lyrics', f);
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(state.selectedAdminTrackId)}/lyrics`, {
        method: 'POST',
        headers: headers(),
        body: form
      });
      if (!res.ok) {
        alert(`Lyrics upload failed (${res.status}).`);
        return;
      }
      $('trackLyricsFile').value = '';
      await loadTrackDetails(state.selectedAdminTrackId);
      await loadData();
    }

    async function uploadTrackLyricsPlain() {
      if (!state.selectedAdminTrackId) {
        alert('Select a track first in Track Management.');
        return;
      }
      const f = $('trackLyricsPlainFile').files[0];
      if (!f) {
        alert('Select a .txt file first.');
        return;
      }
      const form = new FormData();
      form.append('lyrics', f);
      const res = await apiFetch(`/api/v1/manage/tracks/${encodeURIComponent(state.selectedAdminTrackId)}/lyrics-plain`, {
        method: 'POST',
        headers: headers(),
        body: form
      });
      if (!res.ok) {
        alert(`Plain lyrics upload failed (${res.status}).`);
        return;
      }
      $('trackLyricsPlainFile').value = '';
      await loadTrackDetails(state.selectedAdminTrackId);
      await loadData();
    }

    async function saveAlbumMetadata() {
      if (!state.selectedManageAlbumId) {
        alert('Select an album first in Album Management.');
        return;
      }
      const res = await apiFetch(`/api/v1/manage/albums/${state.selectedManageAlbumId}`, {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          title: $('albumEditTitle').value.trim(),
          artist: $('albumEditArtist').value.trim(),
          genre: $('albumEditGenre').value.trim(),
          visibility: $('albumEditVisibility').value
        })
      });
      if (!res.ok) {
        alert(`Album update failed (${res.status}).`);
        return;
      }
      await loadData();
    }

    async function deleteAlbumMetadata() {
      if (!state.selectedManageAlbumId) {
        alert('Select an album first.');
        return;
      }
      const ok = confirm('Delete selected album? Only empty albums can be removed.');
      if (!ok) return;
      const res = await apiFetch(`/api/v1/manage/albums/${state.selectedManageAlbumId}`, {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        const text = (await res.text().catch(() => '')).trim();
        alert(text || `Album delete failed (${res.status}).`);
        return;
      }
      setAlbumTarget(null);
      await loadData();
    }

    async function uploadAlbumCover() {
      if (!state.selectedManageAlbumId) {
        alert('Select an album first in Album Management.');
        return;
      }
      const f = $('albumCoverFile').files[0];
      if (!f) {
        alert('Select a cover image first.');
        return;
      }
      const form = new FormData();
      form.append('cover', f);
      const res = await apiFetch(`/api/v1/manage/albums/${state.selectedManageAlbumId}/cover`, {
        method: 'POST',
        headers: headers(),
        body: form
      });
      if (!res.ok) {
        alert(`Album cover upload failed (${res.status}).`);
        return;
      }
      $('albumCoverFile').value = '';
      await loadData();
    }

Object.assign(window.HexSonic, {
      setAdminTarget,
      applyTrackManageMode,
      loadTrackForEditor,
      setAlbumTarget,
      renderManageAlbums,
      loadTrackDetails,
      loadManageData,
      renderAdminUsers,
      loadAdminUsers,
      adminToggleUserEnabled,
      adminSaveUserRoles,
      topRoleFromRoles,
      adminDeleteUserByID,
      adminDeleteFilteredUsers,
      adminCreateInvite,
      loadAdminInvites,
      renderAdminInvites,
      adminDeleteInviteByID,
      signupWithInvitePage,
      esc,
      statusPill,
      adminProxyURL,
      openAdminMonitor,
      refreshAdminProxySessions,
      renderAdminSystemOverview,
      loadAdminSystemOverview,
      loadAdminAuditLogs,
      loadAdminDebugToggle,
      saveAdminDebugToggle,
      toggleRegistrationSetting,
      saveAdminJukeboxSettings,
      loadJobs,
      adminSetVisibility,
      adminDeleteTrack,
      uploadTrackCover,
      uploadTrackLyrics,
      uploadTrackLyricsPlain,
      saveAlbumMetadata,
      deleteAlbumMetadata,
      uploadAlbumCover
});
})(window.HexSonic = window.HexSonic || {});
