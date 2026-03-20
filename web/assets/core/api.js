(function(ns) {
  const { state, $, SELECTED_ALBUM_KEY } = ns;

    function headers(extra = {}, tokenOverride = '') {
      const h = { ...extra };
      const tok = tokenOverride || state.token;
      if (tok) h.Authorization = `Bearer ${tok}`;
      return h;
    }

    function saveSelectedAlbumID(id) {
      const n = Number(id || 0);
      if (Number.isFinite(n) && n > 0) {
        localStorage.setItem(SELECTED_ALBUM_KEY, String(n));
      } else {
        localStorage.removeItem(SELECTED_ALBUM_KEY);
      }
    }

    function loadSelectedAlbumID() {
      const raw = localStorage.getItem(SELECTED_ALBUM_KEY) || '';
      const n = Number(raw);
      if (!Number.isFinite(n) || n <= 0) return 0;
      return n;
    }

    function applyInviteFromURL() {
      const url = new URL(window.location.href);
      const token = (url.searchParams.get('invite') || '').trim();
      if (!token) return;
      state.inviteToken = token;
      state.invitePageMode = window.location.pathname.startsWith('/register');
      $('signupDrawer').classList.add('hidden');
      $('authDrawer').classList.add('hidden');
      const note = $('signupInviteNote');
      if (note) {
        note.classList.remove('hidden');
        note.textContent = 'Invite link detected. Registration is enabled for this token.';
      }
      const regMeta = $('inviteRegisterMeta');
      if (regMeta) {
        regMeta.textContent = 'Invite token detected. Complete registration below.';
      }
      syncPublicUI();
    }

    function applyInvitePageMode() {
      document.body.classList.toggle('invite-page', !!state.invitePageMode);
    }

    function saveSession(tokenPayload) {
      state.token = tokenPayload.access_token || '';
      if (tokenPayload.refresh_token) {
        state.refreshToken = tokenPayload.refresh_token;
      }
      const expIn = Number(tokenPayload.expires_in || 300);
      state.tokenExpUnix = Math.floor(Date.now() / 1000) + Math.max(30, expIn-20);
      localStorage.setItem('hex_token', state.token);
      localStorage.setItem('hex_refresh_token', state.refreshToken || '');
      localStorage.setItem('hex_token_exp_unix', String(state.tokenExpUnix));
    }

    function clearSession() {
      state.token = '';
      state.refreshToken = '';
      state.tokenExpUnix = 0;
      state.notifications = [];
      state.notificationsUnread = 0;
      localStorage.removeItem('hex_token');
      localStorage.removeItem('hex_refresh_token');
      localStorage.removeItem('hex_token_exp_unix');
    }

    function stopNotificationPolling() {
      if (state.notificationPollTimer) {
        clearInterval(state.notificationPollTimer);
        state.notificationPollTimer = null;
      }
    }

    function startNotificationPolling() {
      stopNotificationPolling();
      if (!state.me || state.popoutMode) return;
      state.notificationPollTimer = window.setInterval(async () => {
        if (document.hidden || !state.me) return;
        if (typeof ns.loadNotifications === 'function') await ns.loadNotifications();
      }, 15000);
    }

    async function refreshAccessToken() {
      if (!state.refreshToken) return false;
      try {
        const res = await fetch('/api/v1/auth/refresh', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: state.refreshToken })
        });
        if (!res.ok) {
          // Clear session only for explicit auth failures.
          if (res.status === 400 || res.status === 401) {
            clearSession();
          }
          return false;
        }
        const json = await res.json();
        if (!json.access_token) {
          return false;
        }
        saveSession(json);
        return true;
      } catch (_) {
        // Temporary network/proxy hiccup: keep current session state.
        return false;
      }
    }

    async function ensureAccessToken() {
      if (!state.token) return false;
      const now = Math.floor(Date.now() / 1000);
      if (state.tokenExpUnix > now) return true;
      return await refreshAccessToken();
    }

    async function apiFetch(url, options = {}, useAuth = true) {
      const headersIn = { ...(options.headers || {}) };
      if (useAuth && state.token) {
        headersIn.Authorization = `Bearer ${state.token}`;
      }
      let res = await fetch(url, { ...options, headers: headersIn });
      if (useAuth && res.status === 401 && state.refreshToken) {
        const refreshed = await refreshAccessToken();
        if (refreshed) {
          const retryHeaders = { ...(options.headers || {}), Authorization: `Bearer ${state.token}` };
          res = await fetch(url, { ...options, headers: retryHeaders });
        }
      }
      return res;
    }

    function setStatus(msg, cls = '') {
      const el = $('topStatus');
      el.textContent = msg;
      el.className = `status ${cls}`.trim();
    }

    function syncPublicUI() {
      const loggedIn = !!state.me;
      const registrationEnabled = !!(state.publicSettings && state.publicSettings.registration_enabled);
      $('btnOpenSignup').classList.toggle('hidden', loggedIn || (!registrationEnabled && !state.inviteToken));
    }

    function hasRole(role) {
      if (!state.me || !Array.isArray(state.me.roles)) return false;
      return state.me.roles.some((r) => String(r).toLowerCase() === String(role).toLowerCase());
    }

    function canUpload() {
      return canAdmin() || !!(state.me && state.me.creator_badge);
    }

    function canAdmin() {
      return hasRole('admin');
    }

    function canManage() {
      return !!state.me;
    }

    function canManageTrack(track) {
      if (!state.me || !track) return false;
      if (canAdmin()) return true;
      return String(track.owner_sub || '') === String(state.me.subject || '');
    }

  Object.assign(ns, {
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
    canManageTrack
  });
})(window.HexSonic = window.HexSonic || {});
