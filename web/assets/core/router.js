(function(ns) {
  const { state, $, canAdmin, canUpload, canManage } = ns;

    function buildViewPath(view) {
      switch (view) {
        case 'discovery': return '/';
        case 'jukebox': return '/jukebox';
        case 'creators': return '/creators';
        case 'albums': return '/albums';
        case 'tracks':
          if (state.selectedAlbum && Number(state.selectedAlbum.id) > 0 && Number(state.tracksAlbumContextID) === Number(state.selectedAlbum.id)) {
            return `/album/${encodeURIComponent(String(state.selectedAlbum.id))}`;
          }
          return '/tracks';
        case 'playlists': return '/playlists';
        case 'favorites': return '/favorites';
        case 'profile': return '/profile';
        case 'admin_users': return '/admin/users';
        case 'admin_system': return '/admin/system';
        case 'admin_logs': return '/admin/logs';
        case 'creator_center': return '/creator-center';
        case 'creator_stats': return '/creator-stats';
        case 'jobs': return '/jobs';
        case 'track_detail':
          return state.selectedDetailTrackId ? `/track/${encodeURIComponent(state.selectedDetailTrackId)}` : '/tracks';
        case 'user_profile':
          return `/user/${encodeURIComponent(state.selectedUserHandle || state.selectedUserSub || '')}`;
        case 'invite_register':
          return '/register';
        default:
          return '/';
      }
    }

    function syncTracksContextChrome() {
      const hasAlbumContext = state.currentView === 'tracks' && !!(state.selectedAlbum && Number(state.selectedAlbum.id) === Number(state.tracksAlbumContextID));
      $('tracksAlbumHero').classList.toggle('hidden', !hasAlbumContext);
      $('tracksActionbar').classList.toggle('hidden', !hasAlbumContext);
      $('albumSocialPanel').classList.toggle('hidden', !hasAlbumContext);
    }

    function syncToolbarForView(view) {
      const showLibraryToolbar = view === 'albums' || view === 'tracks';
      $('libraryToolbar').classList.toggle('hidden', !showLibraryToolbar);
      if (!showLibraryToolbar) return;
      if (typeof ns.syncGenreFilterControl === 'function') ns.syncGenreFilterControl(view);
      if (view === 'albums') {
        $('searchInput').placeholder = 'Search albums';
        return;
      }
      const hasAlbumContext = !!(state.selectedAlbum && Number(state.selectedAlbum.id) === Number(state.tracksAlbumContextID));
      $('searchInput').placeholder = hasAlbumContext ? 'Search songs in this album' : 'Search songs';
    }
    function allowedViewOrDefault(view) {
      if (view === 'upload' || view === 'user_track_manage' || view === 'admin_track_manage' || view === 'album_manage') view = 'creator_center';
      if (view === 'playlists' && !ns.canCreatePlaylists()) view = 'discovery';
      if (view === 'jukebox' && !state.me) view = 'discovery';
      if (view === 'favorites' && !state.me) view = 'discovery';
      if (view === 'profile' && !state.me) view = 'discovery';
      if ((view === 'admin_users' || view === 'admin_system' || view === 'admin_logs') && !canAdmin()) view = 'discovery';
      if (view === 'creator_center' && !canUpload()) view = 'discovery';
      if (view === 'jobs' && !canAdmin()) view = 'discovery';
      if (view === 'creator_stats' && !canUpload()) view = 'discovery';
      if (view === 'invite_register' && !state.inviteToken) view = 'discovery';
      const valid = new Set(['discovery', 'jukebox', 'creators', 'albums', 'tracks', 'playlists', 'favorites', 'profile', 'admin_users', 'admin_system', 'admin_logs', 'track_detail', 'creator_center', 'user_track_manage', 'admin_track_manage', 'album_manage', 'creator_stats', 'jobs', 'user_profile', 'invite_register']);
      if (!valid.has(view)) view = 'discovery';
      return view;
    }

    function readViewFromHash() {
      const path = String(window.location.pathname || '').trim();
      if (path === '' || path === '/') return 'discovery';
      if (path.startsWith('/album/')) {
        const id = Number(path.slice('/album/'.length));
        state.selectedAlbum = null;
        state.tracksAlbumContextID = Number.isFinite(id) && id > 0 ? id : 0;
        return 'tracks';
      }
      if (path.startsWith('/track/')) {
        const id = decodeURIComponent(path.slice('/track/'.length));
        state.selectedDetailTrackId = id;
        return 'track_detail';
      }
      if (path.startsWith('/user/')) {
        const ident = decodeURIComponent(path.slice('/user/'.length));
        state.selectedUserHandle = ident;
        state.selectedUserSub = ident;
        return 'user_profile';
      }
      if (path === '/jukebox') return 'jukebox';
      if (path === '/creators') return 'creators';
      if (path === '/albums') return 'albums';
      if (path === '/tracks') return 'tracks';
      if (path === '/playlists') return 'playlists';
      if (path === '/favorites') return 'favorites';
      if (path === '/profile') return 'profile';
      if (path === '/creator-center') return 'creator_center';
      if (path === '/creator-stats') return 'creator_stats';
      if (path === '/admin/users') return 'admin_users';
      if (path === '/admin/system') return 'admin_system';
      if (path === '/admin/logs') return 'admin_logs';
      if (path === '/jobs') return 'jobs';
      if (path === '/register') return state.inviteToken ? 'invite_register' : 'discovery';

      const raw = (window.location.hash || '').replace(/^#/, '').trim();
      if (!raw) return 'discovery';
      if (raw.startsWith('album/')) {
        const id = Number(raw.slice('album/'.length));
        state.selectedAlbum = null;
        state.tracksAlbumContextID = Number.isFinite(id) && id > 0 ? id : 0;
        return 'tracks';
      }
      if (raw.startsWith('track/')) {
        const id = decodeURIComponent(raw.slice('track/'.length));
        state.selectedDetailTrackId = id;
        return 'track_detail';
      }
      if (raw.startsWith('user_profile/')) {
        const ident = decodeURIComponent(raw.slice('user_profile/'.length));
        state.selectedUserHandle = ident;
        state.selectedUserSub = ident;
        return 'user_profile';
      }
      return raw;
    }

  function switchView(view, updateHistory = true, replaceHistory = false) {
    view = allowedViewOrDefault(view);
    state.currentView = view;

    document.querySelectorAll('.nav-item').forEach((n) => n.classList.toggle('active', n.dataset.view === view));
    $('viewDiscovery').classList.toggle('hidden', view !== 'discovery');
    $('viewJukebox').classList.toggle('hidden', view !== 'jukebox');
    $('viewCreators').classList.toggle('hidden', view !== 'creators');
    $('viewAlbums').classList.toggle('hidden', view !== 'albums');
    $('viewTracks').classList.toggle('hidden', view !== 'tracks');
    $('viewPlaylists').classList.toggle('hidden', view !== 'playlists');
    $('viewFavorites').classList.toggle('hidden', view !== 'favorites');
    $('viewProfile').classList.toggle('hidden', view !== 'profile');
    $('viewAdminUsers').classList.toggle('hidden', view !== 'admin_users');
    $('viewAdminSystem').classList.toggle('hidden', view !== 'admin_system');
    $('viewAdminLogs').classList.toggle('hidden', view !== 'admin_logs');
    $('viewTrackDetail').classList.toggle('hidden', view !== 'track_detail');
    $('viewCreatorCenter').classList.toggle('hidden', view !== 'creator_center');
    $('viewCreatorStats').classList.toggle('hidden', view !== 'creator_stats');
    $('viewTrackManage').classList.add('hidden');
    $('viewAlbumManage').classList.add('hidden');
    $('viewJobs').classList.toggle('hidden', view !== 'jobs');
    $('viewUserProfile').classList.toggle('hidden', view !== 'user_profile');
    $('viewInviteRegister').classList.toggle('hidden', view !== 'invite_register');
    syncToolbarForView(view);
    ns.applyFilters();
    if (view === 'tracks') {
      ns.renderTracks();
    } else {
      syncTracksContextChrome();
    }
    if (view === 'admin_users' && canAdmin()) {
      ns.loadAdminUsers();
    }
    if (view === 'admin_system' && canAdmin()) {
      ns.loadAdminSystemOverview();
    }
    if (view === 'admin_logs' && canAdmin()) {
      ns.loadAdminDebugToggle();
      ns.loadAdminAuditLogs();
    }
    if (view === 'profile' && state.me) {
      ns.switchProfileTab('identity');
      ns.loadMyProfile();
    }
    if (view === 'favorites' && state.me) {
      ns.loadFavorites().then(() => ns.renderFavorites());
    }
    if (view === 'jukebox' && state.me) {
      ns.renderJukebox();
    }
    if (view === 'creator_center' && canUpload()) {
      ns.loadManageData();
      if (typeof ns.renderCreatorCenter === 'function') ns.renderCreatorCenter();
    }
    if (view === 'creator_stats' && canUpload()) {
      ns.loadCreatorStats();
    }
    if (view === 'creators') {
      ns.loadCreatorHighscore();
    }
    if (view === 'discovery' && typeof ns.ensureDiscoveryLoaded === 'function') {
      ns.ensureDiscoveryLoaded();
    }
    if (view === 'jobs' && canAdmin()) {
      ns.loadJobs();
    }
    if (view === 'user_profile' && state.selectedUserSub) {
      ns.loadPublicUserProfile();
    }
    const targetPath = buildViewPath(view);
    const currentPath = `${window.location.pathname}${window.location.search}`;
    if (updateHistory && currentPath !== targetPath) {
      if (replaceHistory) {
        history.replaceState(null, '', targetPath);
      } else {
        history.pushState(null, '', targetPath);
      }
    }
  }

  Object.assign(ns, {
    buildViewPath,
    syncTracksContextChrome,
    syncToolbarForView,
    allowedViewOrDefault,
    readViewFromHash,
    switchView
  });
})(window.HexSonic = window.HexSonic || {});
