(function(ns) {
    const {
      state,
      ICON_PLAY,
      ICON_PAUSE,
      ICON_MUTE,
      ICON_UNMUTE,
      ICON_DETAIL,
      ICON_PLAYLIST,
      PLAYER_BRIDGE_CHANNEL,
      PLAYER_TICK_MS,
      SELECTED_ALBUM_KEY,
      $,
      escapeHtml,
      fmt,
      normText,
      headers,
      saveSelectedAlbumID,
      loadSelectedAlbumID,
      applyInviteFromURL,
      applyInvitePageMode,
      saveSession,
      clearSession,
      stopNotificationPolling,
      startNotificationPolling,
      refreshAccessToken,
      ensureAccessToken,
      apiFetch,
      setStatus,
      syncPublicUI,
      hasRole,
      canUpload,
      canAdmin,
      canManage,
      canManageTrack,
      syncTracksContextChrome,
      syncToolbarForView,
      allowedViewOrDefault,
      readViewFromHash,
      switchView,
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
      signStream,
      startTrackById,
      activeAudio,
      emitJukeboxState,
      emitJukeboxTick,
      openJukeboxPopout,
      maybeScheduleJukeboxCrossfade,
      playAlbum,
      insertQueueAfterCurrent,
      appendToQueue,
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
      sendPublicUserComment,
      renderDiscoveryTrackList,
      renderDiscoveryAlbumList,
      bindDiscoveryActions,
      renderDiscovery,
      loadDiscovery,
      renderCreatorStats,
      loadCreatorStats,
      syncJukeboxModeControls,
      updateJukeboxControls,
      renderJukeboxNowPlaying,
      renderJukeboxQueue,
      startJukeboxSession,
      refreshJukeboxQueue,
      sendJukeboxFeedback,
      applyJukeboxControl,
      handleJukeboxAdvance,
      renderJukebox,
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
      removeTrackFromPlaylist,
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
      selectAlbum,
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
      uploadAlbumCover,
    } = ns;

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
      $('navJukebox').classList.toggle('hidden', !loggedIn);
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
      if (!loggedIn && !$('viewJukebox').classList.contains('hidden')) {
        switchView('discovery');
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

    function renderAlbumDetail() {
      // Deprecated: album bottom detail strip removed from Albums view.
    }

    function syncUploadMode() {
      const mode = $('uploadMode').value;
      const single = $('uploadSingleFile');
      const album = $('uploadAlbumFiles');
      const title = $('uploadTitle');
      if (mode === 'album') {
        single.classList.add('hidden');
        album.classList.remove('hidden');
        single.value = '';
        title.disabled = true;
        title.placeholder = 'Ignored for album mode (taken from tags/filenames)';
        updateAlbumFileSummary();
      } else {
        single.classList.remove('hidden');
        album.classList.add('hidden');
        album.value = '';
        title.disabled = false;
        title.placeholder = 'Track title (optional, otherwise from tag/filename)';
      }
    }

    function isAudioLikeFile(file) {
      const name = (file.name || '').toLowerCase();
      const type = (file.type || '').toLowerCase();
      if (type.startsWith('audio/')) return true;
      return ['.mp3', '.wav', '.flac', '.m4a', '.aac', '.ogg', '.opus', '.aif', '.aiff', '.wma'].some((ext) => name.endsWith(ext));
    }

    function selectedAlbumAudioFiles() {
      const all = Array.from($('uploadAlbumFiles').files || []);
      const audio = all.filter(isAudioLikeFile);
      return { all, audio, ignored: all.length - audio.length };
    }

    function updateAlbumFileSummary() {
      if ($('uploadMode').value !== 'album') return;
      const stat = selectedAlbumAudioFiles();
      const out = $('uploadStatus');
      out.className = stat.audio.length > 0 ? 'ok' : 'muted';
      out.textContent = `Album selection: ${stat.audio.length} audio files, ${stat.ignored} ignored`;
    }

    function fmtUploadBytes(n) {
      const v = Number(n || 0);
      if (!Number.isFinite(v) || v <= 0) return '0 B';
      const units = ['B', 'KB', 'MB', 'GB'];
      let i = 0;
      let x = v;
      while (x >= 1024 && i < units.length - 1) {
        x /= 1024;
        i += 1;
      }
      const fixed = i === 0 ? 0 : 1;
      return `${x.toFixed(fixed)} ${units[i]}`;
    }

    function setUploadProgress(percent = 0, loaded = 0, total = 0) {
      const p = Math.max(0, Math.min(100, Math.round(Number(percent) || 0)));
      $('uploadProgressFill').style.width = `${p}%`;
      $('uploadProgressLabel').textContent = `${p}%`;
      $('uploadProgressBytes').textContent = `${fmtUploadBytes(loaded)} / ${fmtUploadBytes(total)}`;
    }

    function markUploadProgressDone() {
      $('uploadProgressFill').style.width = '100%';
      $('uploadProgressLabel').textContent = '100%';
      $('uploadProgressBytes').textContent = 'Done';
    }

    function setUploadBusy(busy) {
      const form = $('uploadForm');
      if (!form) return;
      form.querySelectorAll('input, select, button').forEach((el) => {
        el.disabled = !!busy;
      });
    }

    function buildAlbumUploadChunks(files, maxFiles = 8, maxBytes = 48 * 1024 * 1024) {
      const list = Array.from(files || []);
      const chunks = [];
      let cur = [];
      let curBytes = 0;
      list.forEach((f) => {
        const size = Number(f.size || 0);
        const overFiles = cur.length >= maxFiles;
        const overBytes = cur.length > 0 && (curBytes + size) > maxBytes;
        if (overFiles || overBytes) {
          chunks.push(cur);
          cur = [];
          curBytes = 0;
        }
        cur.push(f);
        curBytes += size;
      });
      if (cur.length) chunks.push(cur);
      return chunks;
    }

    async function uploadMultipartWithProgress(url, formData, onProgress = null) {
      await ensureAccessToken();
      return await new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open('POST', url, true);
        if (state.token) {
          xhr.setRequestHeader('Authorization', `Bearer ${state.token}`);
        }
        xhr.upload.onprogress = (ev) => {
          if (typeof onProgress === 'function') {
            onProgress(ev.loaded, ev.total, ev.lengthComputable);
            return;
          }
          if (ev.lengthComputable && ev.total > 0) {
            const p = (ev.loaded / ev.total) * 100;
            setUploadProgress(p, ev.loaded, ev.total || 0);
          } else {
            $('uploadProgressLabel').textContent = 'Uploading...';
            $('uploadProgressBytes').textContent = `${fmtUploadBytes(ev.loaded)} / ?`;
          }
        };
        xhr.onerror = () => reject(new Error('network_error'));
        xhr.onload = () => {
          const text = xhr.responseText || '';
          let json = null;
          try { json = text ? JSON.parse(text) : null; } catch (_) {}
          resolve({ ok: xhr.status >= 200 && xhr.status < 300, status: xhr.status, json, text });
        };
        xhr.send(formData);
      });
    }

    async function handleUpload(e) {
      e.preventDefault();
      if (!canUpload()) {
        alert('Upload requires creator badge or admin role.');
        return;
      }

      const mode = $('uploadMode').value;
      const batchOut = $('uploadBatchResult');
      const out = $('uploadStatus');
      out.className = 'muted';
      out.textContent = 'Uploading...';
      batchOut.textContent = '';
      setUploadProgress(0, 0, 0);
      setUploadBusy(true);

      try {
        if (mode === 'album') {
          const stat = selectedAlbumAudioFiles();
          if (!stat.audio.length) {
            alert('Select album files first.');
            return;
          }

          const chunks = buildAlbumUploadChunks(stat.audio);
          const totalBytes = stat.audio.reduce((sum, f) => sum + Number(f.size || 0), 0);
          let uploadedBytes = 0;
          const agg = { imported: 0, deduped: 0, skipped: 0, failed: 0, results: [] };

          for (let i = 0; i < chunks.length; i += 1) {
            const chunk = chunks[i];
            const chunkBytes = chunk.reduce((sum, f) => sum + Number(f.size || 0), 0);
            out.className = 'muted';
            out.textContent = `Uploading chunk ${i + 1}/${chunks.length} (${chunk.length} files)...`;

            const form = new FormData();
            chunk.forEach((file) => form.append('files', file, file.webkitRelativePath || file.name));
            form.append('artist', e.target.artist.value || '');
            form.append('album', e.target.album.value || '');
            form.append('visibility', e.target.visibility.value || 'private');

            const res = await uploadMultipartWithProgress('/api/v1/tracks/import-batch', form, (loaded, total, computable) => {
              const innerTotal = (computable && total > 0) ? total : chunkBytes;
              const overallLoaded = uploadedBytes + Math.min(innerTotal, Math.max(0, loaded));
              const p = totalBytes > 0 ? (overallLoaded / totalBytes) * 100 : 0;
              setUploadProgress(p, overallLoaded, totalBytes);
            });
            uploadedBytes += chunkBytes;

            if (!res.ok) {
              out.className = 'bad';
              out.textContent = `Batch upload failed in chunk ${i + 1}/${chunks.length} (${res.status}).`;
              return;
            }
            const json = res.json || {};
            agg.imported += Number(json.imported || 0);
            agg.deduped += Number(json.deduped || 0);
            agg.skipped += Number(json.skipped || 0);
            agg.failed += Number(json.failed || 0);
            agg.results = agg.results.concat(Array.isArray(json.results) ? json.results : []);
          }

          markUploadProgressDone();
          out.className = agg.failed > 0 ? 'bad' : 'ok';
          out.textContent = `Batch done: imported=${agg.imported}, deduped=${agg.deduped}, skipped=${agg.skipped}, failed=${agg.failed}`;
          const lines = (agg.results || []).slice(0, 30).map((r) => `${r.file || '-'}: ${r.status}${r.error ? ` (${r.error})` : ''}`);
          batchOut.textContent = lines.join('\n');
          await loadData();
          return;
        }

        const form = new FormData(e.target);
        const res = await uploadMultipartWithProgress('/api/v1/tracks/import', form);
        if (!res.ok) {
          out.className = 'bad';
          out.textContent = `Upload failed (${res.status}).`;
          return;
        }

        markUploadProgressDone();
        const json = res.json || {};
        out.className = 'ok';
        out.textContent = `${json.status}: ${json.track_id}`;
        batchOut.textContent = '';
        e.target.reset();
        syncUploadMode();
        await loadData();
      } catch (err) {
        out.className = 'bad';
        out.textContent = 'Upload failed (network or aborted).';
      } finally {
        setUploadBusy(false);
      }
    }

    async function loginWithCredentials() {
      const username = $('loginUser').value.trim();
      const password = $('loginPass').value;
      if (!username || !password) {
        alert('Username and password required.');
        return;
      }
      const res = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      });
      if (!res.ok) {
        alert('Login failed.');
        return;
      }
      const json = await res.json();
      if (!json.access_token) {
        alert('Login failed: no access token returned.');
        return;
      }
      saveSession(json);
      $('loginPass').value = '';
      closePanels();
      await loadMe();
      await loadData();
    }

    async function signupWithCredentials() {
      const username = $('signupUser').value.trim();
      const email = $('signupEmail').value.trim();
      const password = $('signupPass').value;
      if (!username || !password) {
        alert('Username and password required.');
        return;
      }
      const res = await fetch('/api/v1/auth/signup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, email, password, invite_token: state.inviteToken || '' })
      });
      if (!res.ok) {
        let msg = 'Registration failed.';
        try {
          msg = await res.text();
        } catch (_) {}
        alert(msg || 'Registration failed.');
        return;
      }
      $('loginUser').value = username;
      $('signupPass').value = '';
      $('signupEmail').value = '';
      state.inviteToken = '';
      const note = $('signupInviteNote');
      if (note) {
        note.classList.add('hidden');
        note.textContent = '';
      }
      closePanels();
      alert('Registrierung erfolgreich. Standardrolle: user (hoeren, kommentieren, raten).');
    }

    async function logout() {
      finalizeListeningSession('logout');
      state.me = null;
      stopNotificationPolling();
      clearSession();
      clearAlbumCoverURLs();
      closePanels();
      $('notificationsDrawer').classList.add('hidden');
      hidePlayer();
      setAdminTarget(null);
      clearSelectedPlaylist();
      await loadMe();
      await loadPublicSettings();
      await loadData();
      switchView('albums');
    }

    function bindEvents() {
      $('uploadForm').addEventListener('submit', handleUpload);
      $('uploadMode').onchange = syncUploadMode;
      $('uploadAlbumFiles').addEventListener('change', updateAlbumFileSummary);
      $('uploadForm').addEventListener('reset', () => {
        setTimeout(syncUploadMode, 0);
        $('uploadStatus').className = 'muted';
        $('uploadStatus').textContent = 'No upload started yet.';
        $('uploadBatchResult').textContent = '';
        setUploadProgress(0, 0, 0);
      });

      $('btnReload').onclick = loadData;
      $('searchInput').oninput = () => {
        applyFilters();
        renderAlbums();
        renderTracks();
        renderAlbumDetail();
      };
      $('filterGenre').onchange = () => {
        if (state.currentView === 'albums') state.albumGenreFilter = $('filterGenre').value;
        if (state.currentView === 'tracks') state.trackGenreFilter = $('filterGenre').value;
        applyFilters();
        renderAlbums();
        renderTracks();
        renderAlbumDetail();
      };
      $('filterVisibility').onchange = () => {
        applyFilters();
        renderAlbums();
        renderTracks();
        renderAlbumDetail();
      };

      document.querySelectorAll('.nav-item').forEach((n) => {
        n.onclick = () => {
          if (n.dataset.view === 'tracks') {
            state.tracksAlbumContextID = 0;
            renderTracks();
          }
          switchView(n.dataset.view);
        };
      });
      window.addEventListener('hashchange', () => {
        switchView(readViewFromHash(), false);
      });

      $('btnOpenLogin').onclick = () => {
        $('authDrawer').classList.toggle('hidden');
        $('signupDrawer').classList.add('hidden');
        $('notificationsDrawer').classList.add('hidden');
      };
      $('btnOpenSignup').onclick = () => {
        $('signupDrawer').classList.toggle('hidden');
        $('authDrawer').classList.add('hidden');
        $('notificationsDrawer').classList.add('hidden');
      };
      $('btnNotifications').onclick = async () => {
        await loadNotifications();
        $('notificationsDrawer').classList.toggle('hidden');
        $('authDrawer').classList.add('hidden');
        $('signupDrawer').classList.add('hidden');
      };
      document.addEventListener('visibilitychange', async () => {
        if (!document.hidden && state.me) {
          await loadNotifications();
        }
      });
      window.addEventListener('focus', async () => {
        if (state.me) {
          await loadNotifications();
        }
      });

      $('btnLogin').onclick = loginWithCredentials;
      $('btnSignup').onclick = signupWithCredentials;
      $('btnLogout').onclick = logout;
      $('btnNotificationsReadAll').onclick = async () => {
        await apiFetch('/api/v1/me/notifications/read-all', { method: 'POST', headers: headers() });
        await loadNotifications();
      };
      if ($('btnProfileJukeboxReset')) {
        $('btnProfileJukeboxReset').onclick = resetMyJukeboxProfile;
      }

      $('loginPass').addEventListener('keydown', async (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          await loginWithCredentials();
        }
      });
      $('signupPass').addEventListener('keydown', async (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          await signupWithCredentials();
        }
      });

      $('btnTrackSaveMeta').onclick = adminSetVisibility;
      $('btnAdminDeleteTrack').onclick = adminDeleteTrack;
      $('btnTrackUploadCover').onclick = uploadTrackCover;
      $('btnTrackUploadLyrics').onclick = uploadTrackLyrics;
      $('btnTrackUploadLyricsPlain').onclick = uploadTrackLyricsPlain;
      $('btnTrackLoadID').onclick = async () => {
        if (!canAdmin()) {
          alert('Admin role required.');
          return;
        }
        const id = $('trackLoadID').value.trim();
        if (!id) {
          alert('Please enter a Track ID.');
          return;
        }
        const ok = await loadTrackForEditor(id, false);
        if (!ok) {
          alert('Track not found or no access.');
        }
      };
      $('btnAlbumSaveMeta').onclick = saveAlbumMetadata;
      $('btnAlbumUploadCover').onclick = uploadAlbumCover;
      $('btnAdminRefresh').onclick = loadJobs;
      $('btnAdminUsersRefresh').onclick = loadAdminUsers;
      $('btnAdminUsersDeleteFiltered').onclick = adminDeleteFilteredUsers;
      $('btnAdminInviteCreate').onclick = adminCreateInvite;
      $('btnAdminInviteManage').onclick = async () => {
        $('inviteModalOverlay').classList.remove('hidden');
        await loadAdminInvites();
      };
      $('btnInviteModalClose').onclick = () => $('inviteModalOverlay').classList.add('hidden');
      $('btnInviteModalRefresh').onclick = loadAdminInvites;
      $('inviteModalOverlay').onclick = (e) => {
        if (e.target && e.target.id === 'inviteModalOverlay') {
          $('inviteModalOverlay').classList.add('hidden');
        }
      };
      $('adminInviteLink').onclick = async () => {
        const v = $('adminInviteLink').value.trim();
        if (!v) return;
        try { await navigator.clipboard.writeText(v); } catch (_) {}
      };
      $('btnPlaylistPickerClose').onclick = closePlaylistPickerModal;
      $('btnPlaylistPickerRefresh').onclick = async () => {
        await loadPlaylists();
        renderPlaylistPickerModal();
      };
      $('btnPlaylistPickerCreate').onclick = createPlaylistFromPicker;
      $('playlistPickerModalOverlay').onclick = (e) => {
        if (e.target && e.target.id === 'playlistPickerModalOverlay') {
          closePlaylistPickerModal();
        }
      };
      $('btnAdminSystemRefresh').onclick = loadAdminSystemOverview;
      $('btnAdminLogsRefresh').onclick = loadAdminAuditLogs;
      $('btnAdminDebugSave').onclick = saveAdminDebugToggle;
      $('btnSaveJukeboxSettings').onclick = saveAdminJukeboxSettings;
      $('adminLogSource').onchange = loadAdminAuditLogs;
      $('btnToggleRegistration').onclick = toggleRegistrationSetting;
      $('btnProfileSave').onclick = saveMyProfile;
      $('btnProfileAvatarUpload').onclick = uploadMyAvatar;
      $('btnProfileBannerUpload').onclick = uploadMyBanner;
      $('btnProfilePasswordSave').onclick = updateMyPassword;
      $('btnProfileSubsonicSave').onclick = updateMySubsonicPassword;
      $('btnProfileSubsonicDelete').onclick = deleteMySubsonicPassword;
      document.querySelectorAll('[data-profile-tab-btn]').forEach((el) => {
        el.onclick = () => switchProfileTab(el.getAttribute('data-profile-tab-btn'));
      });
      ['profileDisplayName','profileStatusLine','profileBio','profileAccentColor','profileFeaturedAlbum1','profileFeaturedAlbum2','profileFeaturedAlbum3','profileFeaturedAlbum4','profileFeaturedPlaylist','profileJukeboxGenres','profileGuestShowFollowers','profileGuestShowPlaylists','profileGuestShowFavorites','profileGuestShowStats','profileGuestShowUploads'].forEach((id) => {
        const el = $(id);
        if (!el) return;
        const evt = el.tagName === 'SELECT' || el.type === 'checkbox' ? 'change' : 'input';
        el.addEventListener(evt, syncProfilePreviewFromForm);
      });
      $('profileAvatarFile').addEventListener('change', () => {
        previewLocalProfileImage('profileAvatarFile', 'profileAvatarPreview');
        previewLocalProfileImage('profileAvatarFile', 'profileAvatarBoxPreview');
      });
      $('profileBannerFile').addEventListener('change', () => {
        previewLocalProfileImage('profileBannerFile', 'profileBannerPreview');
        previewLocalProfileImage('profileBannerFile', 'profileBannerBoxPreview');
      });
      $('btnCreatorStatsRefresh').onclick = loadCreatorStats;
      $('creatorStatsWindow').onchange = loadCreatorStats;
      $('btnCreatorHighscoreRefresh').onclick = loadCreatorHighscore;
      $('creatorHighscoreWindow').onchange = loadCreatorHighscore;
      document.querySelectorAll('[data-jukebox-mode]').forEach((btn) => {
        btn.onclick = () => {
          state.jukebox.mode = btn.getAttribute('data-jukebox-mode') || 'for_you';
          syncJukeboxModeControls();
        };
      });
      $('btnJukeboxStart').onclick = startJukeboxSession;
      $('btnJukeboxRefresh').onclick = refreshJukeboxQueue;
      $('btnJukeboxMoreLike').onclick = () => sendJukeboxFeedback('more_like_this');
      $('btnJukeboxLessLike').onclick = () => sendJukeboxFeedback('less_like_this');
      $('btnJukeboxStayGenre').onclick = () => sendJukeboxFeedback('stay_in_genre');
      $('btnJukeboxSurprise').onclick = () => sendJukeboxFeedback('surprise_me');
      $('btnJukeboxPlayPause').onclick = () => applyJukeboxControl('play_pause');
      $('btnJukeboxPrev').onclick = () => applyJukeboxControl('prev');
      $('btnJukeboxNext').onclick = () => applyJukeboxControl('next');
      $('btnJukeboxMute').onclick = () => applyJukeboxControl('mute_toggle');
      $('jukeboxSeekRange').oninput = (e) => applyJukeboxControl('seek', Number(e.target.value) / 1000);
      $('jukeboxVolumeRange').oninput = (e) => applyJukeboxControl('volume', Number(e.target.value));
      $('btnJukeboxPopout').onclick = openJukeboxPopout;
      $('adminUserSearch').addEventListener('keydown', async (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          await loadAdminUsers();
        }
      });
      $('btnCreatePlaylist').onclick = createPlaylist;
      $('btnPlaylistDelete').onclick = deleteSelectedPlaylist;
      document.querySelectorAll('[data-favorites-tab]').forEach((btn) => {
        btn.onclick = () => setFavoritesTab(btn.getAttribute('data-favorites-tab'));
      });
      $('btnOpenPlaylistDock').onclick = () => setPlaylistDockOpen(true);
      $('btnPlaylistDockToggle').onclick = () => setPlaylistDockOpen(!state.playlistDockOpen);
      $('btnPlaylistDockClose').onclick = () => setPlaylistDockOpen(false);
      $('btnPlaylistDockOpen').onclick = () => switchView('playlists');
      $('playlistDockSelect').onchange = async (e) => {
        const id = Number(e.target.value || '0');
        if (!id) {
          clearSelectedPlaylist();
          return;
        }
        await selectPlaylist(id);
      };
      $('btnPlaylistPlay').onclick = async () => {
        if (!state.selectedPlaylistTracks.length) return;
        const list = state.selectedPlaylistTracks.map((x) => ({
          id: x.id, title: x.title, artist: x.artist, album: x.album, duration: Math.round(x.duration_seconds || 0), visibility: x.visibility || 'private', genre: x.genre || ''
        }));
        await startTrackById(list[0].id, list);
      };
      $('btnPlaylistShuffle').onclick = async () => {
        if (!state.selectedPlaylistTracks.length) return;
        const list = state.selectedPlaylistTracks.map((x) => ({
          id: x.id, title: x.title, artist: x.artist, album: x.album, duration: Math.round(x.duration_seconds || 0), visibility: x.visibility || 'private', genre: x.genre || ''
        })).sort(() => Math.random() - 0.5);
        await startTrackById(list[0].id, list);
      };
      $('btnPlaylistDockPlay').onclick = async () => {
        if (!state.selectedPlaylistTracks.length) return;
        const list = state.selectedPlaylistTracks.map((x) => ({
          id: x.id, title: x.title, artist: x.artist, album: x.album, duration: Math.round(x.duration_seconds || 0), visibility: x.visibility || 'private', genre: x.genre || ''
        }));
        await startTrackById(list[0].id, list);
      };
      $('btnPlaylistDockShuffle').onclick = async () => {
        if (!state.selectedPlaylistTracks.length) return;
        const list = state.selectedPlaylistTracks.map((x) => ({
          id: x.id, title: x.title, artist: x.artist, album: x.album, duration: Math.round(x.duration_seconds || 0), visibility: x.visibility || 'private', genre: x.genre || ''
        })).sort(() => Math.random() - 0.5);
        await startTrackById(list[0].id, list);
      };

      $('btnTracksPlay').onclick = () => playAlbum(false);
      $('btnTracksShuffle').onclick = () => playAlbum(true);
      $('btnTracksPlayNext').onclick = () => {
        const rows = getSelectedAlbumTracks();
        if (!rows.length) return;
        insertQueueAfterCurrent(rows);
        alert(`Queued next: ${rows.length} tracks`);
      };
      $('btnTracksPlayLater').onclick = () => {
        const rows = getSelectedAlbumTracks();
        if (!rows.length) return;
        appendToQueue(rows);
        alert(`Queued later: ${rows.length} tracks`);
      };
      $('btnTracksAddPlaylist').onclick = () => {
        const rows = getSelectedAlbumTracks();
        if (!rows.length) return;
        addTracksToPlaylist(rows.map((x) => x.id));
      };
      $('btnAlbumCommentSend').onclick = createAlbumComment;
      $('albumCommentInput').addEventListener('keydown', async (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          await createAlbumComment();
        }
      });
      $('btnDetailBack').onclick = () => switchView('tracks');
      $('btnDetailPlay').onclick = async () => {
        if (!state.selectedDetailTrackId) return;
        await startTrackById(state.selectedDetailTrackId, state.filteredTracks);
      };
      $('btnPopoutPlayer').onclick = openPopoutPlayer;
      $('btnInviteRegisterSubmit').onclick = signupWithInvitePage;
      $('btnInviteRegisterToLogin').onclick = () => {
        state.invitePageMode = false;
        applyInvitePageMode();
        switchView('albums');
        $('authDrawer').classList.remove('hidden');
      };
      $('inviteRegPass').addEventListener('keydown', async (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          await signupWithInvitePage();
        }
      });
      $('btnPubUserBack').onclick = () => switchView('albums');
      $('btnPubUserFollow').onclick = toggleFollowPublicUser;
      $('btnPubUserCommentSend').onclick = sendPublicUserComment;
      $('pubUserCommentInput').addEventListener('keydown', async (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          await sendPublicUserComment();
        }
      });

      $('btnPlayPause').onclick = async () => applyPlayerControl('play_pause');
      $('btnPrev').onclick = async () => applyPlayerControl('prev');
      $('btnNext').onclick = async () => applyPlayerControl('next');
      $('playerThumb').onclick = openAlbumFromPlayerThumb;
      $('playerThumb').addEventListener('keydown', async (e) => {
        if (e.key !== 'Enter' && e.key !== ' ') return;
        e.preventDefault();
        await openAlbumFromPlayerThumb();
      });

      $('volumeRange').oninput = (e) => {
        applyPlayerControl('volume', Number(e.target.value));
      };
      $('eqPreset').onchange = (e) => {
        applyPlayerControl('set_eq', e.target.value);
      };
      $('btnMute').onclick = () => {
        applyPlayerControl('mute_toggle');
      };
      $('seekRange').oninput = (e) => {
        applyPlayerControl('seek', Number(e.target.value) / 1000);
      };

      const bindAudioElement = (audio) => {
      const isJukeboxDeck = audio && audio.id !== 'audio';
      audio.volume = 0.7;
      if (!isJukeboxDeck) {
        updatePlayerButtons();
        applyEQPreset($('eqPreset').value);
      }
      audio.onloadedmetadata = () => {
        if (isJukeboxDeck) {
          updateJukeboxControls();
          emitJukeboxState(true);
          return;
        }
        if (audio !== activeAudio()) return;
        $('durationLabel').textContent = fmt(audio.duration || 0);
        emitPlayerState(true);
      };
      audio.ontimeupdate = () => {
        if (isJukeboxDeck) {
          if (audio !== ns.jukeboxActiveAudio()) return;
          updateListeningSession(audio.currentTime, audio.duration || 0);
          maybeScheduleJukeboxCrossfade();
          updateJukeboxControls();
          if (state.currentView === 'jukebox' && typeof renderJukeboxNowPlaying === 'function') {
            renderJukeboxNowPlaying();
          }
          emitJukeboxTick();
          return;
        }
        if (audio !== activeAudio()) return;
        const p = audio.duration ? (audio.currentTime / audio.duration) : 0;
        $('seekRange').value = Math.round(p * 1000);
        $('elapsedLabel').textContent = fmt(audio.currentTime);
        $('durationLabel').textContent = fmt(audio.duration || 0);
        updateListeningSession(audio.currentTime, audio.duration || 0);
        emitPlayerState(false);
      };
      audio.onended = async () => {
        if (isJukeboxDeck) {
          if (audio !== ns.jukeboxActiveAudio()) return;
          if (state.jukeboxPlayer?.crossfading) return;
          finalizeListeningSession('natural_end');
          await handleJukeboxAdvance();
          return;
        }
        if (audio !== activeAudio()) return;
        if ((state.activeEngine || 'main') === 'jukebox' && state.jukeboxPlayer?.crossfading) return;
        const queue = (state.activeEngine || 'main') === 'jukebox'
          ? (state.jukeboxPlayer?.queue || [])
          : (state.mainPlayer?.queue || []);
        let queueIndex = (state.activeEngine || 'main') === 'jukebox'
          ? Number(state.jukeboxPlayer?.queueIndex || -1)
          : Number(state.mainPlayer?.queueIndex || -1);
        if (!queue.length || queueIndex < 0) return;
        finalizeListeningSession('natural_end');
        if (await handleJukeboxAdvance()) return;
        queueIndex = (queueIndex + 1) % queue.length;
        state.mainPlayer.queueIndex = queueIndex;
        await startTrackById(queue[queueIndex].id, queue);
      };
      audio.onpause = () => {
        if (isJukeboxDeck) {
          if (audio !== ns.jukeboxActiveAudio()) return;
          updateJukeboxControls();
          emitJukeboxState(true);
          return;
        }
        if (audio !== activeAudio()) return;
        updatePlayerButtons();
        emitPlayerState(true);
      };
      audio.onplay = () => {
        if (isJukeboxDeck) {
          if (audio !== ns.jukeboxActiveAudio()) return;
          updateJukeboxControls();
          emitJukeboxState(true);
          return;
        }
        if (audio !== activeAudio()) return;
        updatePlayerButtons();
        emitPlayerState(true);
      };
      audio.onvolumechange = () => {
        if (isJukeboxDeck) {
          if (audio !== ns.jukeboxActiveAudio()) return;
          updateJukeboxControls();
          emitJukeboxState(false);
          return;
        }
        if (audio !== activeAudio()) return;
        $('volumeRange').value = String(Math.round((audio.volume || 0) * 100));
        updatePlayerButtons();
        emitPlayerState(false);
      };
      audio.onseeked = () => {
        if (isJukeboxDeck) {
          if (audio !== ns.jukeboxActiveAudio()) return;
          emitJukeboxState(false);
          return;
        }
        if (audio !== activeAudio()) return;
        const s = state.listeningSession;
        if (s && s.track_id) {
          sendListeningEvent('seek', s.track_id, {
            session_id: s.session_id,
            source_context: s.source_context,
            playback_seconds: Number(audio.currentTime || 0),
            duration_seconds: Number(audio.duration || 0)
          });
        }
        emitPlayerState(false);
      };
      };
      bindAudioElement($('audio'));
      bindAudioElement($('audioJukeboxA'));
      bindAudioElement($('audioJukeboxB'));
    }

    async function boot() {
      $('loginUser').value = '';
      state.popoutMode = isPopoutPlayerMode();
      state.jukeboxPopoutMode = isJukeboxPopoutMode();
      applyPopoutPlayerMode();
      initPlayerBridge();
      initJukeboxBridge();
      if (state.popoutMode) {
        bindPopoutEvents();
        if (!state.popoutSnapshot || !state.popoutSnapshot.track) {
          renderPopoutSnapshot({
            track: null,
            queue: [],
            queue_index: -1,
            paused: true,
            muted: false,
            volume: 70,
            current_time: 0,
            duration: 0,
            eq: 'flat',
            lyrics: { plain: '', cues: [] },
            bins: []
          });
        }
        return;
      }
      if (state.jukeboxPopoutMode) {
        bindJukeboxPopoutEvents();
        return;
      }
      bindEvents();
      startBridgeTicker();
      applyInviteFromURL();
      applyInvitePageMode();
      setPlaylistDockOpen(state.playlistDockOpen);
      syncUploadMode();
      syncJukeboxModeControls();
      renderJukeboxQueue();
      setUploadProgress(0, 0, 0);
      hidePlayer();
      clearSelectedPlaylist();
      await loadPublicSettings();
      await loadMe();
      await loadData();
      syncRoleUI();
      emitPlayerState(true);
      if (state.invitePageMode && state.inviteToken) {
        switchView('invite_register', false);
      } else {
        switchView(readViewFromHash(), false);
      }
    }

    Object.assign(ns, {
      syncRoleUI,
      renderAlbumDetail,
      loadNotifications,
      applyFilters,
      renderTracks,
      applyTrackManageMode,
      loadAdminUsers,
      loadAdminSystemOverview,
      loadAdminDebugToggle,
      loadAdminAuditLogs,
      switchProfileTab,
      loadMyProfile,
      loadFavorites,
      renderFavorites,
      loadCreatorStats,
      loadCreatorHighscore,
      loadJobs,
      loadPublicUserProfile,
      syncGenreFilterControl,
      canCreatePlaylists
    });

    boot();
})(window.HexSonic = window.HexSonic || {});
