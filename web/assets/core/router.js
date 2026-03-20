(function(ns) {
  const { state, $, canAdmin, canUpload, canManage } = ns;

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
      if (view === 'playlists' && !ns.canCreatePlaylists()) view = 'discovery';
      if (view === 'favorites' && !state.me) view = 'discovery';
      if (view === 'profile' && !state.me) view = 'discovery';
      if ((view === 'admin_users' || view === 'admin_system' || view === 'admin_logs') && !canAdmin()) view = 'discovery';
      if (view === 'upload' && !canUpload()) view = 'discovery';
      if ((view === 'user_track_manage' || view === 'admin_track_manage' || view === 'album_manage') && !canManage()) view = 'discovery';
      if (view === 'admin_track_manage' && !canAdmin()) view = 'discovery';
      if (view === 'jobs' && !canAdmin()) view = 'discovery';
      if (view === 'creator_stats' && !canUpload()) view = 'discovery';
      if (view === 'invite_register' && !state.inviteToken) view = 'discovery';
      const valid = new Set(['discovery', 'creators', 'albums', 'tracks', 'playlists', 'favorites', 'profile', 'admin_users', 'admin_system', 'admin_logs', 'track_detail', 'upload', 'user_track_manage', 'admin_track_manage', 'album_manage', 'creator_stats', 'jobs', 'user_profile', 'invite_register']);
      if (!valid.has(view)) view = 'discovery';
      return view;
    }

    function readViewFromHash() {
      const raw = (window.location.hash || '').replace(/^#/, '').trim();
      if (!raw) {
        if (state.inviteToken && window.location.pathname.startsWith('/register')) return 'invite_register';
        return 'discovery';
      }
      if (raw.startsWith('user_profile/')) {
        const ident = decodeURIComponent(raw.slice('user_profile/'.length));
        state.selectedUserHandle = ident;
        state.selectedUserSub = ident;
        return 'user_profile';
      }
      return raw;
    }

  function switchView(view, updateHash = true) {
    view = allowedViewOrDefault(view);
    state.currentView = view;

    document.querySelectorAll('.nav-item').forEach((n) => n.classList.toggle('active', n.dataset.view === view));
    $('viewDiscovery').classList.toggle('hidden', view !== 'discovery');
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
    $('viewUpload').classList.toggle('hidden', view !== 'upload');
    $('viewCreatorStats').classList.toggle('hidden', view !== 'creator_stats');
    $('viewTrackManage').classList.toggle('hidden', !(view === 'user_track_manage' || view === 'admin_track_manage'));
    $('viewAlbumManage').classList.toggle('hidden', view !== 'album_manage');
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
    if (view === 'user_track_manage' || view === 'admin_track_manage') {
      ns.applyTrackManageMode(view);
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
    if (view === 'creator_stats' && canUpload()) {
      ns.loadCreatorStats();
    }
    if (view === 'creators') {
      ns.loadCreatorHighscore();
    }
    if (view === 'jobs' && canAdmin()) {
      ns.loadJobs();
    }
    if (view === 'user_profile' && state.selectedUserSub) {
      ns.loadPublicUserProfile();
    }
    const targetHash = (view === 'user_profile' && (state.selectedUserHandle || state.selectedUserSub))
      ? `#user_profile/${encodeURIComponent(state.selectedUserHandle || state.selectedUserSub)}`
      : `#${view}`;
    if (updateHash && window.location.hash !== targetHash) {
      window.location.hash = targetHash.replace(/^#/, '');
    }
  }

  Object.assign(ns, {
    syncTracksContextChrome,
    syncToolbarForView,
    allowedViewOrDefault,
    readViewFromHash,
    switchView
  });
})(window.HexSonic = window.HexSonic || {});
