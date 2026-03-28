(function(ns) {
const { state, $, escapeHtml, headers, apiFetch, setStatus } = ns;
const PROFILE_BANNER_PLACEHOLDER = '/assets/profile-banner-placeholder.svg';
const stopNotificationPolling = (...args) => ns.stopNotificationPolling(...args);
const syncRoleUI = (...args) => ns.syncRoleUI(...args);
const clearSession = (...args) => ns.clearSession(...args);
const ensureAccessToken = (...args) => ns.ensureAccessToken(...args);
const startNotificationPolling = (...args) => ns.startNotificationPolling(...args);
const saveSession = (...args) => ns.saveSession(...args);
const syncPublicUI = (...args) => ns.syncPublicUI(...args);
const canAdmin = (...args) => ns.canAdmin(...args);
const canUpload = (...args) => ns.canUpload(...args);
const switchView = (...args) => ns.switchView(...args);
const renderAlbums = (...args) => ns.renderAlbums(...args);
const selectAlbum = (...args) => ns.selectAlbum(...args);
const openTrackDetail = (...args) => ns.openTrackDetail(...args);
const selectPlaylist = (...args) => ns.selectPlaylist(...args);
const loadPlaylists = (...args) => ns.loadPlaylists(...args);

    function displayRoleLabel(roles) {
      const list = Array.isArray(roles) ? roles.map((r) => String(r || '').trim().toLowerCase()) : [];
      if (list.includes('admin')) return 'Admin';
      if (list.includes('moderator')) return 'Moderator';
      if (list.includes('member')) return 'Member';
      if (list.includes('user')) return 'User';
      return '';
    }

    const PROFILE_ACCENT_PALETTES = [
      { value: '#2d78dd', label: 'Ocean Blue' },
      { value: '#cf5f7a', label: 'Rose Signal' },
      { value: '#52a36d', label: 'Emerald Pulse' },
      { value: '#b48432', label: 'Amber Forge' },
      { value: '#7b63d6', label: 'Royal Violet' },
      { value: '#3c9da3', label: 'Teal Current' },
      { value: '#c65d3d', label: 'Burnt Copper' },
      { value: '#8e9a2f', label: 'Moss Relay' }
    ];

    function switchProfileTab(tab) {
      const target = String(tab || 'identity').trim() || 'identity';
      state.currentProfileTab = target;
      document.querySelectorAll('[data-profile-tab-btn]').forEach((el) => {
        el.classList.toggle('active', el.getAttribute('data-profile-tab-btn') === target);
      });
      document.querySelectorAll('[data-profile-tab]').forEach((el) => {
        el.classList.toggle('hidden', el.getAttribute('data-profile-tab') !== target);
      });
    }

    function profileRouteIdentifier(profile) {
      const preferred = String(profile?.display_name || '').trim();
      if (preferred) return preferred;
      return String(profile?.subject || state.selectedUserSub || '').trim();
    }

    async function loadMe() {
      if (!state.token) {
        state.me = null;
        stopNotificationPolling();
        setStatus('Guest mode (public view)');
        syncRoleUI();
        return;
      }
      await ensureAccessToken();
      if (!state.token) {
        state.me = null;
        stopNotificationPolling();
        setStatus('Guest mode (public view)');
        syncRoleUI();
        return;
      }
      const res = await apiFetch('/api/v1/me', { headers: headers() });
      if (!res.ok) {
        state.me = null;
        stopNotificationPolling();
        if (res.status === 401) {
          clearSession();
          setStatus('Session expired - guest mode', 'bad');
        } else {
          setStatus('Session check temporary failed', 'warn');
        }
        syncRoleUI();
        return;
      }
      const me = await res.json();
      state.me = me;
      const roleLabel = displayRoleLabel(me.roles);
      const creator = me.creator_badge ? ' · Creator' : '';
      const rolePart = roleLabel ? ` · ${roleLabel}` : '';
      setStatus(`Logged in as ${me.username}${rolePart}${creator}`, 'ok');
      syncRoleUI();
      startNotificationPolling();
    }

    async function loadPublicSettings() {
      const res = await apiFetch('/api/v1/public/settings', { headers: headers() }, false);
      if (!res.ok) {
        state.publicSettings = { registration_enabled: false };
        syncPublicUI();
        return;
      }
      const json = await res.json();
      state.publicSettings = {
        registration_enabled: !!json.registration_enabled
      };
      syncPublicUI();
    }

    function renderMyProfile() {
      const p = state.myProfile;
      fillProfileAccentSelector();
      fillFeaturedSelectors();
      if (!p) {
        $('profileDisplayName').value = '';
        $('profileEmail').value = '';
        $('profileBio').value = '';
        $('profileStatusLine').value = '';
        $('profileAccentColor').value = '#2d78dd';
        ['profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4'].forEach((id) => {
          $(id).value = '';
          $(id).disabled = true;
        });
        $('profileFeaturedPlaylist').value = '';
        $('profileJukeboxGenres').value = '';
        $('profileGuestShowFollowers').checked = true;
        $('profileGuestShowPlaylists').checked = true;
        $('profileGuestShowFavorites').checked = false;
        $('profileGuestShowStats').checked = true;
        $('profileGuestShowUploads').checked = true;
        $('profileRoleMeta').textContent = 'Listener profile active. Creator layouts unlock featured modules and richer public presentation.';
        $('profileFeaturedPlaylist').disabled = true;
        $('profileAvatarPreview').removeAttribute('src');
        $('profileAvatarBoxPreview').removeAttribute('src');
        $('profileBannerPreview').removeAttribute('src');
        $('profileBannerBoxPreview').removeAttribute('src');
        $('profileSubsonicStatus').textContent = 'Not set';
        $('profilePreviewName').textContent = 'Your profile';
        $('profilePreviewHandle').textContent = '@user';
        $('profilePreviewRole').textContent = 'Listener';
        $('profilePreviewStatus').textContent = 'Status line';
        $('profilePreviewBio').textContent = 'Bio preview appears here.';
        $('profilePreviewFeaturedCount').textContent = '0';
        $('profilePreviewGuestsMeta').textContent = 'followers, stats and uploads';
        $('profileDisplayNameCooldown').textContent = 'Display name changes are limited to once every 30 days.';
        state.profileSnapshot = '';
        state.profileDirty = false;
        state.profileSaveState = '';
        renderProfileSaveState();
        return;
      }
      const isCreator = !!p.creator_badge;
      const creatorTabBtn = document.querySelector('[data-profile-tab-btn="creator"]');
      const creatorTabPanel = document.querySelector('[data-profile-tab="creator"]');
      if (creatorTabBtn) creatorTabBtn.classList.toggle('hidden', !isCreator);
      if (creatorTabPanel) creatorTabPanel.classList.toggle('hidden', !isCreator);
      $('profileDisplayName').value = p.display_name || '';
      $('profileEmail').value = p.email || '';
      $('profileBio').value = p.bio || '';
      $('profileStatusLine').value = p.status_line || '';
      $('profileAccentColor').value = p.accent_color || '#2d78dd';
      const featured = Array.isArray(p.featured_album_ids) ? p.featured_album_ids.map((x) => String(x)) : [];
      ['profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4'].forEach((id, idx) => {
        $(id).value = featured[idx] || '';
        $(id).disabled = !isCreator;
      });
      $('profileFeaturedPlaylist').value = p.featured_playlist_id ? String(p.featured_playlist_id) : '';
      $('profileJukeboxGenres').value = Array.isArray(p.jukebox_preferred_genres) ? p.jukebox_preferred_genres.join(', ') : '';
      renderProfileJukeboxSummary(p);
      $('profileGuestShowFollowers').checked = !!p.guest_show_followers;
      $('profileGuestShowPlaylists').checked = !!p.guest_show_playlists;
      $('profileGuestShowFavorites').checked = !!p.guest_show_favorites;
      $('profileGuestShowStats').checked = !!p.guest_show_stats;
      $('profileGuestShowUploads').checked = !!p.guest_show_uploads;
      $('profileFeaturedPlaylist').disabled = !isCreator;
      $('profileRoleMeta').textContent = isCreator
        ? 'Creator profile active. Featured modules, uploads and public social proof are fully available.'
        : 'Listener profile active. You can customize identity, playlists and favorites. Featured creator modules stay reserved for creators.';
      $('profileSubsonicStatus').textContent = p.has_subsonic_password ? 'Configured' : 'Not set';
      if (p.avatar_url) {
        $('profileAvatarPreview').src = p.avatar_url;
        $('profileAvatarBoxPreview').src = p.avatar_url;
      } else {
        $('profileAvatarPreview').removeAttribute('src');
        $('profileAvatarBoxPreview').removeAttribute('src');
      }
      if (p.banner_url) {
        $('profileBannerPreview').src = p.banner_url;
        $('profileBannerBoxPreview').src = p.banner_url;
      } else {
        $('profileBannerPreview').src = PROFILE_BANNER_PLACEHOLDER;
        $('profileBannerBoxPreview').src = PROFILE_BANNER_PLACEHOLDER;
      }
      $('profilePreviewName').textContent = p.display_name || p.username || 'User';
      $('profilePreviewHandle').textContent = `@${p.username || 'user'}`;
      $('profilePreviewRole').textContent = isCreator ? 'Creator' : 'Listener';
      $('profilePreviewStatus').textContent = p.status_line || (isCreator ? 'Creator profile active' : 'Listener profile active');
      $('profilePreviewBio').textContent = p.bio || 'Add a bio so visitors understand who you are and what they will find here.';
      $('profilePreviewFeaturedCount').textContent = String(featured.filter(Boolean).length);
      const guestBits = [];
      if (p.guest_show_followers) guestBits.push('followers');
      if (p.guest_show_playlists) guestBits.push('playlists');
      if (p.guest_show_favorites) guestBits.push('favorites');
      if (p.guest_show_stats) guestBits.push('stats');
      if (p.guest_show_uploads) guestBits.push('uploads');
      $('profilePreviewGuestsMeta').textContent = guestBits.length ? guestBits.join(', ') : 'private profile extras';
      $('profilePreviewBannerWrap').style.background = p.accent_color || '#2d78dd';
      const nextChangeAt = p.display_name_change_available_at ? new Date(p.display_name_change_available_at) : null;
      $('profileDisplayNameCooldown').textContent = nextChangeAt && Number.isFinite(nextChangeAt.getTime())
        ? `Next display name change: ${nextChangeAt.toLocaleDateString()}`
        : 'Display name changes are limited to once every 30 days.';
      const targetTab = (!isCreator && state.currentProfileTab === 'creator')
        ? 'identity'
        : (state.currentProfileTab || 'identity');
      switchProfileTab(targetTab);
      state.profileSnapshot = currentProfileFormSnapshot();
      state.profileDirty = false;
      if (state.profileSaveState !== 'saved') {
        state.profileSaveState = '';
      }
      renderProfileSaveState();
      syncProfilePreviewFromForm();
    }

    function renderProfileJukeboxSummary(profile) {
      const tuning = profile?.jukebox_tuning || {};
      const history = Array.isArray(profile?.jukebox_feedback_history) ? profile.jukebox_feedback_history : [];
      const tuningEl = $('profileJukeboxTuning');
      const historyBody = $('profileJukeboxHistoryBody');
      if (tuningEl) {
        const blocks = [];
        const boostedGenres = Array.isArray(tuning.boosted_genres) ? tuning.boosted_genres : [];
        const mutedGenres = Array.isArray(tuning.muted_genres) ? tuning.muted_genres : [];
        const boostedCreators = Array.isArray(tuning.boosted_creators) ? tuning.boosted_creators : [];
        const mutedCreators = Array.isArray(tuning.muted_creators) ? tuning.muted_creators : [];
        const fixedGenre = String(tuning.fixed_genre || '').trim();
        const surpriseBias = Number(tuning.surprise_bias || 0);
        if (boostedGenres.length) blocks.push(`<div><b>More genres:</b> ${escapeHtml(boostedGenres.join(', '))}</div>`);
        if (mutedGenres.length) blocks.push(`<div><b>Less genres:</b> ${escapeHtml(mutedGenres.join(', '))}</div>`);
        if (boostedCreators.length) blocks.push(`<div><b>Boosted creators:</b> ${escapeHtml(boostedCreators.join(', '))}</div>`);
        if (mutedCreators.length) blocks.push(`<div><b>Muted creators:</b> ${escapeHtml(mutedCreators.join(', '))}</div>`);
        if (fixedGenre) blocks.push(`<div><b>Genre lock:</b> ${escapeHtml(fixedGenre)}</div>`);
        if (surpriseBias > 0) blocks.push(`<div><b>Surprise bias:</b> ${surpriseBias}</div>`);
        tuningEl.innerHTML = blocks.length ? blocks.join('') : '<span class="muted">No active tuning yet.</span>';
      }
      if (historyBody) {
        if (!history.length) {
          historyBody.innerHTML = '<tr><td colspan="4" class="muted">No feedback recorded yet.</td></tr>';
          return;
        }
        historyBody.innerHTML = history.map((item) => {
          const action = String(item.action || '').replaceAll('_', ' ');
          const title = String(item.track_title || '').trim() || '-';
          const genre = String(item.genre || '').trim() || '-';
          const createdAt = item.created_at ? new Date(item.created_at) : null;
          const timeLabel = createdAt && Number.isFinite(createdAt.getTime()) ? createdAt.toLocaleString() : '-';
          return `<tr>
            <td>${escapeHtml(action)}</td>
            <td>
              <div class="track-main-title">${escapeHtml(title)}</div>
              <div class="uploader-inline">${escapeHtml(String(item.album_title || '').trim() || String(item.creator || '').trim() || '-')}</div>
            </td>
            <td>${escapeHtml(genre)}</td>
            <td>${escapeHtml(timeLabel)}</td>
          </tr>`;
        }).join('');
      }
    }

    function syncProfilePreviewFromForm() {
      const displayName = $('profileDisplayName')?.value?.trim() || state.myProfile?.display_name || state.me?.username || 'User';
      const username = state.me?.username || state.myProfile?.username || 'user';
      const status = $('profileStatusLine')?.value?.trim() || state.myProfile?.status_line || 'Status line';
      const bio = $('profileBio')?.value?.trim() || state.myProfile?.bio || 'Bio preview appears here.';
      const accent = $('profileAccentColor')?.value?.trim() || state.myProfile?.accent_color || '#2d78dd';
      const featuredCount = ['profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4']
        .map((id) => String($(id)?.value || '').trim())
        .filter(Boolean).filter((v, idx, arr) => arr.indexOf(v) === idx).length;
      const guestBits = [];
      if ($('profileGuestShowFollowers')?.checked) guestBits.push('followers');
      if ($('profileGuestShowPlaylists')?.checked) guestBits.push('playlists');
      if ($('profileGuestShowFavorites')?.checked) guestBits.push('favorites');
      if ($('profileGuestShowStats')?.checked) guestBits.push('stats');
      if ($('profileGuestShowUploads')?.checked) guestBits.push('uploads');
      $('profilePreviewName').textContent = displayName;
      $('profilePreviewHandle').textContent = `@${username}`;
      $('profilePreviewStatus').textContent = status;
      $('profilePreviewBio').textContent = bio;
      $('profilePreviewFeaturedCount').textContent = String(featuredCount);
      $('profilePreviewGuestsMeta').textContent = guestBits.length ? guestBits.join(', ') : 'private profile extras';
      $('profilePreviewBannerWrap').style.background = accent;
      updateProfileDirtyState();
    }

    function currentProfileFormSnapshot() {
      const featuredAlbumIDs = ['profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4']
        .map((id) => Number($(id)?.value || 0))
        .filter((v, idx, arr) => Number.isFinite(v) && v > 0 && arr.indexOf(v) === idx);
      return JSON.stringify({
        display_name: $('profileDisplayName')?.value?.trim() || '',
        email: $('profileEmail')?.value?.trim() || '',
        bio: $('profileBio')?.value?.trim() || '',
        status_line: $('profileStatusLine')?.value?.trim() || '',
        accent_color: $('profileAccentColor')?.value?.trim() || '',
        featured_album_ids: featuredAlbumIDs,
        featured_playlist_id: $('profileFeaturedPlaylist')?.value ? Number($('profileFeaturedPlaylist').value) : null,
        jukebox_preferred_genres: String($('profileJukeboxGenres')?.value || '').split(',').map((x) => x.trim()).filter(Boolean),
        guest_show_followers: !!$('profileGuestShowFollowers')?.checked,
        guest_show_playlists: !!$('profileGuestShowPlaylists')?.checked,
        guest_show_favorites: !!$('profileGuestShowFavorites')?.checked,
        guest_show_stats: !!$('profileGuestShowStats')?.checked,
        guest_show_uploads: !!$('profileGuestShowUploads')?.checked
      });
    }

    function renderProfileSaveState() {
      const el = $('profileSaveState');
      if (!el) return;
      el.classList.remove('dirty', 'saved');
      if (state.profileDirty) {
        el.textContent = 'Unsaved changes';
        el.classList.add('dirty');
        return;
      }
      if (state.profileSaveState === 'saved') {
        el.textContent = 'Saved OK';
        el.classList.add('saved');
        return;
      }
      el.textContent = '';
    }

    function updateProfileDirtyState() {
      if (!state.me || !state.myProfile) {
        state.profileDirty = false;
        renderProfileSaveState();
        return;
      }
      state.profileDirty = currentProfileFormSnapshot() !== String(state.profileSnapshot || '');
      if (state.profileDirty && state.profileSaveState === 'saved') {
        state.profileSaveState = '';
      }
      renderProfileSaveState();
    }

    function previewLocalProfileImage(inputID, targetID) {
      const input = $(inputID);
      const target = $(targetID);
      const file = input?.files?.[0];
      if (!file || !target) return;
      const reader = new FileReader();
      reader.onload = () => {
        target.src = String(reader.result || '');
      };
      reader.readAsDataURL(file);
    }

    async function loadMyProfile() {
      if (!state.me) {
        state.myProfile = null;
        renderMyProfile();
        return;
      }
      const res = await apiFetch('/api/v1/me/profile', { headers: headers() });
      if (!res.ok) {
        state.myProfile = null;
        renderMyProfile();
        return;
      }
      state.myProfile = await res.json();
      renderMyProfile();
    }

    function fillFeaturedSelectors() {
      const albumSelects = ['profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4'].map((id) => $(id));
      const playlistSelect = $('profileFeaturedPlaylist');
      albumSelects.forEach((select, idx) => {
        select.innerHTML = `<option value="">Featured album ${idx + 1} (none)</option>`;
      });
      playlistSelect.innerHTML = '<option value="">Featured playlist (none)</option>';
      (state.albums || []).filter((a) => state.me && (a.owner_sub === state.me.subject || canAdmin())).forEach((a) => {
        albumSelects.forEach((select) => {
          const opt = document.createElement('option');
          opt.value = String(a.id);
          opt.textContent = `${a.title} ${a.artist ? `- ${a.artist}` : ''}`;
          select.appendChild(opt);
        });
      });
      (state.playlists || []).filter((p) => state.me && (p.owner_sub === state.me.subject || canAdmin())).forEach((p) => {
        const opt = document.createElement('option');
        opt.value = String(p.id);
        opt.textContent = p.name || `Playlist ${p.id}`;
        playlistSelect.appendChild(opt);
      });
    }

    function fillProfileAccentSelector() {
      const select = $('profileAccentColor');
      const current = String(state.myProfile?.accent_color || select.value || '#2d78dd').trim();
      select.innerHTML = '';
      PROFILE_ACCENT_PALETTES.forEach((entry) => {
        const opt = document.createElement('option');
        opt.value = entry.value;
        opt.textContent = entry.label;
        select.appendChild(opt);
      });
      if (!PROFILE_ACCENT_PALETTES.some((entry) => entry.value.toLowerCase() === current.toLowerCase())) {
        const opt = document.createElement('option');
        opt.value = current || '#2d78dd';
        opt.textContent = 'Current profile color';
        select.appendChild(opt);
      }
      select.value = current || '#2d78dd';
    }

    async function loadNotifications() {
      if (!state.me) {
        state.notifications = [];
        state.notificationsUnread = 0;
        renderNotifications();
        syncRoleUI();
        return;
      }
      const res = await apiFetch('/api/v1/me/notifications', { headers: headers() });
      if (!res.ok) {
        if (res.status === 401) {
          stopNotificationPolling();
        }
        state.notifications = [];
        state.notificationsUnread = 0;
        renderNotifications();
        syncRoleUI();
        return;
      }
      const json = await res.json();
      state.notifications = json.notifications || [];
      state.notificationsUnread = Number(json.unread || 0);
      renderNotifications();
      syncRoleUI();
    }

    function renderNotifications() {
      const root = $('notificationsList');
      root.innerHTML = '';
      if (!state.notifications.length) {
        root.innerHTML = '<div class="muted">No notifications.</div>';
        return;
      }
      const kindLabel = {
        creator_follow: 'Follow',
        creator_release: 'Release',
        creator_track_comment: 'Track Comment',
        creator_album_comment: 'Album Comment',
        creator_profile_comment: 'Profile Comment'
      };
      state.notifications.forEach((n) => {
        const item = document.createElement('div');
        item.className = 'album-comment-item';
        item.style.opacity = n.is_read ? '0.75' : '1';
        item.style.borderColor = n.is_read ? '#4f5662' : '#bf5e74';
        item.style.background = n.is_read ? '' : 'linear-gradient(180deg, rgba(110, 40, 57, 0.28), rgba(61, 36, 46, 0.12))';
        item.innerHTML = `
          <div class="album-comment-head">
            <span><strong>${escapeHtml(n.title || 'Notification')}</strong> <span class="pill">${escapeHtml(kindLabel[n.kind] || 'Update')}</span></span>
            <span>${escapeHtml((n.created_at || '').replace('T', ' ').slice(0, 19))}</span>
          </div>
          <div class="album-comment-body">${escapeHtml(n.body || '')}</div>
        `;
        item.onclick = async () => {
          if (!n.is_read) {
            await apiFetch(`/api/v1/me/notifications/${encodeURIComponent(n.id)}/read`, { method: 'POST', headers: headers() });
            await loadNotifications();
          }
          if (n.album_id) {
            await selectAlbum(Number(n.album_id));
          } else if (n.actor_sub) {
            await openUserProfile(n.actor_sub);
          }
          $('notificationsDrawer').classList.add('hidden');
        };
        root.appendChild(item);
      });
    }

    async function loadFavorites() {
      if (!state.me) {
        state.favorites = { tracks: [], albums: [], playlists: [] };
        return;
      }
      const res = await apiFetch('/api/v1/favorites', { headers: headers() });
      if (!res.ok) {
        state.favorites = { tracks: [], albums: [], playlists: [] };
        return;
      }
      state.favorites = await res.json();
    }

    function isFavorite(kind, id) {
      const list = Array.isArray(state.favorites?.[`${kind}s`]) ? state.favorites[`${kind}s`] : [];
      return list.some((x) => String(x.id) === String(id));
    }

    async function toggleFavorite(kind, id) {
      if (!state.me) {
        alert('Login required.');
        return;
      }
      const existing = isFavorite(kind, id);
      const url = `/api/v1/favorites/${encodeURIComponent(kind)}/${encodeURIComponent(id)}`;
      if (existing) {
        await apiFetch(url, { method: 'DELETE', headers: headers() });
      } else {
        const payload = { kind };
        if (kind === 'track') payload.track_id = String(id);
        if (kind === 'album') payload.album_id = Number(id);
        if (kind === 'playlist') payload.playlist_id = Number(id);
        await apiFetch('/api/v1/favorites', {
          method: 'POST',
          headers: headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify(payload)
        });
      }
      await loadFavorites();
      renderAlbums();
      renderTracks();
      renderPlaylists();
      renderFavorites();
      if (state.selectedUserSub) renderPublicUserProfile();
    }

    function setFavoritesTab(tab) {
      const valid = new Set(['albums', 'tracks', 'playlists']);
      state.favoritesTab = valid.has(tab) ? tab : 'albums';
      document.querySelectorAll('[data-favorites-tab]').forEach((btn) => {
        btn.classList.toggle('active', btn.getAttribute('data-favorites-tab') === state.favoritesTab);
      });
      renderFavorites();
    }

    function renderFavorites() {
      const albums = Array.isArray(state.favorites?.albums) ? state.favorites.albums : [];
      const tracks = Array.isArray(state.favorites?.tracks) ? state.favorites.tracks : [];
      const playlists = Array.isArray(state.favorites?.playlists) ? state.favorites.playlists : [];
      $('favoritesAlbumCount').textContent = String(albums.length);
      $('favoritesTrackCount').textContent = String(tracks.length);
      $('favoritesPlaylistCount').textContent = String(playlists.length);

      const head = $('favoritesHeadRow');
      const body = $('favoritesBody');
      if (!head || !body) return;
      head.innerHTML = '';
      body.innerHTML = '';

      const titles = {
        albums: 'Favorite Albums',
        tracks: 'Favorite Songs',
        playlists: 'Favorite Playlists'
      };
      const meta = {
        albums: 'A clean overview of the albums you marked as favorites.',
        tracks: 'Direct access to the songs you saved for repeat listening.',
        playlists: 'Pinned playlists you want to keep in immediate reach.'
      };
      $('favoritesTitle').textContent = titles[state.favoritesTab] || titles.albums;
      $('favoritesMeta').textContent = meta[state.favoritesTab] || meta.albums;

      if (!state.me) {
        head.innerHTML = '<th>Favorites</th>';
        body.innerHTML = '<tr><td class="favorites-empty">Login required.</td></tr>';
        return;
      }

      if (state.favoritesTab === 'albums') {
        head.innerHTML = `
          <th>Album</th>
          <th style="width:180px">Artist</th>
          <th style="width:140px">Genre</th>
          <th style="width:120px">Visibility</th>
          <th style="width:150px">Uploader</th>
          <th style="width:150px">Action</th>
        `;
        albums.forEach((a) => {
          const tr = document.createElement('tr');
          const uploader = a.uploader_name || a.owner_sub || '-';
          tr.innerHTML = `
            <td><div class="track-main-title">${escapeHtml(a.title || 'Album')}</div></td>
            <td>${escapeHtml(a.artist || '-')}</td>
            <td>${escapeHtml(a.genre || '-')}</td>
            <td><span class="pill">${escapeHtml(a.visibility || 'public')}</span></td>
            <td><a href="#" class="uploader-link" data-user-sub="${escapeHtml(a.owner_sub || '')}">${escapeHtml(uploader)}</a></td>
            <td>
              <div style="display:flex; gap:6px; flex-wrap:wrap;">
                <button class="btn slim" data-fav-open-album="${a.id}" title="Open album">${ICON_INFO}</button>
                <button class="btn slim" data-fav-play-album="${a.id}" title="Play album">${ICON_PLAY}</button>
                <button class="btn slim" data-fav-remove-album="${a.id}" title="Remove favorite">★</button>
              </div>
            </td>
          `;
          const upLink = tr.querySelector('[data-user-sub]');
          if (upLink && a.owner_sub) upLink.onclick = async (e) => { e.preventDefault(); await openUserProfile(a.owner_sub); };
          tr.querySelector('[data-fav-open-album]').onclick = async () => selectAlbum(Number(a.id));
          tr.querySelector('[data-fav-play-album]').onclick = async () => {
            await selectAlbum(Number(a.id));
            const rows = getSelectedAlbumTracks();
            if (rows.length) await startTrackById(rows[0].id, rows, 'favorites');
          };
          tr.querySelector('[data-fav-remove-album]').onclick = async () => toggleFavorite('album', a.id);
          body.appendChild(tr);
        });
        if (!albums.length) {
          body.innerHTML = '<tr><td colspan="6" class="favorites-empty">No favorite albums yet.</td></tr>';
        }
        return;
      }

      if (state.favoritesTab === 'tracks') {
        head.innerHTML = `
          <th>Song</th>
          <th style="width:180px">Artist</th>
          <th style="width:180px">Album</th>
          <th style="width:130px">Genre</th>
          <th style="width:86px">Time</th>
          <th style="width:150px">Action</th>
        `;
        tracks.forEach((t) => {
          const tr = document.createElement('tr');
          tr.innerHTML = `
            <td><div class="track-main-title">${escapeHtml(t.title || 'Track')}</div></td>
            <td>${escapeHtml(t.artist || '-')}</td>
            <td>${escapeHtml(t.album || '-')}</td>
            <td>${escapeHtml(t.genre || '-')}</td>
            <td>${fmt(Math.round(t.duration_seconds || t.duration || 0))}</td>
            <td>
              <div style="display:flex; gap:6px; flex-wrap:wrap;">
                <button class="btn slim" data-fav-play-track="${t.id}" title="Play">${ICON_PLAY}</button>
                <button class="btn slim" data-fav-track-detail="${t.id}" title="Details">${ICON_INFO}</button>
                <button class="btn slim" data-fav-track-playlist="${t.id}" title="Add to Playlist">${ICON_PLAYLIST}</button>
                <button class="btn slim" data-fav-remove-track="${t.id}" title="Remove favorite">★</button>
              </div>
            </td>
          `;
          tr.querySelector('[data-fav-play-track]').onclick = async () => startTrackById(t.id, tracks, 'favorites');
          tr.querySelector('[data-fav-track-detail]').onclick = async () => openTrackDetail(t.id);
          const addBtn = tr.querySelector('[data-fav-track-playlist]');
          if (addBtn) {
            addBtn.classList.toggle('hidden', !canCreatePlaylists());
            addBtn.onclick = async () => addTrackToPlaylist(t.id);
          }
          tr.querySelector('[data-fav-remove-track]').onclick = async () => toggleFavorite('track', t.id);
          body.appendChild(tr);
        });
        if (!tracks.length) {
          body.innerHTML = '<tr><td colspan="6" class="favorites-empty">No favorite songs yet.</td></tr>';
        }
        return;
      }

      head.innerHTML = `
        <th>Playlist</th>
        <th style="width:120px">Visibility</th>
        <th style="width:120px">Tracks</th>
        <th style="width:150px">Owner</th>
        <th style="width:150px">Action</th>
      `;
      playlists.forEach((pl) => {
        const owner = pl.owner_name || pl.owner_sub || '-';
        const tr = document.createElement('tr');
        tr.innerHTML = `
          <td><div class="track-main-title">${escapeHtml(pl.name || 'Playlist')}</div></td>
          <td><span class="pill">${escapeHtml(pl.visibility || 'public')}</span></td>
          <td>${Number(pl.track_count || 0)}</td>
          <td><a href="#" class="uploader-link" data-user-sub="${escapeHtml(pl.owner_sub || '')}">${escapeHtml(owner)}</a></td>
          <td>
            <div style="display:flex; gap:6px; flex-wrap:wrap;">
              <button class="btn slim" data-fav-open-playlist="${pl.id}" title="Open playlist">${ICON_INFO}</button>
              <button class="btn slim" data-fav-remove-playlist="${pl.id}" title="Remove favorite">★</button>
            </div>
          </td>
        `;
        const upLink = tr.querySelector('[data-user-sub]');
        if (upLink && pl.owner_sub) upLink.onclick = async (e) => { e.preventDefault(); await openUserProfile(pl.owner_sub); };
        tr.querySelector('[data-fav-open-playlist]').onclick = async () => {
          switchView('playlists');
          await loadPlaylists();
          await selectPlaylist(Number(pl.id));
        };
        tr.querySelector('[data-fav-remove-playlist]').onclick = async () => toggleFavorite('playlist', pl.id);
        body.appendChild(tr);
      });
      if (!playlists.length) {
        body.innerHTML = '<tr><td colspan="5" class="favorites-empty">No favorite playlists yet.</td></tr>';
      }
    }

    async function loadCreatorHighscore() {
      const windowValue = $('creatorHighscoreWindow').value || '7d';
      const res = await apiFetch(`/api/v1/creators/highscore?window=${encodeURIComponent(windowValue)}`, { headers: headers() }, false);
      if (!res.ok) {
        state.creatorHighscore = [];
        $('creatorHighscoreMeta').textContent = `Creator highscore unavailable (${res.status}).`;
        $('creatorHighscoreBody').innerHTML = '<tr><td colspan="9" class="muted">Unavailable.</td></tr>';
        return;
      }
      const json = await res.json();
      state.creatorHighscore = json.creators || [];
      $('creatorHighscoreMeta').textContent = `Window: ${json.window || '-'} · Public score based on listeners, playlist adds, listening ratio, replays, plays and ratings.`;
      const body = $('creatorHighscoreBody');
      body.innerHTML = '';
      state.creatorHighscore.forEach((row) => {
        const tr = document.createElement('tr');
        tr.innerHTML = `
          <td>${row.rank}</td>
          <td><a class="uploader-link" data-user-sub="${escapeHtml(row.user_sub || '')}">${escapeHtml(row.display_name || row.user_sub || '-')}</a><div class="uploader-inline">${escapeHtml(row.status_line || '')}</div></td>
          <td>${Number(row.followers || 0)}</td>
          <td>${Number(row.unique_listeners || 0)}</td>
          <td>${Number(row.plays || 0)}</td>
          <td>${Number(row.playlist_adds || 0)}</td>
          <td>${Number(row.replays || 0)}</td>
          <td>${Math.round(Number(row.avg_listen_ratio || 0) * 100)}%</td>
          <td>${Number(row.score || 0).toFixed(1)}</td>
        `;
        const link = tr.querySelector('[data-user-sub]');
        if (link) link.onclick = async (e) => {
          e.preventDefault();
          await openUserProfile(link.getAttribute('data-user-sub'));
        };
        body.appendChild(tr);
      });
      if (!state.creatorHighscore.length) {
        body.innerHTML = '<tr><td colspan="9" class="muted">No creators ranked yet.</td></tr>';
      }
    }

    async function saveMyProfile() {
      if (!state.me) return;
      const featuredAlbumIDs = ['profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4']
        .map((id) => Number($(id).value || 0))
        .filter((v, idx, arr) => Number.isFinite(v) && v > 0 && arr.indexOf(v) === idx);
      const payload = {
        display_name: $('profileDisplayName').value.trim(),
        email: $('profileEmail').value.trim(),
        bio: $('profileBio').value.trim(),
        status_line: $('profileStatusLine').value.trim(),
        accent_color: $('profileAccentColor').value.trim(),
        featured_album_ids: featuredAlbumIDs,
        featured_playlist_id: $('profileFeaturedPlaylist').value ? Number($('profileFeaturedPlaylist').value) : null,
        jukebox_preferred_genres: $('profileJukeboxGenres').value.split(',').map((x) => x.trim()).filter(Boolean),
        guest_show_followers: $('profileGuestShowFollowers').checked,
        guest_show_playlists: $('profileGuestShowPlaylists').checked,
        guest_show_favorites: $('profileGuestShowFavorites').checked,
        guest_show_stats: $('profileGuestShowStats').checked,
        guest_show_uploads: $('profileGuestShowUploads').checked
      };
      const res = await apiFetch('/api/v1/me/profile', {
        method: 'PATCH',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify(payload)
      });
      if (!res.ok) {
        alert(`Profile update failed (${res.status}).`);
        return;
      }
      state.profileSaveState = 'saved';
      await loadMyProfile();
      await loadMe();
    }

    async function resetMyJukeboxProfile() {
      const res = await apiFetch('/api/v1/me/jukebox/reset', {
        method: 'POST',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Jukebox reset failed (${res.status}).`);
        return;
      }
      state.profileSaveState = 'saved';
      await loadMyProfile();
    }

    async function uploadMyAvatar() {
      if (!state.me) return;
      const f = $('profileAvatarFile').files[0];
      if (!f) {
        alert('Select an avatar file first.');
        return;
      }
      const form = new FormData();
      form.append('avatar', f);
      const res = await apiFetch('/api/v1/me/avatar', {
        method: 'POST',
        headers: headers(),
        body: form
      });
      if (!res.ok) {
        alert(`Avatar upload failed (${res.status}).`);
        return;
      }
      $('profileAvatarFile').value = '';
      await loadMyProfile();
    }

    async function uploadMyBanner() {
      if (!state.me) return;
      const f = $('profileBannerFile').files[0];
      if (!f) {
        alert('Select a banner file first.');
        return;
      }
      const name = String(f.name || '').toLowerCase();
      if (!(/\.(jpg|jpeg|png|webp|gif)$/).test(name)) {
        alert('Banner must be JPG, PNG, WEBP or GIF.');
        return;
      }
      if (Number(f.size || 0) > (100 * 1024 * 1024)) {
        alert('Banner must be 100 MB or smaller.');
        return;
      }
      const form = new FormData();
      form.append('banner', f);
      const res = await apiFetch('/api/v1/me/banner', {
        method: 'POST',
        headers: headers(),
        body: form
      });
      if (!res.ok) {
        const msg = (await res.text().catch(() => '')).trim();
        alert(msg || `Banner upload failed (${res.status}).`);
        return;
      }
      $('profileBannerFile').value = '';
      await loadMyProfile();
    }

    async function updateMyPassword() {
      if (!state.me) return;
      const currentPassword = $('profileCurrentPassword').value;
      const newPassword = $('profileNewPassword').value;
      if (!currentPassword || !newPassword) {
        alert('Current and new password required.');
        return;
      }
      const res = await apiFetch('/api/v1/me/password', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword
        })
      });
      if (!res.ok) {
        alert(`Password update failed (${res.status}).`);
        return;
      }
      $('profileCurrentPassword').value = '';
      $('profileNewPassword').value = '';
      alert('Password updated.');
    }

    async function updateMySubsonicPassword() {
      if (!state.me) return;
      const password = $('profileSubsonicPassword').value;
      if (!password || password.length < 8) {
        alert('Subsonic password must be at least 8 characters.');
        return;
      }
      const res = await apiFetch('/api/v1/me/subsonic-password', {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ password })
      });
      if (!res.ok) {
        alert(`Subsonic password update failed (${res.status}).`);
        return;
      }
      $('profileSubsonicPassword').value = '';
      await loadMyProfile();
      alert('Subsonic password updated.');
    }

    async function deleteMySubsonicPassword() {
      if (!state.me) return;
      const res = await apiFetch('/api/v1/me/subsonic-password', {
        method: 'DELETE',
        headers: headers()
      });
      if (!res.ok) {
        alert(`Subsonic password delete failed (${res.status}).`);
        return;
      }
      $('profileSubsonicPassword').value = '';
      await loadMyProfile();
      alert('Subsonic password deleted.');
    }


    async function openUserProfile(userIdentifier) {
      if (!userIdentifier) return;
      state.selectedUserHandle = String(userIdentifier);
      state.selectedUserSub = String(userIdentifier);
      await loadPublicUserProfile();
      switchView('user_profile');
    }

    async function loadPublicUserProfile() {
      const identifier = String(state.selectedUserHandle || state.selectedUserSub || '').trim();
      if (!identifier) return;
      const [pRes, uRes, cRes] = await Promise.all([
        apiFetch(`/api/v1/users/${encodeURIComponent(identifier)}/profile`, { headers: headers() }, false),
        apiFetch(`/api/v1/users/${encodeURIComponent(identifier)}/uploads`, { headers: headers() }, false),
        apiFetch(`/api/v1/users/${encodeURIComponent(identifier)}/comments`, { headers: headers() }, false)
      ]);
      if (!pRes.ok) {
        alert(`User profile load failed (${pRes.status}).`);
        return;
      }
      state.selectedPublicUserProfile = await pRes.json();
      state.selectedUserSub = String(state.selectedPublicUserProfile?.subject || identifier);
      state.selectedUserHandle = profileRouteIdentifier(state.selectedPublicUserProfile);
      const canonicalUserPath = `/user/${encodeURIComponent(state.selectedUserHandle || state.selectedUserSub)}`;
      if (location.pathname !== canonicalUserPath) {
        history.replaceState(null, '', canonicalUserPath);
      }
      state.selectedPublicUserUploads = uRes.ok ? await uRes.json() : { albums: [], tracks: [] };
      state.selectedPublicUserComments = cRes.ok ? ((await cRes.json()).comments || []) : [];
      renderPublicUserProfile();
    }

    function renderPublicUserProfile() {
      const p = state.selectedPublicUserProfile;
      if (!p) return;
      $('pubUserName').textContent = p.display_name || p.subject || 'User';
      $('pubUserHandle').textContent = `@${String(p.display_name || p.subject || 'user').trim()}`;
      $('pubUserSub').textContent = canAdmin() ? (p.subject || '') : '';
      $('pubUserSub').classList.toggle('hidden', !canAdmin() || !(p.subject || '').trim());
      if (p.avatar_url) $('pubUserAvatar').src = p.avatar_url;
      else $('pubUserAvatar').removeAttribute('src');
      if (p.banner_url) {
        $('pubUserBanner').src = p.banner_url;
        $('pubUserBanner').classList.remove('hidden');
        $('pubUserHero').style.background = 'transparent';
      } else {
        $('pubUserBanner').src = PROFILE_BANNER_PLACEHOLDER;
        $('pubUserBanner').classList.remove('hidden');
        $('pubUserHero').style.background = p.accent_color || '#2d78dd';
      }
      $('pubUserCard').style.borderColor = p.accent_color || '#4a5160';
      $('pubUserStatus').textContent = p.status_line || (p.creator_badge ? 'Creator profile' : 'Listener profile');
      $('pubUserBio').textContent = p.bio && p.bio.trim() ? p.bio : 'No bio.';
      const metaBits = [];
      if (p.creator_badge) metaBits.push('Creator');
      if (p.playlist_count) metaBits.push(`${p.playlist_count} playlist${Number(p.playlist_count) === 1 ? '' : 's'}`);
      if (p.favorites_count) metaBits.push(`${p.favorites_count} favorite${Number(p.favorites_count) === 1 ? '' : 's'}`);
      $('pubUserMetaLine').textContent = metaBits.join(' · ');
      $('pubUserBadge').classList.toggle('hidden', !p.creator_badge);
      $('pubUserBadge').textContent = p.creator_badge ? 'Creator' : 'Listener';
      $('pubStatFollowers').textContent = String(p.followers || 0);
      $('pubStatFollowing').textContent = String(p.following || 0);
      $('pubStatAlbums').textContent = String(p.album_uploads || 0);
      $('pubStatTracks').textContent = String(p.track_uploads || 0);
      const canFollow = !!state.me && !p.is_self && !!p.creator_badge;
      $('btnPubUserFollow').classList.toggle('hidden', !canFollow);
      $('btnPubUserFollow').textContent = p.is_following ? 'Unfollow' : 'Follow';
      $('pubUserCommentForm').classList.toggle('hidden', !state.me);
      $('pubUserCommentGuestHint').classList.toggle('hidden', !!state.me);
      const featuredBits = [];
      if (Array.isArray(p.featured_albums) && p.featured_albums.length) {
        featuredBits.push(`<div><b>Featured Albums</b><div style="display:grid; gap:4px;">${p.featured_albums.map((row) => `<a class="uploader-link" data-featured-album="${row.id}">${escapeHtml(row.title)}</a>`).join('')}</div></div>`);
      } else if (p.featured_album && p.featured_album.title) {
        featuredBits.push(`<div><b>Featured Album</b><div><a class="uploader-link" data-featured-album="${p.featured_album.id}">${escapeHtml(p.featured_album.title)}</a></div></div>`);
      }
      if (p.featured_playlist && p.featured_playlist.name) {
        featuredBits.push(`<div><b>Featured Playlist</b><div><a class="uploader-link" data-featured-playlist="${p.featured_playlist.id}">${escapeHtml(p.featured_playlist.name)}</a></div></div>`);
      }
      if (p.top_track && p.top_track.title) {
        featuredBits.push(`<div><b>Top Track</b><div>${escapeHtml(p.top_track.title)}</div></div>`);
      }
      $('pubUserFeatured').innerHTML = featuredBits.length ? featuredBits.join('') : '<div class="muted">No featured modules.</div>';
      $('pubUserFeatured').querySelectorAll('[data-featured-album]').forEach((featuredAlbumLink) => {
        featuredAlbumLink.onclick = async (e) => {
          e.preventDefault();
          await selectAlbum(Number(featuredAlbumLink.getAttribute('data-featured-album')));
        };
      });
      const featuredPlaylistLink = $('pubUserFeatured').querySelector('[data-featured-playlist]');
      if (featuredPlaylistLink) {
        featuredPlaylistLink.onclick = async (e) => {
          e.preventDefault();
          switchView('playlists');
          await loadPlaylists();
          await selectPlaylist(Number(featuredPlaylistLink.getAttribute('data-featured-playlist')));
        };
      }

      const up = state.selectedPublicUserUploads || { albums: [], tracks: [], playlists: [], favorites: { tracks: [], albums: [], playlists: [] } };
      const albums = up.albums || [];
      const tracks = up.tracks || [];
      const playlists = up.playlists || [];
      const favorites = up.favorites || { tracks: [], albums: [], playlists: [] };
      $('pubUserUploadsPanel').classList.toggle('hidden', !albums.length);
      $('pubUserPlaylistsPanel').classList.toggle('hidden', !(playlists || []).length);
      $('pubUserFavoritesPanel').classList.toggle('hidden', !((favorites.tracks || []).length || (favorites.albums || []).length || (favorites.playlists || []).length));
      $('pubUserUploadsMeta').textContent = `${String(albums.length)} album(s) · ${String(tracks.length)} track(s)`;
      const albumList = $('pubUserAlbumsList');
      albumList.innerHTML = '';
      albums.forEach((a) => {
        const item = document.createElement('div');
        item.className = 'playlist-item';
        item.innerHTML = `
          <div style="font-weight:600;">${escapeHtml(a.title || 'Album')}</div>
          <div class="muted" style="font-size:12px;">${escapeHtml(a.artist || '-')} · ${escapeHtml(a.genre || 'No genre')} · ${escapeHtml(a.visibility || 'public')}</div>
          <div class="muted" style="font-size:12px;">Open album to browse tracks and details.</div>
        `;
        item.onclick = async () => {
          await selectAlbum(Number(a.id));
        };
        albumList.appendChild(item);
      });
      if (!albums.length) {
        albumList.innerHTML = '<div class="muted">No albums visible.</div>';
      }

      $('pubUserPlaylistsMeta').textContent = `${playlists.length} public playlist(s)`;
      const pList = $('pubUserPlaylistsList');
      pList.innerHTML = '';
      playlists.forEach((pl) => {
        const item = document.createElement('div');
        item.className = 'playlist-item';
        item.innerHTML = `<div style="font-weight:600;">${escapeHtml(pl.name || 'Playlist')}</div><div class="muted" style="font-size:12px;">${escapeHtml(pl.visibility || 'public')} · tracks ${Number(pl.track_count || 0)}</div>`;
        item.onclick = async () => {
          switchView('playlists');
          await loadPlaylists();
          await selectPlaylist(pl.id);
        };
        pList.appendChild(item);
      });
      if (!playlists.length) {
        pList.innerHTML = '<div class="muted">No public playlists visible.</div>';
      }

      $('pubUserFavoritesMeta').textContent = `Tracks ${Number((favorites.tracks || []).length)} · Albums ${Number((favorites.albums || []).length)} · Playlists ${Number((favorites.playlists || []).length)}`;
      const favRoot = $('pubUserFavoritesList');
      favRoot.innerHTML = '';
      (favorites.albums || []).forEach((a) => {
        const card = document.createElement('div');
        card.className = 'service-card';
        card.innerHTML = `<div class="service-head"><b>${escapeHtml(a.title || '-')}</b><span class="pill">Album</span></div><div class="muted">${escapeHtml(a.artist || '-')}</div>`;
        card.onclick = async () => selectAlbum(a.id);
        favRoot.appendChild(card);
      });
      (favorites.tracks || []).forEach((t) => {
        const card = document.createElement('div');
        card.className = 'service-card';
        card.innerHTML = `<div class="service-head"><b>${escapeHtml(t.title || '-')}</b><span class="pill">Track</span></div><div class="muted">${escapeHtml(t.artist || '-')} · ${escapeHtml(t.album || '-')}</div>`;
        card.onclick = async () => startTrackById(t.id, state.tracks);
        favRoot.appendChild(card);
      });
      (favorites.playlists || []).forEach((pl) => {
        const card = document.createElement('div');
        card.className = 'service-card';
        card.innerHTML = `<div class="service-head"><b>${escapeHtml(pl.name || '-')}</b><span class="pill">Playlist</span></div><div class="muted">${escapeHtml(pl.visibility || 'public')}</div>`;
        card.onclick = async () => {
          switchView('playlists');
          await loadPlaylists();
          await selectPlaylist(pl.id);
        };
        favRoot.appendChild(card);
      });
      if (!favRoot.children.length) {
        favRoot.innerHTML = '<div class="muted">No public favorites visible.</div>';
      }

      const cRoot = $('pubUserCommentsList');
      cRoot.innerHTML = '';
      (state.selectedPublicUserComments || []).forEach((c) => {
        const item = document.createElement('div');
        item.className = 'album-comment-item';
        const head = document.createElement('div');
        head.className = 'album-comment-head';
        const n = document.createElement('a');
        n.className = 'uploader-link';
        n.textContent = c.author_name || c.author_sub || 'user';
        n.href = '#';
        n.onclick = async (e) => {
          e.preventDefault();
          if (c.author_sub) await openUserProfile(c.author_sub);
        };
        const ts = document.createElement('span');
        ts.textContent = (c.created_at || '').replace('T', ' ').slice(0, 19);
        head.appendChild(n);
        head.appendChild(ts);
        const body = document.createElement('div');
        body.className = 'album-comment-body';
        body.textContent = c.content || '';
        item.appendChild(head);
        item.appendChild(body);
        cRoot.appendChild(item);
      });
      if (!(state.selectedPublicUserComments || []).length) {
        cRoot.innerHTML = '<div class="muted">No comments yet.</div>';
      }
    }

    async function toggleFollowPublicUser() {
      const p = state.selectedPublicUserProfile;
      if (!p || !state.me || p.is_self) return;
      const method = p.is_following ? 'DELETE' : 'POST';
      const res = await apiFetch(`/api/v1/follow/${encodeURIComponent(p.subject)}`, { method, headers: headers() });
      if (!res.ok) {
        alert(`Follow action failed (${res.status}).`);
        return;
      }
      await loadPublicUserProfile();
    }

    async function sendPublicUserComment() {
      if (!state.me || !state.selectedUserSub) return;
      const content = $('pubUserCommentInput').value.trim();
      if (!content) return;
      const res = await apiFetch(`/api/v1/users/${encodeURIComponent(state.selectedUserSub)}/comments`, {
        method: 'POST',
        headers: headers({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ content })
      });
      if (!res.ok) {
        alert(`Profile comment failed (${res.status}).`);
        return;
      }
      $('pubUserCommentInput').value = '';
      await loadPublicUserProfile();
    }

Object.assign(window.HexSonic, {
      displayRoleLabel,
      switchProfileTab,
      profileRouteIdentifier,
      loadMe,
      loadPublicSettings,
      renderMyProfile,
      syncProfilePreviewFromForm,
      currentProfileFormSnapshot,
      renderProfileSaveState,
      updateProfileDirtyState,
      previewLocalProfileImage,
      loadMyProfile,
      fillFeaturedSelectors,
      fillProfileAccentSelector,
      loadNotifications,
      renderNotifications,
      loadFavorites,
      isFavorite,
      toggleFavorite,
      setFavoritesTab,
      renderFavorites,
      saveMyProfile,
      resetMyJukeboxProfile,
      uploadMyAvatar,
      uploadMyBanner,
      updateMyPassword,
      updateMySubsonicPassword,
      deleteMySubsonicPassword,
      openUserProfile,
      loadPublicUserProfile,
      renderPublicUserProfile,
      toggleFollowPublicUser,
      sendPublicUserComment
});
})(window.HexSonic = window.HexSonic || {});
